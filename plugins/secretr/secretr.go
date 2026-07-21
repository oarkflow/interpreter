// Package secretr registers SPL builtins backed by github.com/oarkflow/secretr:
// a local, encrypted secrets vault (secretr_get/set/delete/list) plus a
// hardcoded-secret source scanner that plugs into
// security.SecurityPolicy.BlockHardcodedSecrets, so scripts are rejected
// before they even run if they embed something that looks like a real API
// key, token, or private key instead of fetching it from the vault.
//
// secretr's module path is private and only resolvable via a local
// replace directive (see go.mod) - this plugin is not usable outside a
// checkout that also has secretr on disk. It must be built with
// `-tags dev` (secretr's own dev-mode build tag) so its licensing checks
// stay out of the picture entirely for this kind of embedded, local use;
// see README.md.
package secretr

import (
	"context"
	"errors"
	"fmt"

	"github.com/oarkflow/secretr/pkg/core/compliance"
	"github.com/oarkflow/secretr/pkg/core/secrets"
	"github.com/oarkflow/secretr/pkg/types"

	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

func init() {
	security.RegisterSecretScanner(scanForHardcodedSecrets)
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		"secretr_get":    {Fn: fnGet},
		"secretr_set":    {Fn: fnSet},
		"secretr_delete": {Fn: fnDelete},
		"secretr_list":   {Fn: fnList},
		"secretr_scan":   {Fn: fnScan},
	})
}

func checkCapability() object.Object {
	if err := security.CheckCapabilityAllowed(security.CapabilitySecrets); err != nil {
		return object.NewError("secretr: %v", err)
	}
	return nil
}

func argString(args []object.Object, idx int, name string) (string, object.Object) {
	if idx >= len(args) {
		return "", object.NewError("secretr: missing argument `%s`", name)
	}
	s, ok := args[idx].(*object.String)
	if !ok {
		return "", object.NewError("secretr: argument `%s` must be STRING, got %s", name, args[idx].Type())
	}
	return s.Value, nil
}

func optString(args []object.Object, idx int, fallback string) string {
	if idx >= len(args) {
		return fallback
	}
	if s, ok := args[idx].(*object.String); ok {
		return s.Value
	}
	return fallback
}

// fnGet fetches a secret's decrypted value and returns it wrapped in a
// SECRET object (see pkg/object.Secret), so it prints as *** everywhere
// (print, logs, error messages) and must be unwrapped explicitly with
// secret_reveal() at the point of use - exactly like config_load()'s
// existing secret-wrapping convention for token/secret/password-like keys.
func fnGet(args ...object.Object) object.Object {
	if errObj := checkCapability(); errObj != nil {
		return errObj
	}
	if len(args) != 1 {
		return object.NewError("secretr_get: wrong number of arguments. got=%d, want=1", len(args))
	}
	name, errObj := argString(args, 0, "name")
	if errObj != nil {
		return errObj
	}
	h, err := getVault()
	if err != nil {
		return object.NewError("secretr_get: %v", err)
	}
	value, err := h.vault.Get(context.Background(), name, h.actorID, false)
	if err != nil {
		if errors.Is(err, secrets.ErrSecretNotFound) {
			return object.NewError("secretr_get: secret %q not found", name)
		}
		return object.NewError("secretr_get: %v", err)
	}
	return &object.Secret{Value: string(value)}
}

// fnSet creates or updates a secret. The value may be a SECRET (as returned
// by secretr_get, secret(), or a config_load() secret field) or a plain
// STRING.
func fnSet(args ...object.Object) object.Object {
	if errObj := checkCapability(); errObj != nil {
		return errObj
	}
	if len(args) != 2 {
		return object.NewError("secretr_set: wrong number of arguments. got=%d, want=2", len(args))
	}
	name, errObj := argString(args, 0, "name")
	if errObj != nil {
		return errObj
	}
	value, errObj := secretOrStringValue(args[1])
	if errObj != nil {
		return errObj
	}
	h, err := getVault()
	if err != nil {
		return object.NewError("secretr_set: %v", err)
	}
	ctx := context.Background()
	_, err = h.vault.Create(ctx, secrets.CreateSecretOptions{
		Name:      name,
		Type:      types.SecretTypeGeneric,
		Value:     []byte(value),
		CreatorID: h.actorID,
	})
	if errors.Is(err, secrets.ErrSecretExists) {
		_, err = h.vault.Update(ctx, name, []byte(value), h.actorID)
	}
	if err != nil {
		return object.NewError("secretr_set: %v", err)
	}
	return object.TRUE
}

func secretOrStringValue(arg object.Object) (string, object.Object) {
	switch v := arg.(type) {
	case *object.Secret:
		return v.Value, nil
	case *object.String:
		return v.Value, nil
	default:
		return "", object.NewError("secretr_set: argument `value` must be SECRET or STRING, got %s", arg.Type())
	}
}

func fnDelete(args ...object.Object) object.Object {
	if errObj := checkCapability(); errObj != nil {
		return errObj
	}
	if len(args) != 1 {
		return object.NewError("secretr_delete: wrong number of arguments. got=%d, want=1", len(args))
	}
	name, errObj := argString(args, 0, "name")
	if errObj != nil {
		return errObj
	}
	h, err := getVault()
	if err != nil {
		return object.NewError("secretr_delete: %v", err)
	}
	if err := h.vault.Delete(context.Background(), name, h.actorID); err != nil {
		return object.NewError("secretr_delete: %v", err)
	}
	return object.TRUE
}

// fnList returns secret names only (never values) matching an optional
// prefix filter.
func fnList(args ...object.Object) object.Object {
	if errObj := checkCapability(); errObj != nil {
		return errObj
	}
	if len(args) > 1 {
		return object.NewError("secretr_list: wrong number of arguments. got=%d, want=0 or 1", len(args))
	}
	prefix := optString(args, 0, "")
	h, err := getVault()
	if err != nil {
		return object.NewError("secretr_list: %v", err)
	}
	list, err := h.vault.List(context.Background(), secrets.ListSecretsOptions{Prefix: prefix})
	if err != nil {
		return object.NewError("secretr_list: %v", err)
	}
	names := make([]object.Object, 0, len(list))
	for _, s := range list {
		names = append(names, &object.String{Value: s.Name})
	}
	return &object.Array{Elements: names}
}

// fnScan runs the same hardcoded-secret DLP scan used automatically by
// BlockHardcodedSecrets against arbitrary text, so a script can check "does
// this string look like a secret" before deciding what to do with it (e.g.
// validating user-submitted config) without needing execution to be
// rejected outright.
func fnScan(args ...object.Object) object.Object {
	if errObj := checkCapability(); errObj != nil {
		return errObj
	}
	if len(args) != 1 {
		return object.NewError("secretr_scan: wrong number of arguments. got=%d, want=1", len(args))
	}
	text, errObj := argString(args, 0, "text")
	if errObj != nil {
		return errObj
	}
	h, err := getVault()
	if err != nil {
		return object.NewError("secretr_scan: %v", err)
	}
	result, err := h.dlp.ScanContent(context.Background(), compliance.ScanOptions{
		Content:      []byte(text),
		ResourceType: "spl_source",
		ActorID:      h.actorID,
	})
	if err != nil {
		return object.NewError("secretr_scan: %v", err)
	}
	findings := make([]object.Object, 0, len(result.Violations))
	for _, v := range result.Violations {
		for _, m := range v.MatchedData {
			pairs := map[object.HashKey]object.HashPair{}
			for key, value := range map[string]object.Object{
				"pattern":  &object.String{Value: m.Pattern},
				"redacted": &object.String{Value: m.Redacted},
				"severity": &object.String{Value: string(v.Severity)},
			} {
				k := &object.String{Value: key}
				pairs[k.HashKey()] = object.HashPair{Key: k, Value: value}
			}
			findings = append(findings, &object.Hash{Pairs: pairs})
		}
	}
	return &object.Array{Elements: findings}
}

// scanForHardcodedSecrets is registered with security.RegisterSecretScanner
// and consulted automatically before a script is parsed/evaluated when
// SecurityPolicy.BlockHardcodedSecrets is set. It reuses the same DLP rule
// (and hence the same builtin secret patterns) as secretr_scan(), so
// "what gets blocked automatically" and "what secretr_scan() reports" never
// drift apart.
func scanForHardcodedSecrets(src string) ([]string, error) {
	h, err := getVault()
	if err != nil {
		return nil, fmt.Errorf("secretr: %w", err)
	}
	result, err := h.dlp.ScanContent(context.Background(), compliance.ScanOptions{
		Content:      []byte(src),
		ResourceType: "spl_source",
		ActorID:      h.actorID,
	})
	if err != nil {
		return nil, fmt.Errorf("secretr: %w", err)
	}
	var violations []string
	for _, v := range result.Violations {
		for _, m := range v.MatchedData {
			violations = append(violations, fmt.Sprintf("%s (%s)", m.Pattern, m.Redacted))
		}
	}
	return violations, nil
}

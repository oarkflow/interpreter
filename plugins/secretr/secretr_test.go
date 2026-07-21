package secretr

import (
	"os"
	"strings"
	"testing"

	interp "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/object"
)

// The package-level vault handle is a process-wide singleton (bootstrapped
// once via sync.Once, matching how a real embedding host has exactly one
// vault per process) - so every test in this file shares one vault instance
// and must use distinct secret names rather than expecting per-test
// isolation. SECRETR_DATA_DIR is set once here, before anything triggers
// bootstrap.
func TestMain(m *testing.M) {
	dir, err := os.MkdirTemp("", "secretr-plugin-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(dir)
	os.Setenv("SECRETR_DATA_DIR", dir)
	m.Run()
}

func str(s string) *object.String { return &object.String{Value: s} }

func requireOK(t *testing.T, result object.Object) object.Object {
	t.Helper()
	if errObj, ok := result.(*object.Error); ok {
		t.Fatalf("expected success, got error: %s", errObj.Message)
	}
	return result
}

func TestSecretrSetGetRoundTrip(t *testing.T) {
	requireOK(t, fnSet(str("test/api-key"), str("sk_live_abc123")))

	got := requireOK(t, fnGet(str("test/api-key")))
	secretObj, ok := got.(*object.Secret)
	if !ok {
		t.Fatalf("expected secretr_get to return a SECRET, got %T", got)
	}
	if secretObj.Value != "sk_live_abc123" {
		t.Fatalf("expected round-tripped value, got %q", secretObj.Value)
	}
	if secretObj.Inspect() != "***" {
		t.Fatalf("expected SECRET to mask on Inspect(), got %q", secretObj.Inspect())
	}
}

func TestSecretrSetUpdatesExisting(t *testing.T) {
	requireOK(t, fnSet(str("test/rotating"), str("v1")))
	requireOK(t, fnSet(str("test/rotating"), str("v2")))
	got := requireOK(t, fnGet(str("test/rotating"))).(*object.Secret)
	if got.Value != "v2" {
		t.Fatalf("expected secretr_set to update an existing secret, got %q", got.Value)
	}
}

func TestSecretrListAndDelete(t *testing.T) {
	requireOK(t, fnSet(str("test/list/a"), str("1")))
	requireOK(t, fnSet(str("test/list/b"), str("2")))

	listed := requireOK(t, fnList(str("test/list/"))).(*object.Array)
	if len(listed.Elements) != 2 {
		t.Fatalf("expected 2 secrets under prefix, got %d: %v", len(listed.Elements), listed.Elements)
	}

	requireOK(t, fnDelete(str("test/list/a")))
	listed = requireOK(t, fnList(str("test/list/"))).(*object.Array)
	if len(listed.Elements) != 1 {
		t.Fatalf("expected 1 secret after delete, got %d", len(listed.Elements))
	}
}

func TestSecretrGetMissingReturnsError(t *testing.T) {
	result := fnGet(str("test/does-not-exist"))
	if _, ok := result.(*object.Error); !ok {
		t.Fatalf("expected an error for a missing secret, got %T", result)
	}
}

func TestSecretrScanDetectsAWSKey(t *testing.T) {
	text := "let key = \"AKIAABCDEFGHIJKLMNOP\";"
	result := requireOK(t, fnScan(str(text))).(*object.Array)
	if len(result.Elements) == 0 {
		t.Fatalf("expected secretr_scan to flag a fake AWS access key, got no findings")
	}
}

func TestSecretrScanCleanTextReportsNothing(t *testing.T) {
	result := requireOK(t, fnScan(str("let name = \"Ada Lovelace\";"))).(*object.Array)
	if len(result.Elements) != 0 {
		t.Fatalf("expected no findings for ordinary text, got %d", len(result.Elements))
	}
}

// TestBlockHardcodedSecretsRejectsScript exercises the full wiring: a
// SecurityPolicy with BlockHardcodedSecrets set causes
// interpreter.ExecWithOptions itself to refuse a script embedding a
// fake-but-real-shaped AWS key, while an equivalent script that fetches the
// same value via secretr_get runs normally.
func TestBlockHardcodedSecretsRejectsScript(t *testing.T) {
	requireOK(t, fnSet(str("test/aws-key"), str("AKIAABCDEFGHIJKLMNOP")))

	policy := &interp.SecurityPolicy{BlockHardcodedSecrets: true}

	_, err := interp.ExecWithOptions(
		`let key = "AKIAABCDEFGHIJKLMNOP"; print key;`,
		nil,
		interp.ExecOptions{Security: policy},
	)
	if err == nil {
		t.Fatalf("expected a hardcoded secret in source to be rejected")
	}
	if !strings.Contains(err.Error(), "hardcoded secret") {
		t.Fatalf("expected a hardcoded-secret error message, got: %v", err)
	}

	_, err = interp.ExecWithOptions(
		`import "secretr" as secretr; let key = secretr.get("test/aws-key"); print "loaded ok";`,
		nil,
		interp.ExecOptions{Security: policy},
	)
	if err != nil {
		t.Fatalf("expected fetching the same value via secretr_get to run cleanly, got: %v", err)
	}
}

// TestSecretrDotNotationNesting proves secretr_set/secretr_get already
// support dot-notation keys like "database.password" - Vault.Create/Get
// treat any name containing "." as addressing a nested structure
// transparently (see secretr's pkg/core/secrets/vault_nested.go), so no
// extra plumbing was needed in this plugin for JSON-path-style secret
// addressing.
func TestSecretrDotNotationNesting(t *testing.T) {
	requireOK(t, fnSet(str("app.database.password"), str("hunter2")))
	requireOK(t, fnSet(str("app.database.host"), str("localhost")))

	got := requireOK(t, fnGet(str("app.database.password"))).(*object.Secret)
	if got.Value != "hunter2" {
		t.Fatalf("expected nested dot-notation get to round-trip, got %q", got.Value)
	}
	got2 := requireOK(t, fnGet(str("app.database.host"))).(*object.Secret)
	if got2.Value != "localhost" {
		t.Fatalf("expected sibling nested key to round-trip independently, got %q", got2.Value)
	}
}

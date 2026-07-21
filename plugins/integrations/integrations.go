//go:build !js

package integrations

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/smtp"
	"os"
	"strings"
	"time"

	"github.com/jlaffaye/ftp"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"

	"github.com/oarkflow/interpreter/pkg/builtins"
	"github.com/oarkflow/interpreter/pkg/eval"
	"github.com/oarkflow/interpreter/pkg/object"
	"github.com/oarkflow/interpreter/pkg/security"
)

// ── Helpers ────────────────────────────────────────────────────────

func runtimeContext(env *object.Environment) context.Context {
	if env != nil && env.RuntimeLimits != nil && env.RuntimeLimits.Ctx != nil {
		return env.RuntimeLimits.Ctx
	}
	return context.Background()
}

func runtimeContextWithTimeout(env *object.Environment, timeout time.Duration) (context.Context, context.CancelFunc) {
	base := runtimeContext(env)
	if timeout > 0 {
		return context.WithTimeout(base, timeout)
	}
	return context.WithCancel(base)
}

func objectToNative(obj object.Object) interface{} {
	switch v := obj.(type) {
	case *object.Integer:
		return v.Value
	case *object.Float:
		return v.Value
	case *object.String:
		return v.Value
	case *object.Boolean:
		return v.Value
	case *object.Null:
		return nil
	case *object.Array:
		result := make([]interface{}, len(v.Elements))
		for i, el := range v.Elements {
			result[i] = objectToNative(el)
		}
		return result
	case *object.Hash:
		result := make(map[string]interface{})
		for _, pair := range v.Pairs {
			result[pair.Key.Inspect()] = objectToNative(pair.Value)
		}
		return result
	default:
		return obj.Inspect()
	}
}

func toObject(val interface{}) object.Object {
	if val == nil {
		return object.NULL
	}
	switch v := val.(type) {
	case object.Object:
		return v
	case bool:
		return object.NativeBoolToBooleanObject(v)
	case int:
		return &object.Integer{Value: int64(v)}
	case int64:
		return &object.Integer{Value: v}
	case uint64:
		return &object.Integer{Value: int64(v)}
	case float64:
		return &object.Float{Value: v}
	case string:
		return &object.String{Value: v}
	case []byte:
		return &object.String{Value: string(v)}
	case map[string]interface{}:
		pairs := make(map[object.HashKey]object.HashPair)
		for k, vv := range v {
			key := &object.String{Value: k}
			pairs[key.HashKey()] = object.HashPair{Key: key, Value: toObject(vv)}
		}
		return &object.Hash{Pairs: pairs}
	case []interface{}:
		elems := make([]object.Object, len(v))
		for i, el := range v {
			elems[i] = toObject(el)
		}
		return &object.Array{Elements: elems}
	default:
		return &object.String{Value: fmt.Sprintf("%v", v)}
	}
}

func objectToDisplayString(obj object.Object) string {
	if obj == nil {
		return "null"
	}
	if s, ok := obj.(*object.Secret); ok {
		_ = s
		return "***"
	}
	if s, ok := obj.(*object.String); ok {
		return s.Value
	}
	return obj.Inspect()
}

func secretValue(v object.Object) (string, bool) {
	switch s := v.(type) {
	case *object.Secret:
		return s.Value, true
	default:
		return "", false
	}
}

func asString(arg object.Object, name string) (string, object.Object) {
	if s, ok := arg.(*object.Secret); ok {
		return s.Value, nil
	}
	if arg.Type() != object.STRING_OBJ {
		return "", object.NewError("argument `%s` must be STRING, got %s", name, arg.Type())
	}
	return arg.(*object.String).Value, nil
}

func tupleOK(v object.Object) object.Object {
	return &object.Array{Elements: []object.Object{v, object.NULL}}
}

func tupleErr(msg string) object.Object {
	return &object.Array{Elements: []object.Object{object.NULL, &object.String{Value: msg}}}
}

func tupleBoolErr(ok bool, msg string) object.Object {
	if ok {
		return &object.Array{Elements: []object.Object{object.TRUE, object.NULL}}
	}
	return &object.Array{Elements: []object.Object{object.FALSE, &object.String{Value: msg}}}
}

func hashGet(h *object.Hash, key string) (object.Object, bool) {
	hk := (&object.String{Value: key}).HashKey()
	pair, ok := h.Pairs[hk]
	if !ok {
		return nil, false
	}
	return pair.Value, true
}

func hashGetString(h *object.Hash, key string, required bool) (string, object.Object) {
	v, ok := hashGet(h, key)
	if !ok {
		if required {
			return "", object.NewError("missing required config key %q", key)
		}
		return "", nil
	}
	if s, ok := secretValue(v); ok {
		return s, nil
	}
	if v.Type() != object.STRING_OBJ {
		return "", object.NewError("config key %q must be STRING/SECRET, got %s", key, v.Type())
	}
	return v.(*object.String).Value, nil
}

func hashGetInt(h *object.Hash, key string, def int, required bool) (int, object.Object) {
	v, ok := hashGet(h, key)
	if !ok {
		if required {
			return 0, object.NewError("missing required config key %q", key)
		}
		return def, nil
	}
	if v.Type() != object.INTEGER_OBJ {
		return 0, object.NewError("config key %q must be INTEGER, got %s", key, v.Type())
	}
	return int(v.(*object.Integer).Value), nil
}

func hashGetStringSlice(h *object.Hash, key string) ([]string, object.Object) {
	v, ok := hashGet(h, key)
	if !ok {
		return nil, nil
	}
	if v.Type() == object.STRING_OBJ {
		return []string{v.(*object.String).Value}, nil
	}
	if v.Type() != object.ARRAY_OBJ {
		return nil, object.NewError("config key %q must be STRING or ARRAY, got %s", key, v.Type())
	}
	arr := v.(*object.Array)
	out := make([]string, 0, len(arr.Elements))
	for i, el := range arr.Elements {
		if el.Type() != object.STRING_OBJ {
			return nil, object.NewError("config key %q element at index %d must be STRING, got %s", key, i, el.Type())
		}
		out = append(out, el.(*object.String).Value)
	}
	return out, nil
}

func objectToStringMap(obj object.Object) (map[string]string, object.Object) {
	if obj == nil || obj == object.NULL {
		return map[string]string{}, nil
	}
	if obj.Type() != object.HASH_OBJ {
		return nil, object.NewError("headers must be HASH, got %s", obj.Type())
	}
	h := obj.(*object.Hash)
	m := make(map[string]string, len(h.Pairs))
	for _, pair := range h.Pairs {
		ks, ok := pair.Key.(*object.String)
		if !ok {
			return nil, object.NewError("headers key must be STRING")
		}
		m[ks.Value] = objectToDisplayString(pair.Value)
	}
	return m, nil
}

// ── HTTP Helpers ──────────────────────────────────────────────────

func parseHTTPBodyAndHeaders(args []object.Object, bodyIndex int) ([]byte, map[string]string, time.Duration, object.Object) {
	body := []byte{}
	headers := map[string]string{}
	timeout := 30 * time.Second

	if len(args) > bodyIndex {
		if args[bodyIndex].Type() == object.STRING_OBJ {
			body = []byte(args[bodyIndex].(*object.String).Value)
		} else if args[bodyIndex] != object.NULL {
			enc, err := json.Marshal(objectToNative(args[bodyIndex]))
			if err != nil {
				return nil, nil, 0, object.NewError("failed to encode request body as JSON: %s", err)
			}
			body = enc
			headers["Content-Type"] = "application/json"
		}
	}

	if len(args) > bodyIndex+1 {
		h, errObj := objectToStringMap(args[bodyIndex+1])
		if errObj != nil {
			return nil, nil, 0, errObj
		}
		for k, v := range h {
			headers[k] = v
		}
	}

	if len(args) > bodyIndex+2 {
		if args[bodyIndex+2].Type() != object.INTEGER_OBJ {
			return nil, nil, 0, object.NewError("timeout_ms must be INTEGER, got %s", args[bodyIndex+2].Type())
		}
		ms := args[bodyIndex+2].(*object.Integer).Value
		if ms <= 0 {
			return nil, nil, 0, object.NewError("timeout_ms must be > 0")
		}
		timeout = time.Duration(ms) * time.Millisecond
	}

	return body, headers, timeout, nil
}

func parseHTTPHeadersAndTimeout(args []object.Object, headersIndex int) (map[string]string, time.Duration, object.Object) {
	headers := map[string]string{}
	timeout := 30 * time.Second

	if len(args) > headersIndex {
		h, errObj := objectToStringMap(args[headersIndex])
		if errObj != nil {
			return nil, 0, errObj
		}
		for k, v := range h {
			headers[k] = v
		}
	}

	if len(args) > headersIndex+1 {
		if args[headersIndex+1].Type() != object.INTEGER_OBJ {
			return nil, 0, object.NewError("timeout_ms must be INTEGER, got %s", args[headersIndex+1].Type())
		}
		ms := args[headersIndex+1].(*object.Integer).Value
		if ms <= 0 {
			return nil, 0, object.NewError("timeout_ms must be > 0")
		}
		timeout = time.Duration(ms) * time.Millisecond
	}

	return headers, timeout, nil
}

func doHTTPRequest(env *object.Environment, method, target string, body []byte, headers map[string]string, timeout time.Duration) object.Object {
	if err := security.CheckNetworkAllowed(target); err != nil {
		return tupleErr(fmt.Sprintf("network policy denied request: %v", err))
	}
	client := &http.Client{
		Timeout: timeout,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if err := security.CheckNetworkAllowed(req.URL.String()); err != nil {
				return fmt.Errorf("redirect blocked by network policy: %w", err)
			}
			if len(via) >= 10 {
				return http.ErrUseLastResponse
			}
			return nil
		},
	}
	ctx, cancel := runtimeContextWithTimeout(env, timeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, strings.ToUpper(method), target, bytes.NewReader(body))
	if err != nil {
		return tupleErr(fmt.Sprintf("invalid request: %v", err))
	}
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	start := time.Now()
	resp, err := client.Do(req)
	if err != nil {
		if errors.Is(ctx.Err(), context.Canceled) {
			return tupleErr("http request cancelled")
		}
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return tupleErr(fmt.Sprintf("http request timed out after %dms", timeout/time.Millisecond))
		}
		return tupleErr(fmt.Sprintf("http request failed: %v", err))
	}
	defer resp.Body.Close()

	bodyLimit := httpBodyLimit(env)
	reader := io.Reader(resp.Body)
	if bodyLimit > 0 {
		reader = io.LimitReader(resp.Body, bodyLimit+1)
	}
	rawBody, err := io.ReadAll(reader)
	if err != nil {
		return tupleErr(fmt.Sprintf("failed reading response body: %v", err))
	}
	if bodyLimit > 0 && int64(len(rawBody)) > bodyLimit {
		return tupleErr(fmt.Sprintf("http response body exceeded %d bytes", bodyLimit))
	}

	headersMap := make(map[object.HashKey]object.HashPair)
	for k, vals := range resp.Header {
		key := &object.String{Value: k}
		headersMap[key.HashKey()] = object.HashPair{Key: key, Value: &object.String{Value: strings.Join(vals, ",")}}
	}

	payload := map[string]interface{}{
		"status":      resp.Status,
		"status_code": resp.StatusCode,
		"body":        string(rawBody),
		"url":         resp.Request.URL.String(),
		"ok":          resp.StatusCode >= 200 && resp.StatusCode < 300,
		"duration_ms": time.Since(start).Milliseconds(),
		"headers":     &object.Hash{Pairs: headersMap},
	}
	return tupleOK(toObject(payload))
}

func httpBodyLimit(env *object.Environment) int64 {
	if env != nil && env.RuntimeLimits != nil && env.RuntimeLimits.MaxHTTPBodyBytes > 0 {
		return env.RuntimeLimits.MaxHTTPBodyBytes
	}
	if v := object.ParsePositiveInt64Env("SPL_HTTP_MAX_BODY_BYTES"); v > 0 {
		return v
	}
	return 1024 * 1024
}

// ── Builtins ───────────────────────────────────────────────────────

func init() {
	eval.RegisterPluginBuiltins(map[string]*object.Builtin{
		"http_request": {FnWithEnv: builtinHTTPRequest},
		"http_get":     {FnWithEnv: builtinHTTPGet},
		"http_post":    {FnWithEnv: builtinHTTPPost},
		"webhook":      {FnWithEnv: builtinWebhook},
		"smtp_send":    {Fn: builtinSMTPSend},
		"ftp_list":     {FnWithEnv: builtinFTPList},
		"ftp_get":      {FnWithEnv: builtinFTPGet},
		"ftp_put":      {FnWithEnv: builtinFTPPut},
		"sftp_list":    {FnWithEnv: builtinSFTPList},
		"sftp_get":     {FnWithEnv: builtinSFTPGet},
		"sftp_put":     {FnWithEnv: builtinSFTPPut},
	})
}

func builtinHTTPRequest(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 5 {
		return tupleErr("http_request requires 2-5 arguments: method, url [, body] [, headers] [, timeout_ms]")
	}
	method, errObj := asString(args[0], "method")
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	target, errObj := asString(args[1], "url")
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	body, headers, timeout, ferr := parseHTTPBodyAndHeaders(args, 2)
	if ferr != nil {
		return tupleErr(ferr.Inspect())
	}
	return doHTTPRequest(env, method, target, body, headers, timeout)
}

func builtinHTTPGet(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 1 || len(args) > 3 {
		return tupleErr("http_get requires 1-3 arguments: url [, headers] [, timeout_ms]")
	}
	target, errObj := asString(args[0], "url")
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	headers, timeout, ferr := parseHTTPHeadersAndTimeout(args, 1)
	if ferr != nil {
		return tupleErr(ferr.Inspect())
	}
	return doHTTPRequest(env, http.MethodGet, target, nil, headers, timeout)
}

func builtinHTTPPost(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 4 {
		return tupleErr("http_post requires 2-4 arguments: url, body [, headers] [, timeout_ms]")
	}
	target, errObj := asString(args[0], "url")
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	body, headers, timeout, ferr := parseHTTPBodyAndHeaders(args, 1)
	if ferr != nil {
		return tupleErr(ferr.Inspect())
	}
	return doHTTPRequest(env, http.MethodPost, target, body, headers, timeout)
}

func builtinWebhook(env *object.Environment, args ...object.Object) object.Object {
	if len(args) < 2 || len(args) > 4 {
		return tupleErr("webhook requires 2-4 arguments: url, payload [, headers] [, timeout_ms]")
	}
	target, errObj := asString(args[0], "url")
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	body, headers, timeout, ferr := parseHTTPBodyAndHeaders(args, 1)
	if ferr != nil {
		return tupleErr(ferr.Inspect())
	}
	if _, ok := headers["Content-Type"]; !ok {
		headers["Content-Type"] = "application/json"
	}
	return doHTTPRequest(env, http.MethodPost, target, body, headers, timeout)
}

// ── SMTP ──────────────────────────────────────────────────────────

var headerCRLFReplacer = strings.NewReplacer("\r", "", "\n", "")

// sanitizeHeaderValue strips CR/LF characters to prevent header/command injection
// when untrusted values are spliced into a manually-built SMTP header block.
func sanitizeHeaderValue(s string) string {
	return headerCRLFReplacer.Replace(s)
}

func sanitizeHeaderValues(vals []string) []string {
	out := make([]string, len(vals))
	for i, v := range vals {
		out[i] = sanitizeHeaderValue(v)
	}
	return out
}

func buildSMTPMessage(from string, to []string, cc []string, subject string, body string, html string) []byte {
	from = sanitizeHeaderValue(from)
	to = sanitizeHeaderValues(to)
	cc = sanitizeHeaderValues(cc)
	subject = sanitizeHeaderValue(subject)

	var b strings.Builder
	b.WriteString("From: " + from + "\r\n")
	b.WriteString("To: " + strings.Join(to, ",") + "\r\n")
	if len(cc) > 0 {
		b.WriteString("Cc: " + strings.Join(cc, ",") + "\r\n")
	}
	b.WriteString("Subject: " + subject + "\r\n")
	b.WriteString("MIME-Version: 1.0\r\n")
	if html != "" {
		b.WriteString("Content-Type: text/html; charset=UTF-8\r\n\r\n")
		b.WriteString(html)
	} else {
		b.WriteString("Content-Type: text/plain; charset=UTF-8\r\n\r\n")
		b.WriteString(body)
	}
	return []byte(b.String())
}

func builtinSMTPSend(args ...object.Object) object.Object {
	if len(args) != 1 {
		return tupleBoolErr(false, "smtp_send requires 1 argument: config hash")
	}
	if args[0].Type() != object.HASH_OBJ {
		return tupleBoolErr(false, fmt.Sprintf("config must be HASH, got %s", args[0].Type()))
	}
	cfg := args[0].(*object.Hash)

	host, errObj := hashGetString(cfg, "host", true)
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	port, errObj := hashGetInt(cfg, "port", 25, true)
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	username, errObj := hashGetString(cfg, "username", false)
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	password, errObj := hashGetString(cfg, "password", false)
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	from, errObj := hashGetString(cfg, "from", true)
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	to, errObj := hashGetStringSlice(cfg, "to")
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	if len(to) == 0 {
		return tupleBoolErr(false, "config key \"to\" is required")
	}
	cc, errObj := hashGetStringSlice(cfg, "cc")
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	bcc, errObj := hashGetStringSlice(cfg, "bcc")
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	subject, errObj := hashGetString(cfg, "subject", false)
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	body, errObj := hashGetString(cfg, "body", false)
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}
	html, errObj := hashGetString(cfg, "html", false)
	if errObj != nil {
		return tupleBoolErr(false, errObj.Inspect())
	}

	recipients := append(append(append([]string{}, to...), cc...), bcc...)
	addr := fmt.Sprintf("%s:%d", host, port)
	if err := security.CheckNetworkAllowed(addr); err != nil {
		return tupleBoolErr(false, fmt.Sprintf("network policy denied smtp_send: %v", err))
	}
	msg := buildSMTPMessage(from, to, cc, subject, body, html)

	var auth smtp.Auth
	if username != "" {
		auth = smtp.PlainAuth("", username, password, host)
	}

	if err := smtp.SendMail(addr, auth, from, recipients, msg); err != nil {
		return tupleBoolErr(false, fmt.Sprintf("smtp send failed: %v", err))
	}
	return tupleBoolErr(true, "")
}

// defaultTransferTimeout bounds FTP/SFTP file transfer operations when the
// script's runtime limits don't already impose a tighter deadline.
const defaultTransferTimeout = 5 * time.Minute

// runWithContext races fn against ctx cancellation/deadline. If ctx fires
// first, it returns a timeout/cancellation error and abandons fn's goroutine
// (fn is expected to finish on its own eventually; the underlying FTP/SFTP
// client libraries used here have no per-operation context support, so this
// is the pragmatic way to bound a stalled transfer without blocking the
// calling goroutine indefinitely).
func runWithContext(ctx context.Context, fn func() error) error {
	done := make(chan error, 1)
	go func() {
		done <- fn()
	}()
	select {
	case err := <-done:
		return err
	case <-ctx.Done():
		if errors.Is(ctx.Err(), context.DeadlineExceeded) {
			return fmt.Errorf("transfer timed out")
		}
		return fmt.Errorf("transfer cancelled")
	}
}

// ── FTP ───────────────────────────────────────────────────────────

func ftpConnect(cfg *object.Hash) (*ftp.ServerConn, object.Object) {
	host, errObj := hashGetString(cfg, "host", true)
	if errObj != nil {
		return nil, errObj
	}
	port, errObj := hashGetInt(cfg, "port", 21, false)
	if errObj != nil {
		return nil, errObj
	}
	username, errObj := hashGetString(cfg, "username", false)
	if errObj != nil {
		return nil, errObj
	}
	password, errObj := hashGetString(cfg, "password", false)
	if errObj != nil {
		return nil, errObj
	}
	timeoutMs, errObj := hashGetInt(cfg, "timeout_ms", 10000, false)
	if errObj != nil {
		return nil, errObj
	}
	if err := security.CheckNetworkAllowed(fmt.Sprintf("%s:%d", host, port)); err != nil {
		return nil, object.NewError("network policy denied ftp connection: %s", err)
	}

	c, err := ftp.Dial(fmt.Sprintf("%s:%d", host, port), ftp.DialWithTimeout(time.Duration(timeoutMs)*time.Millisecond))
	if err != nil {
		return nil, object.NewError("ftp dial failed: %s", err)
	}
	if err := c.Login(username, password); err != nil {
		_ = c.Quit()
		return nil, object.NewError("ftp login failed: %s", err)
	}
	return c, nil
}

func builtinFTPList(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 2 {
		return tupleErr("ftp_list requires 2 arguments: config, remote_dir")
	}
	if args[0].Type() != object.HASH_OBJ || args[1].Type() != object.STRING_OBJ {
		return tupleErr("ftp_list expects (HASH, STRING)")
	}
	c, errObj := ftpConnect(args[0].(*object.Hash))
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	defer c.Quit()

	ctx, cancel := runtimeContextWithTimeout(env, defaultTransferTimeout)
	defer cancel()

	var entries []*ftp.Entry
	err := runWithContext(ctx, func() error {
		var listErr error
		entries, listErr = c.List(args[1].(*object.String).Value)
		return listErr
	})
	if err != nil {
		return tupleErr(fmt.Sprintf("ftp list failed: %v", err))
	}
	out := make([]object.Object, 0, len(entries))
	for _, e := range entries {
		item := map[string]interface{}{
			"name":     e.Name,
			"size":     e.Size,
			"is_dir":   e.Type == ftp.EntryTypeFolder,
			"modified": e.Time.Unix(),
		}
		out = append(out, toObject(item))
	}
	return tupleOK(&object.Array{Elements: out})
}

func builtinFTPGet(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 3 {
		return tupleErr("ftp_get requires 3 arguments: config, remote_path, local_path")
	}
	if args[0].Type() != object.HASH_OBJ || args[1].Type() != object.STRING_OBJ || args[2].Type() != object.STRING_OBJ {
		return tupleErr("ftp_get expects (HASH, STRING, STRING)")
	}
	safePath, err := builtins.SanitizePathLocal(args[2].(*object.String).Value)
	if err != nil {
		return tupleErr(err.Error())
	}
	if err := security.CheckFileWriteAllowed(safePath); err != nil {
		return tupleErr(err.Error())
	}

	c, errObj := ftpConnect(args[0].(*object.Hash))
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	defer c.Quit()

	ctx, cancel := runtimeContextWithTimeout(env, defaultTransferTimeout)
	defer cancel()

	var content []byte
	err = runWithContext(ctx, func() error {
		r, retrErr := c.Retr(args[1].(*object.String).Value)
		if retrErr != nil {
			return fmt.Errorf("ftp retr failed: %w", retrErr)
		}
		defer r.Close()
		data, readErr := io.ReadAll(r)
		if readErr != nil {
			return fmt.Errorf("ftp read failed: %w", readErr)
		}
		content = data
		return nil
	})
	if err != nil {
		return tupleErr(err.Error())
	}
	if err := os.WriteFile(safePath, content, 0o644); err != nil {
		return tupleErr(fmt.Sprintf("local write failed: %v", err))
	}
	return tupleBoolErr(true, "")
}

func builtinFTPPut(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 3 {
		return tupleErr("ftp_put requires 3 arguments: config, local_path, remote_path")
	}
	if args[0].Type() != object.HASH_OBJ || args[1].Type() != object.STRING_OBJ || args[2].Type() != object.STRING_OBJ {
		return tupleErr("ftp_put expects (HASH, STRING, STRING)")
	}
	safePath, err := builtins.SanitizePathLocal(args[1].(*object.String).Value)
	if err != nil {
		return tupleErr(err.Error())
	}
	if err := security.CheckFileReadAllowed(safePath); err != nil {
		return tupleErr(err.Error())
	}
	content, err := os.ReadFile(safePath)
	if err != nil {
		return tupleErr(fmt.Sprintf("local read failed: %v", err))
	}

	c, errObj := ftpConnect(args[0].(*object.Hash))
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	defer c.Quit()

	ctx, cancel := runtimeContextWithTimeout(env, defaultTransferTimeout)
	defer cancel()

	err = runWithContext(ctx, func() error {
		return c.Stor(args[2].(*object.String).Value, bytes.NewReader(content))
	})
	if err != nil {
		return tupleErr(fmt.Sprintf("ftp store failed: %v", err))
	}
	return tupleBoolErr(true, "")
}

// ── SFTP ──────────────────────────────────────────────────────────

func sftpConnect(cfg *object.Hash) (*sftp.Client, *ssh.Client, object.Object) {
	host, errObj := hashGetString(cfg, "host", true)
	if errObj != nil {
		return nil, nil, errObj
	}
	port, errObj := hashGetInt(cfg, "port", 22, false)
	if errObj != nil {
		return nil, nil, errObj
	}
	username, errObj := hashGetString(cfg, "username", true)
	if errObj != nil {
		return nil, nil, errObj
	}
	password, _ := hashGetString(cfg, "password", false)
	privateKey, _ := hashGetString(cfg, "private_key", false)
	timeoutMs, _ := hashGetInt(cfg, "timeout_ms", 10000, false)
	if err := security.CheckNetworkAllowed(fmt.Sprintf("%s:%d", host, port)); err != nil {
		return nil, nil, object.NewError("network policy denied sftp connection: %s", err)
	}

	authMethods := []ssh.AuthMethod{}
	if password != "" {
		authMethods = append(authMethods, ssh.Password(password))
	}
	if privateKey != "" {
		signer, err := ssh.ParsePrivateKey([]byte(privateKey))
		if err != nil {
			return nil, nil, object.NewError("invalid private_key: %s", err)
		}
		authMethods = append(authMethods, ssh.PublicKeys(signer))
	}
	if len(authMethods) == 0 {
		return nil, nil, object.NewError("sftp config requires password or private_key")
	}

	sshConfig := &ssh.ClientConfig{
		User:            username,
		Auth:            authMethods,
		HostKeyCallback: ssh.InsecureIgnoreHostKey(),
		Timeout:         time.Duration(timeoutMs) * time.Millisecond,
	}

	client, err := ssh.Dial("tcp", fmt.Sprintf("%s:%d", host, port), sshConfig)
	if err != nil {
		return nil, nil, object.NewError("sftp dial failed: %s", err)
	}
	sftpClient, err := sftp.NewClient(client)
	if err != nil {
		client.Close()
		return nil, nil, object.NewError("sftp client init failed: %s", err)
	}
	return sftpClient, client, nil
}

func builtinSFTPList(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 2 {
		return tupleErr("sftp_list requires 2 arguments: config, remote_dir")
	}
	if args[0].Type() != object.HASH_OBJ || args[1].Type() != object.STRING_OBJ {
		return tupleErr("sftp_list expects (HASH, STRING)")
	}
	c, sshClient, errObj := sftpConnect(args[0].(*object.Hash))
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	defer c.Close()
	defer sshClient.Close()

	ctx, cancel := runtimeContextWithTimeout(env, defaultTransferTimeout)
	defer cancel()

	entries, err := c.ReadDirContext(ctx, args[1].(*object.String).Value)
	if err != nil {
		return tupleErr(fmt.Sprintf("sftp list failed: %v", err))
	}
	out := make([]object.Object, 0, len(entries))
	for _, e := range entries {
		item := map[string]interface{}{
			"name":     e.Name(),
			"size":     e.Size(),
			"is_dir":   e.IsDir(),
			"modified": e.ModTime().Unix(),
		}
		out = append(out, toObject(item))
	}
	return tupleOK(&object.Array{Elements: out})
}

func builtinSFTPGet(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 3 {
		return tupleErr("sftp_get requires 3 arguments: config, remote_path, local_path")
	}
	if args[0].Type() != object.HASH_OBJ || args[1].Type() != object.STRING_OBJ || args[2].Type() != object.STRING_OBJ {
		return tupleErr("sftp_get expects (HASH, STRING, STRING)")
	}
	safePath, err := builtins.SanitizePathLocal(args[2].(*object.String).Value)
	if err != nil {
		return tupleErr(err.Error())
	}
	if err := security.CheckFileWriteAllowed(safePath); err != nil {
		return tupleErr(err.Error())
	}
	c, sshClient, errObj := sftpConnect(args[0].(*object.Hash))
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	defer c.Close()
	defer sshClient.Close()

	ctx, cancel := runtimeContextWithTimeout(env, defaultTransferTimeout)
	defer cancel()

	var content []byte
	err = runWithContext(ctx, func() error {
		r, openErr := c.Open(args[1].(*object.String).Value)
		if openErr != nil {
			return fmt.Errorf("sftp open failed: %w", openErr)
		}
		defer r.Close()
		data, readErr := io.ReadAll(r)
		if readErr != nil {
			return fmt.Errorf("sftp read failed: %w", readErr)
		}
		content = data
		return nil
	})
	if err != nil {
		return tupleErr(err.Error())
	}
	if err := os.WriteFile(safePath, content, 0o644); err != nil {
		return tupleErr(fmt.Sprintf("local write failed: %v", err))
	}
	return tupleBoolErr(true, "")
}

func builtinSFTPPut(env *object.Environment, args ...object.Object) object.Object {
	if len(args) != 3 {
		return tupleErr("sftp_put requires 3 arguments: config, local_path, remote_path")
	}
	if args[0].Type() != object.HASH_OBJ || args[1].Type() != object.STRING_OBJ || args[2].Type() != object.STRING_OBJ {
		return tupleErr("sftp_put expects (HASH, STRING, STRING)")
	}
	safePath, err := builtins.SanitizePathLocal(args[1].(*object.String).Value)
	if err != nil {
		return tupleErr(err.Error())
	}
	if err := security.CheckFileReadAllowed(safePath); err != nil {
		return tupleErr(err.Error())
	}
	content, err := os.ReadFile(safePath)
	if err != nil {
		return tupleErr(fmt.Sprintf("local read failed: %v", err))
	}

	c, sshClient, errObj := sftpConnect(args[0].(*object.Hash))
	if errObj != nil {
		return tupleErr(errObj.Inspect())
	}
	defer c.Close()
	defer sshClient.Close()

	ctx, cancel := runtimeContextWithTimeout(env, defaultTransferTimeout)
	defer cancel()

	err = runWithContext(ctx, func() error {
		w, createErr := c.Create(args[2].(*object.String).Value)
		if createErr != nil {
			return fmt.Errorf("sftp create failed: %w", createErr)
		}
		defer w.Close()
		if _, writeErr := w.Write(content); writeErr != nil {
			return fmt.Errorf("sftp write failed: %w", writeErr)
		}
		return nil
	})
	if err != nil {
		return tupleErr(err.Error())
	}
	return tupleBoolErr(true, "")
}

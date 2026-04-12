package interpreter

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestStrictModeDeniesNetworkByDefault(t *testing.T) {
	res, err := ExecWithOptions(`let x, e = http_get("https://example.com"); e;`, nil, ExecOptions{
		Security: &SecurityPolicy{StrictMode: true},
	})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	msg, ok := res.(*String)
	if !ok {
		t.Fatalf("expected string error from tuple second, got %T", res)
	}
	if !strings.Contains(strings.ToLower(msg.Value), "denied") {
		t.Fatalf("expected denied message, got %q", msg.Value)
	}
}

func TestStrictModeAllowsNetworkWhenWhitelisted(t *testing.T) {
	_, err := withSecurityPolicyOverride(&SecurityPolicy{StrictMode: true, AllowedNetworkHosts: []string{"example.com"}}, func() (Object, error) {
		if cerr := checkNetworkAllowed("https://example.com"); cerr != nil {
			t.Fatalf("expected allowed host, got %v", cerr)
		}
		return NULL, nil
	})
	if err != nil {
		t.Fatalf("unexpected error from override call: %v", err)
	}
}

func TestEnvWriteDeniedForSPLPrefix(t *testing.T) {
	obj := testEval(`os_env("SPL_DISABLE_EXEC", "0")`)
	msg, ok := obj.(*String)
	if !ok {
		t.Fatalf("expected error string, got %T", obj)
	}
	if !strings.Contains(strings.ToLower(msg.Value), "refusing") {
		t.Fatalf("unexpected response: %q", msg.Value)
	}
}

func TestProtectHostDeniesExec(t *testing.T) {
	_, err := ExecWithOptions(`exec("echo", "hi")`, nil, ExecOptions{
		Security: &SecurityPolicy{ProtectHost: true},
	})
	if err == nil {
		t.Fatalf("expected runtime policy error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "host protection") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProtectHostDeniesFileMutation(t *testing.T) {
	path := filepath.Join("testdata", "host-protection-denied.txt")
	res, err := ExecWithOptions(`let ok, err = write_file("`+path+`", "blocked"); err;`, nil, ExecOptions{
		Security: &SecurityPolicy{ProtectHost: true},
	})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	msg, ok := res.(*String)
	if !ok {
		t.Fatalf("expected string message, got %T", res)
	}
	if !strings.Contains(strings.ToLower(msg.Value), "host protection") {
		t.Fatalf("unexpected error: %q", msg.Value)
	}
}

func TestProtectHostDeniesEnvMutation(t *testing.T) {
	_, err := ExecWithOptions(`os_env("APP_MODE", "sandboxed")`, nil, ExecOptions{
		Security: &SecurityPolicy{ProtectHost: true},
	})
	if err == nil {
		t.Fatalf("expected runtime policy error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "host protection") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestProtectHostDeniesExit(t *testing.T) {
	_, err := ExecWithOptions(`exit(5)`, nil, ExecOptions{
		Security: &SecurityPolicy{ProtectHost: true},
	})
	if err == nil {
		t.Fatalf("expected runtime policy error")
	}
	if !strings.Contains(strings.ToLower(err.Error()), "host protection") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestPlaygroundProtectsHostByDefault(t *testing.T) {
	res := EvalForPlayground(`exec("echo", "hi")`, PlaygroundOptions{TimeoutMS: 2000})
	if res.Error == "" {
		t.Fatalf("expected runtime error")
	}
	if !strings.Contains(strings.ToLower(res.Error), "host protection") {
		t.Fatalf("unexpected error: %q", res.Error)
	}
}

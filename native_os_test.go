//go:build ignore

package interpreter_test

import (
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
)

func TestNativeOSModulePlatformCapabilitiesWhichList(t *testing.T) {
	res, err := ExecWithOptions(`
import "native/os" as os;
let p = os.platform();
let c = os.capabilities();
let w = os.which("go");
let list = os.list({"limit": 20});
{
	"os": p["os"],
	"arch": p["arch"],
	"exec": c["exec"],
	"which_go": w,
	"list_count": len(list)
};
`, nil, ExecOptions{
		Security: &SecurityPolicy{
			AllowedCapabilities: []string{"exec"},
			AllowedExecCommands: []string{"go"},
		},
	})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}
	hash, ok := res.(*Hash)
	if !ok {
		t.Fatalf("expected hash result, got %T", res)
	}
	if got := hashString(t, hash, "os"); got == "" {
		t.Fatalf("expected os platform")
	}
	if got := hashString(t, hash, "arch"); got == "" {
		t.Fatalf("expected arch platform")
	}
	if got := hashBool(t, hash, "exec"); !got {
		t.Fatalf("expected exec capability to be true")
	}
	if got := hashString(t, hash, "which_go"); !strings.Contains(got, "go") {
		t.Fatalf("expected go executable path, got %q", got)
	}
	if got := hashInt(t, hash, "list_count"); got < 0 {
		t.Fatalf("expected non-negative list count, got %d", got)
	}
}

func TestNativeOSRunStructuredSuccessAndLegacyExecCompatibility(t *testing.T) {
	res, err := ExecWithOptions(`
import "native/os" as os;
let structured = os.run("go", ["version"]);
let legacy = exec("go", "version");
{
	"ok": structured["ok"],
	"exit_code": structured["exit_code"],
	"stdout_has_go": contains(structured["stdout"], "go version"),
	"stderr": structured["stderr"],
	"legacy_has_go": contains(legacy, "go version")
};
`, nil, ExecOptions{
		Security: &SecurityPolicy{
			AllowedCapabilities: []string{"exec"},
			AllowedExecCommands: []string{"go"},
		},
	})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}
	hash, ok := res.(*Hash)
	if !ok {
		t.Fatalf("expected hash result, got %T", res)
	}
	if !hashBool(t, hash, "ok") {
		t.Fatalf("expected os.run ok")
	}
	if got := hashInt(t, hash, "exit_code"); got != 0 {
		t.Fatalf("expected exit_code 0, got %d", got)
	}
	if !hashBool(t, hash, "stdout_has_go") {
		t.Fatalf("expected structured stdout to contain go version")
	}
	if got := hashString(t, hash, "stderr"); got != "" {
		t.Fatalf("expected empty stderr, got %q", got)
	}
	if !hashBool(t, hash, "legacy_has_go") {
		t.Fatalf("expected legacy exec string to contain go version")
	}
}

func TestNativeOSRunDeniedByStrictProfileUnlessExecAllowed(t *testing.T) {
	res, err := ExecWithOptions(`
import "native/os" as os;
let r = os.run("go", ["version"]);
r["ok"] == false and contains(r["error"], "exec");
`, nil, ExecOptions{
		Profile:                "untrusted",
		AllowInProcessFallback: true,
	})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}
	if got, ok := res.(*Boolean); !ok || !got.Value {
		t.Fatalf("expected denied native run predicate to be true, got %T %#v", res, res)
	}
}

func TestNativeOSRunDisabledAndDeniedCommand(t *testing.T) {
	t.Setenv("SPL_DISABLE_EXEC", "1")
	res, err := ExecWithOptions(`
import "native/os" as os;
let r = os.run("go", ["version"]);
r["ok"] == false and contains(r["error"], "SPL_DISABLE_EXEC");
`, nil, ExecOptions{
		Security: &SecurityPolicy{
			AllowedCapabilities: []string{"exec"},
			AllowedExecCommands: []string{"go"},
		},
	})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}
	if got, ok := res.(*Boolean); !ok || !got.Value {
		t.Fatalf("expected disabled exec predicate to be true, got %T %#v", res, res)
	}

	t.Setenv("SPL_DISABLE_EXEC", "0")
	res, err = ExecWithOptions(`
import "native/os" as os;
let r = os.run("go", ["version"]);
r["ok"] == false and contains(r["error"], "not allowed");
`, nil, ExecOptions{
		Security: &SecurityPolicy{
			AllowedCapabilities: []string{"exec"},
			AllowedExecCommands: []string{"definitely-not-go"},
		},
	})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}
	if got, ok := res.(*Boolean); !ok || !got.Value {
		t.Fatalf("expected denied command predicate to be true, got %T %#v", res, res)
	}
}

func TestNativeOSRunStderrTimeoutTruncationAndInvalidArgs(t *testing.T) {
	res, err := ExecWithOptions(`
import "native/os" as os;
let fail = os.run("go", ["definitely-not-a-go-command"]);
let tiny = os.run("go", ["version"], {"max_output_bytes": 2});
let timed = os.run("go", ["run", "./testdata/native_sleep.go"], {"timeout_ms": 1});
{
	"failed": fail["ok"] == false,
	"exit_nonzero": fail["exit_code"] != 0,
	"stderr_has_go": contains(fail["stderr"], "go"),
	"truncated": tiny["truncated"],
	"timed_out": timed["timed_out"]
};
`, nil, ExecOptions{
		Security: &SecurityPolicy{
			AllowedCapabilities: []string{"exec"},
			AllowedExecCommands: []string{"go"},
		},
	})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}
	hash, ok := res.(*Hash)
	if !ok {
		t.Fatalf("expected hash result, got %T", res)
	}
	for _, key := range []string{"failed", "exit_nonzero", "stderr_has_go", "truncated", "timed_out"} {
		if !hashBool(t, hash, key) {
			t.Fatalf("expected %s to be true", key)
		}
	}

	_, err = ExecWithOptions(`
import "native/os" as os;
os.run("go", "version");
`, nil, ExecOptions{
		Security: &SecurityPolicy{
			AllowedCapabilities: []string{"exec"},
			AllowedExecCommands: []string{"go"},
		},
	})
	if err == nil || !strings.Contains(err.Error(), "must be ARRAY") {
		t.Fatalf("expected invalid args error, got %v", err)
	}
}

func hashString(t *testing.T, h *Hash, key string) string {
	t.Helper()
	pair, ok := h.Pairs[(&String{Value: key}).HashKey()]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	value, ok := pair.Value.(*String)
	if !ok {
		t.Fatalf("key %q: expected String, got %T", key, pair.Value)
	}
	return value.Value
}

func hashBool(t *testing.T, h *Hash, key string) bool {
	t.Helper()
	pair, ok := h.Pairs[(&String{Value: key}).HashKey()]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	value, ok := pair.Value.(*Boolean)
	if !ok {
		t.Fatalf("key %q: expected Boolean, got %T", key, pair.Value)
	}
	return value.Value
}

func hashInt(t *testing.T, h *Hash, key string) int64 {
	t.Helper()
	pair, ok := h.Pairs[(&String{Value: key}).HashKey()]
	if !ok {
		t.Fatalf("missing key %q", key)
	}
	value, ok := pair.Value.(*Integer)
	if !ok {
		t.Fatalf("key %q: expected Integer, got %T", key, pair.Value)
	}
	return value.Value
}

package interpreter_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/object"
)

func TestRuntimeExecWithPresetAndObservability(t *testing.T) {
	var started bool
	var finished ExecutionMetrics
	rt, err := NewRuntime(RuntimeOptions{
		Profile:                "readonly",
		AllowInProcessFallback: true,
		Observability: &ObservabilityHooks{
			OnStart: func(event ExecutionEvent) {
				started = event.Profile == "readonly"
			},
			OnFinish: func(metrics ExecutionMetrics) {
				finished = metrics
			},
		},
	})
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	res, err := rt.Exec(`40 + 2;`, nil)
	if err != nil {
		t.Fatalf("Runtime.Exec failed: %v", err)
	}
	if got, ok := res.(*Integer); !ok || got.Value != 42 {
		t.Fatalf("unexpected result: %T %#v", res, res)
	}
	if !started || finished.Profile != "readonly" || finished.Duration <= 0 {
		t.Fatalf("observability hooks did not capture execution: started=%v finished=%#v", started, finished)
	}
}

func TestUntrustedObservabilityCapturesOutputBytes(t *testing.T) {
	var finished ExecutionMetrics
	_, err := ExecWithOptions(`print "hello"; 1;`, nil, ExecOptions{
		Profile:                "untrusted",
		AllowInProcessFallback: true,
		Observability: &ObservabilityHooks{
			OnFinish: func(metrics ExecutionMetrics) {
				finished = metrics
			},
		},
	})
	if err != nil {
		t.Fatalf("ExecWithOptions failed: %v", err)
	}
	if finished.Profile != "untrusted" || finished.OutputBytes == 0 {
		t.Fatalf("expected untrusted output metrics, got %#v", finished)
	}
}

func TestRuntimePluginAndStdModuleRegistration(t *testing.T) {
	builtinName := "plugin_answer_runtime_extensibility"
	rt, err := NewRuntime(RuntimeOptions{
		Plugins: []Plugin{PluginFunc{
			PluginName: "runtime-extensibility-test",
			Fn: func(*Runtime) error {
				RegisterRuntimeBuiltins(map[string]*object.Builtin{
					builtinName: {Fn: func(args ...object.Object) object.Object {
						return &object.Integer{Value: 42}
					}},
				})
				return RegisterStdModule("std/test/runtime_extensibility", map[string]Object{
					"answer": &Integer{Value: 42},
				})
			},
		}},
	})
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	res, err := rt.Exec(`import "std/test/runtime_extensibility" as ext; plugin_answer_runtime_extensibility() + ext.answer;`, nil)
	if err != nil {
		t.Fatalf("Runtime.Exec failed: %v", err)
	}
	if got, ok := res.(*Integer); !ok || got.Value != 84 {
		t.Fatalf("unexpected result: %T %#v", res, res)
	}
}

func TestStructuredDiagnosticsAndResourceLimits(t *testing.T) {
	_, err := ExecWithOptions(`let x = ;`, nil, ExecOptions{})
	execErr, ok := err.(*ExecError)
	if !ok || len(execErr.StructuredDiagnostics) == 0 {
		t.Fatalf("expected structured parser diagnostics, got %T %#v", err, err)
	}

	_, err = ExecWithOptions(`[1, 2, 3];`, nil, ExecOptions{MaxArrayLength: 2})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "array length limit") {
		t.Fatalf("expected array length limit error, got %v", err)
	}

	_, err = ExecWithOptions(`"abcdef";`, nil, ExecOptions{MaxStringBytes: 3})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "string literal size limit") {
		t.Fatalf("expected string size limit error, got %v", err)
	}

	_, err = ExecWithOptions(`{"a": 1, "b": 2};`, nil, ExecOptions{MaxHashEntries: 1})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "hash entry limit") {
		t.Fatalf("expected hash entry limit error, got %v", err)
	}
}

func TestImportPolicyAndModuleLockVerification(t *testing.T) {
	dir := t.TempDir()
	dep := filepath.Join(dir, "deps", "mathlib")
	if err := os.MkdirAll(dep, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dep, "math.spl"), []byte(`export let answer = 42;`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, SPLManifestFileName), []byte(`{"module":"app","dependencies":{"mathlib":"./deps/mathlib"}}`), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := SyncModuleLock(dir); err != nil {
		t.Fatalf("SyncModuleLock failed: %v", err)
	}
	if err := VerifyModuleLock(dir); err != nil {
		t.Fatalf("VerifyModuleLock failed: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dep, "math.spl"), []byte(`export let answer = 43;`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := VerifyModuleLock(dir); err == nil {
		t.Fatalf("expected modified dependency to fail lock verification")
	}

	entry := filepath.Join(dir, "entry.spl")
	if err := os.WriteFile(entry, []byte(`import "mathlib/math.spl" as math; math.answer;`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ExecFileWithOptions(entry, nil, ExecOptions{
		Security: &SecurityPolicy{
			StrictMode:          true,
			AllowedCapabilities: []string{"filesystem_read"},
			AllowedFileReadPaths: []string{
				dir,
			},
			AllowedImportPackages: []string{"otherlib/math.spl"},
		},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "import package not allowed") {
		t.Fatalf("expected import package policy denial, got %v", err)
	}
}

func TestUntrustedProfilePropagatesDeniedImportPolicy(t *testing.T) {
	dir := t.TempDir()
	mod := filepath.Join(dir, "mod.spl")
	if err := os.WriteFile(mod, []byte(`export let answer = 42;`), 0o644); err != nil {
		t.Fatal(err)
	}
	entry := filepath.Join(dir, "entry.spl")
	if err := os.WriteFile(entry, []byte(`import "mod.spl" as mod; mod.answer;`), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ExecFileWithOptions(entry, nil, ExecOptions{
		Profile:                "untrusted",
		AllowInProcessFallback: true,
		Security: &SecurityPolicy{
			AllowedCapabilities: []string{"filesystem_read"},
			AllowedFileReadPaths: []string{
				dir,
			},
			DeniedImportPackages: []string{"mod.spl"},
		},
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "import package denied") {
		t.Fatalf("expected denied import package policy to propagate, got %v", err)
	}
}

func TestEnvironmentSnapshotAndNames(t *testing.T) {
	env := NewEnvironment()
	env.Set("b", &Integer{Value: 2})
	env.Set("a", &Integer{Value: 1})
	names := EnvironmentNames(env)
	if strings.Join(names, ",") != "a,b" {
		t.Fatalf("unexpected names: %#v", names)
	}
	snap := EnvironmentSnapshot(env)
	env.Set("a", &Integer{Value: 99})
	if got := snap["a"].(*Integer).Value; got != 1 {
		t.Fatalf("snapshot should retain original object reference, got %d", got)
	}
}

package interpreter_test

import (
	"sync"
	"sync/atomic"
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/security"
)

// TestDenialHookConcurrentSetAndTrigger is a regression guard covering two
// generations of the same underlying state:
//
//  1. Originally, security's denial hook was a bare
//     `var DenialHook func(category, detail string)` written directly from
//     Runtime construction (see NewRuntime in runtime.go) and read from
//     every Check*Allowed denial path (security.notifyDenial), with no
//     synchronization between the two - a genuine data race under
//     `go test -race`. That was fixed with security.SetDenialHook/
//     GetDenialHook, backed by an atomic.Pointer.
//  2. NewRuntime no longer installs a process-wide hook at all: each
//     Runtime's Observability.OnPolicyDenied is now threaded per-call
//     through security.WithDenialHookOverride (see withDenialHookOverride
//     in interpreter.go), scoped to that Runtime's own Exec/ExecFile calls,
//     so concurrently-executing Runtimes with distinct hooks each reliably
//     observe only their own denials instead of colliding on "last write
//     wins" process-wide state (see
//     TestConcurrentExecWithDistinctDenialHooksDoNotCrossFire for the
//     dedicated isolation test).
//
// This test spawns many goroutines concurrently constructing distinct
// Runtimes with distinct Observability.OnPolicyDenied callbacks and calling
// rt.Exec with a StrictMode policy that denies exec, asserting the race
// detector finds nothing, nothing panics, and every Runtime's own hook
// fires for its own denials.
func TestDenialHookConcurrentSetAndTrigger(t *testing.T) {
	const runtimes = 25
	const itersPerGoroutine = 20

	var wg sync.WaitGroup
	var hooksFired int64

	for i := 0; i < runtimes; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itersPerGoroutine; j++ {
				rt, err := NewRuntime(RuntimeOptions{
					Profile: "trusted",
					Security: &SecurityPolicy{
						StrictMode: true,
					},
					Observability: &ObservabilityHooks{
						OnPolicyDenied: func(category, detail string) {
							atomic.AddInt64(&hooksFired, 1)
						},
					},
				})
				if err != nil {
					t.Errorf("NewRuntime failed: %v", err)
					return
				}
				_, _ = rt.Exec(`exec("echo", "hi")`, nil)
			}
		}(i)
	}

	wg.Wait()

	want := int64(runtimes * itersPerGoroutine)
	if got := atomic.LoadInt64(&hooksFired); got != want {
		t.Fatalf("expected every Runtime's own denial hook to fire exactly once per Exec call (got %d, want %d) - each Runtime's hook must be isolated to its own calls, not colliding with concurrent Runtimes", got, want)
	}
}

// TestDenialHookSetGetRaceRaw exercises security.SetDenialHook/GetDenialHook
// directly (bypassing Runtime construction) with a tight concurrent
// set/get/invoke loop, which is closer to a worst-case stress of the
// underlying atomic storage than the Runtime-level test above.
func TestDenialHookSetGetRaceRaw(t *testing.T) {
	t.Cleanup(func() { security.SetDenialHook(nil) })

	const goroutines = 50
	const iters = 200

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				switch j % 3 {
				case 0:
					security.SetDenialHook(func(category, detail string) {})
				case 1:
					security.SetDenialHook(nil)
				default:
					if hook := security.GetDenialHook(); hook != nil {
						hook("race-test", "race-test-detail")
					}
				}
			}
		}(i)
	}
	wg.Wait()
}

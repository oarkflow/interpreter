package interpreter_test

import (
	"sync"
	"sync/atomic"
	"testing"

	. "github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/security"
)

// TestDenialHookConcurrentSetAndTrigger is a regression guard for the data
// race that used to exist on security's process-wide denial hook: it was
// previously a bare `var DenialHook func(category, detail string)` written
// directly from Runtime construction (see NewRuntime in runtime.go) and read
// from every Check*Allowed denial path (security.notifyDenial), with no
// synchronization between the two. Under `go test -race` that produced a
// genuine data race (reproduced during development of this fix).
//
// The fix (security.SetDenialHook / security.GetDenialHook, backed by an
// atomic.Pointer) does not change the documented "last write wins,
// process-wide" semantics: many goroutines may still race to decide *which*
// hook is active, and that race is fine/expected. What must never happen is
// a *data* race on the underlying storage, or a panic from a torn/partial
// read. This test spawns many goroutines concurrently calling
// SetDenialHook with distinct hooks (exercised via constructing distinct
// Runtimes with distinct Observability.OnPolicyDenied callbacks, the real
// call path) alongside many goroutines concurrently triggering policy
// denials that read the current hook (via ExecWithOptions with a
// StrictMode policy that denies exec), and asserts the race detector finds
// nothing and nothing panics.
func TestDenialHookConcurrentSetAndTrigger(t *testing.T) {
	t.Cleanup(func() { security.SetDenialHook(nil) })

	const setters = 25
	const triggers = 25
	const itersPerGoroutine = 20

	var wg sync.WaitGroup
	var hooksFired int64

	// Goroutines that concurrently construct Runtimes with distinct denial
	// hooks, each installing itself as the process-wide hook via
	// security.SetDenialHook (through NewRuntime's real code path).
	for i := 0; i < setters; i++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			for j := 0; j < itersPerGoroutine; j++ {
				_, err := NewRuntime(RuntimeOptions{
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
			}
		}(i)
	}

	// Goroutines that concurrently trigger policy denials (which read the
	// current process-wide hook via security.notifyDenial internally).
	for i := 0; i < triggers; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < itersPerGoroutine; j++ {
				_, _ = ExecWithOptions(`exec("echo", "hi")`, nil, ExecOptions{
					Security: &SecurityPolicy{StrictMode: true},
				})
			}
		}()
	}

	wg.Wait()

	// No strict assertion on the exact count (which hook "wins" any given
	// race is unspecified/last-write-wins by design), just that triggering
	// denials concurrently with hook swaps didn't panic and at least some
	// hook invocations were observed.
	if atomic.LoadInt64(&hooksFired) == 0 {
		t.Fatalf("expected at least one denial hook invocation, got 0")
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

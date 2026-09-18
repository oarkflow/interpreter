package interpreter_test

import (
	"fmt"
	"sync"
	"testing"

	. "github.com/oarkflow/interpreter"
)

// TestConcurrentExecWithDistinctDenialHooksDoNotCrossFire is a regression
// test for the "process-wide, last write wins" denial hook gap: previously
// Observability.OnPolicyDenied was installed once via security.SetDenialHook
// at NewRuntime construction time, so two concurrently-executing
// Runtimes/ExecWithOptions calls with distinct hooks would only ever have
// the most-recently-installed one fire for BOTH. ExecWithOptions now scopes
// each call's hook via security.WithDenialHookOverride (see
// withDenialHookOverride in interpreter.go), so each concurrent call must
// observe only its own denial.
func TestConcurrentExecWithDistinctDenialHooksDoNotCrossFire(t *testing.T) {
	const callers = 20
	const itersPerCaller = 10

	var wg sync.WaitGroup
	errCh := make(chan error, callers*itersPerCaller)

	for c := 0; c < callers; c++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			marker := fmt.Sprintf("caller-%d", id)
			for i := 0; i < itersPerCaller; i++ {
				var observed []string
				_, _ = ExecWithOptions(`exec("echo", "hi")`, nil, ExecOptions{
					Security: &SecurityPolicy{StrictMode: true},
					Observability: &ObservabilityHooks{
						OnPolicyDenied: func(category, detail string) {
							observed = append(observed, marker)
						},
					},
				})
				if len(observed) == 0 {
					errCh <- fmt.Errorf("%s iter %d: expected own denial hook to fire at least once, got none", marker, i)
					continue
				}
				for _, o := range observed {
					if o != marker {
						errCh <- fmt.Errorf("%s iter %d: observed foreign caller's denial marker %q (hook cross-fired across concurrent Execs)", marker, i, o)
					}
				}
			}
		}(c)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

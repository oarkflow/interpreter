package security

import (
	"fmt"
	"sync"
	"testing"
	"time"
)

// TestWithSecurityPolicyOverrideSerializesConcurrentCallers is a regression
// guard documenting (and locking in) the current correct-but-serializing
// behavior of WithSecurityPolicyOverride: two goroutines running under
// different security policies must serialize (never interleave) so that
// neither ever observes the other's policy as ActiveSecurityPolicy() during
// its own execution window. This is the deliberate tradeoff described in
// WithSecurityPolicyOverride's doc comment (mu held for the full duration of
// fn(), not just the swap) - see docs/PRODUCTION_CHECKLIST.md for the planned
// per-execution-context refactor that will remove the need for this
// serialization.
//
// The test also serves as a `go test -race` regression guard: policyOverride
// is process-wide global state (see its doc comment), and this exercises it
// under heavy concurrency to catch any future change that reintroduces an
// unsynchronized access.
func TestWithSecurityPolicyOverrideSerializesConcurrentCallers(t *testing.T) {
	const goroutines = 20
	const itersPerGoroutine = 25

	var mu sync.Mutex
	inside := 0
	maxObservedConcurrent := 0
	var violations int

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			policy := &SecurityPolicy{StrictMode: id%2 == 0}
			for i := 0; i < itersPerGoroutine; i++ {
				_, _ = WithSecurityPolicyOverride(policy, func() (any, error) {
					mu.Lock()
					inside++
					if inside > maxObservedConcurrent {
						maxObservedConcurrent = inside
					}
					if inside > 1 {
						violations++
					}
					mu.Unlock()

					// Give another goroutine a chance to run concurrently
					// with this fn() call, which - if the mutex were absent
					// or broken - would let it observe/overwrite our policy.
					time.Sleep(time.Microsecond)

					active := ActiveSecurityPolicy()
					if active != policy {
						mu.Lock()
						violations++
						mu.Unlock()
					}

					mu.Lock()
					inside--
					mu.Unlock()
					return nil, nil
				})
			}
		}(g)
	}
	wg.Wait()

	if violations > 0 {
		t.Fatalf("observed %d serialization violations (max concurrent fn() calls: %d); WithSecurityPolicyOverride must fully serialize overlapping override scopes", violations, maxObservedConcurrent)
	}
	if maxObservedConcurrent != 1 {
		t.Fatalf("expected exactly 1 concurrent WithSecurityPolicyOverride fn() call at a time, observed max %d", maxObservedConcurrent)
	}
}

// TestActiveSecurityPolicyNoLeakBetweenOverrides concurrently runs many
// goroutines under distinct policies and asserts each goroutine only ever
// observes its own policy while inside its override window - i.e. no policy
// leaks into another goroutine's execution window, matching the documented
// guarantee of WithSecurityPolicyOverride.
func TestActiveSecurityPolicyNoLeakBetweenOverrides(t *testing.T) {
	const goroutines = 30
	const itersPerGoroutine = 30

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*itersPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			policy := &SecurityPolicy{AllowEnvWrite: id%2 == 0}
			for i := 0; i < itersPerGoroutine; i++ {
				_, _ = WithSecurityPolicyOverride(policy, func() (any, error) {
					if got := ActiveSecurityPolicy(); got != policy {
						errCh <- fmt.Errorf("goroutine %d iter %d: policy leaked across concurrent WithSecurityPolicyOverride calls: got %p, want %p", id, i, got, policy)
					}
					return nil, nil
				})
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

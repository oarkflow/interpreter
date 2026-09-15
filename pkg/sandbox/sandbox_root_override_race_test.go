package sandbox

import (
	"fmt"
	"sync"
	"testing"
	"time"

	"github.com/oarkflow/interpreter/pkg/object"
)

// TestWithSandboxRootOverrideSerializesConcurrentCallers is a regression
// guard documenting (and locking in) the current correct-but-serializing
// behavior of WithSandboxRootOverride: concurrent callers with different
// sandbox roots must serialize (never interleave) so that no goroutine ever
// observes another's root via ActiveSandboxBaseDir during its own execution
// window. This mirrors security.WithSecurityPolicyOverride's guarantee - see
// that function's doc comment, and
// docs/PRODUCTION_CHECKLIST.md for the planned per-execution-context
// refactor that will remove the need for this serialization.
//
// sandboxRootOverride is process-wide global state (see its doc comment);
// this test also exercises it under heavy concurrency as a `go test -race`
// regression guard.
func TestWithSandboxRootOverrideSerializesConcurrentCallers(t *testing.T) {
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
			root := fmt.Sprintf("/root-%d", id)
			for i := 0; i < itersPerGoroutine; i++ {
				WithSandboxRootOverride(root, func() object.Object {
					mu.Lock()
					inside++
					if inside > maxObservedConcurrent {
						maxObservedConcurrent = inside
					}
					if inside > 1 {
						violations++
					}
					mu.Unlock()

					time.Sleep(time.Microsecond)

					if got := ActiveSandboxBaseDir(); got != root {
						mu.Lock()
						violations++
						mu.Unlock()
					}

					mu.Lock()
					inside--
					mu.Unlock()
					return nil
				})
			}
		}(g)
	}
	wg.Wait()

	if violations > 0 {
		t.Fatalf("observed %d serialization violations (max concurrent fn() calls: %d); WithSandboxRootOverride must fully serialize overlapping override scopes", violations, maxObservedConcurrent)
	}
	if maxObservedConcurrent != 1 {
		t.Fatalf("expected exactly 1 concurrent WithSandboxRootOverride fn() call at a time, observed max %d", maxObservedConcurrent)
	}
}

// TestActiveSandboxBaseDirNoLeakBetweenOverrides concurrently runs many
// goroutines under distinct sandbox roots and asserts each goroutine only
// ever observes its own root while inside its override window.
func TestActiveSandboxBaseDirNoLeakBetweenOverrides(t *testing.T) {
	const goroutines = 30
	const itersPerGoroutine = 30

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*itersPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			root := fmt.Sprintf("/root-%d", id)
			for i := 0; i < itersPerGoroutine; i++ {
				WithSandboxRootOverride(root, func() object.Object {
					if got := ActiveSandboxBaseDir(); got != root {
						errCh <- fmt.Errorf("goroutine %d iter %d: root leaked across concurrent WithSandboxRootOverride calls: got %q, want %q", id, i, got, root)
					}
					return nil
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

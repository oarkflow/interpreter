package security

import (
	"fmt"
	"sync"
	"testing"
)

// TestWithDenialHookOverrideIsolatesConcurrentCallers is a regression test
// for the process-wide "last write wins" denial hook bug: before
// WithDenialHookOverride existed, the only way to install a denial hook was
// the process-wide SetDenialHook, so two concurrently-executing Runtimes
// with distinct Observability.OnPolicyDenied hooks would only ever have the
// most-recently-installed one fire for both. WithDenialHookOverride scopes
// the hook to the duration of its own fn() call (like
// WithSecurityPolicyOverride does for policies), so each concurrent caller
// must observe only denials it triggers itself.
func TestWithDenialHookOverrideIsolatesConcurrentCallers(t *testing.T) {
	const goroutines = 20
	const itersPerGoroutine = 25

	var wg sync.WaitGroup
	errCh := make(chan error, goroutines*itersPerGoroutine)

	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(id int) {
			defer wg.Done()
			category := fmt.Sprintf("cat-%d", id)
			for i := 0; i < itersPerGoroutine; i++ {
				var observed []string
				hook := func(cat, detail string) {
					observed = append(observed, cat)
				}
				_, _ = WithDenialHookOverride(hook, func() (any, error) {
					notifyDenial(category, "denied")
					notifyDenial(category, "denied again")
					return nil, nil
				})
				if len(observed) != 2 {
					errCh <- fmt.Errorf("goroutine %d iter %d: expected 2 denials observed by own hook, got %d", id, i, len(observed))
					continue
				}
				for _, cat := range observed {
					if cat != category {
						errCh <- fmt.Errorf("goroutine %d iter %d: hook observed foreign category %q, want %q (denial hook leaked across concurrent overrides)", id, i, cat, category)
					}
				}
			}
		}(g)
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Error(err)
	}
}

// TestWithDenialHookOverrideFallsBackToProcessWideHook confirms
// notifyDenial still reaches the process-wide SetDenialHook when no
// per-call override is active, preserving backward compatibility for
// callers that bypass Runtime.Exec/ExecFile.
func TestWithDenialHookOverrideFallsBackToProcessWideHook(t *testing.T) {
	defer SetDenialHook(nil)

	var got string
	SetDenialHook(func(category, detail string) {
		got = category + ":" + detail
	})

	notifyDenial("cap", "no override active")
	if got != "cap:no override active" {
		t.Fatalf("expected process-wide hook to fire when no override is active, got %q", got)
	}

	// While an override is active, it takes priority over the process-wide
	// hook entirely (the override's own fn() should not also invoke the
	// global one).
	got = ""
	var overrideGot string
	_, _ = WithDenialHookOverride(func(category, detail string) {
		overrideGot = category + ":" + detail
	}, func() (any, error) {
		notifyDenial("cap2", "inside override")
		return nil, nil
	})
	if overrideGot != "cap2:inside override" {
		t.Fatalf("expected override hook to fire, got %q", overrideGot)
	}
	if got != "" {
		t.Fatalf("expected process-wide hook NOT to fire while an override is active, got %q", got)
	}
}

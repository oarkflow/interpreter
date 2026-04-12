package interpreter

import (
	"errors"
	"path/filepath"
	"sync"
	"testing"
)

func TestExecOptionsValidationKinds(t *testing.T) {
	_, err := ExecWithOptions("1;", nil, ExecOptions{Timeout: -1})
	if err == nil {
		t.Fatalf("expected validation error")
	}
	var ee *ExecError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if ee.Kind != ExecErrorValidation {
		t.Fatalf("expected validation kind, got %q", ee.Kind)
	}
}

func TestExecFileWithOptionsErrorPathKinds(t *testing.T) {
	_, err := ExecFileWithOptions("missing-file.spl", nil, ExecOptions{})
	if err == nil {
		t.Fatalf("expected io error")
	}
	var ee *ExecError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExecError, got %T", err)
	}
	if ee.Kind != ExecErrorIO {
		t.Fatalf("expected io kind, got %q", ee.Kind)
	}
}

func TestExecConcurrentIsolation(t *testing.T) {
	const n = 40
	var wg sync.WaitGroup
	errCh := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			res, err := ExecWithOptions("let x = 40; let y = 2; x + y;", nil, ExecOptions{MaxSteps: 10000})
			if err != nil {
				errCh <- err
				return
			}
			iv, ok := res.(*Integer)
			if !ok || iv.Value != 42 {
				errCh <- errors.New("unexpected result")
			}
		}()
	}
	wg.Wait()
	close(errCh)
	for err := range errCh {
		t.Fatalf("concurrency run failed: %v", err)
	}
}

func TestExecFileWithOptionsModuleDirOverride(t *testing.T) {
	path := filepath.Join("testdata", "modules", "entry_relative_import.spl")
	res, err := ExecFileWithOptions(path, nil, ExecOptions{ModuleDir: filepath.Join("testdata", "modules")})
	if err != nil {
		t.Fatalf("ExecFileWithOptions failed: %v", err)
	}
	if _, ok := res.(*Integer); !ok {
		t.Fatalf("expected integer result, got %T", res)
	}
}

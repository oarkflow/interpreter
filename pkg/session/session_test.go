package session_test

import (
	"strings"
	"testing"
	"time"

	session "github.com/oarkflow/interpreter/pkg/session"
)

// TestSessionCancelIdleIsNoOp ensures calling Cancel on a session with no
// in-flight execution does not panic and returns no error.
func TestSessionCancelIdleIsNoOp(t *testing.T) {
	s, err := session.New(session.SessionOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	if err := s.Cancel(); err != nil {
		t.Fatalf("Cancel on idle session returned error: %v", err)
	}
}

// TestSessionCancelStopsRunningExecution starts a long-running loop in a
// goroutine, calls Cancel shortly after, and verifies the execution ends
// with a cancellation-classified error rather than hanging or timing out.
func TestSessionCancelStopsRunningExecution(t *testing.T) {
	s, err := session.New(session.SessionOptions{})
	if err != nil {
		t.Fatalf("New: %v", err)
	}

	resultCh := make(chan session.ExecutionResult, 1)
	go func() {
		resultCh <- s.Execute(session.ExecutionRequest{Source: `while (true) { }`})
	}()

	// Give the execution a moment to actually start running before cancelling.
	time.Sleep(20 * time.Millisecond)
	if err := s.Cancel(); err != nil {
		t.Fatalf("Cancel: %v", err)
	}

	select {
	case res := <-resultCh:
		if res.OK {
			t.Fatalf("expected cancelled execution to fail, got OK result")
		}
		if !strings.Contains(res.Error, "execution cancelled") {
			t.Fatalf("expected cancellation error, got: %q", res.Error)
		}
		if res.Metrics.ErrorKind != session.ErrorKindCancelled {
			t.Fatalf("expected ErrorKindCancelled, got %q", res.Metrics.ErrorKind)
		}
	case <-time.After(5 * time.Second):
		t.Fatalf("execution did not stop after Cancel()")
	}
}

// TestSessionTimeoutClassifiedAsTimeout ensures a session-level Timeout that
// elapses is classified as ErrorKindTimeout, distinct from an explicit
// Cancel() (ErrorKindCancelled).
func TestSessionTimeoutClassifiedAsTimeout(t *testing.T) {
	s, err := session.New(session.SessionOptions{Timeout: 20 * time.Millisecond})
	if err != nil {
		t.Fatalf("New: %v", err)
	}
	res := s.Execute(session.ExecutionRequest{Source: `while (true) { }`})
	if res.OK {
		t.Fatalf("expected timed-out execution to fail, got OK result")
	}
	if res.Metrics.ErrorKind != session.ErrorKindTimeout {
		t.Fatalf("expected ErrorKindTimeout, got %q (error=%q)", res.Metrics.ErrorKind, res.Error)
	}
}

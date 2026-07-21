//go:build ignore

package interpreter_test

import (
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
)

func TestSessionExecutionCheckpointRestoreReplayAndInspect(t *testing.T) {
	rt, err := NewRuntime(RuntimeOptions{Profile: "trusted", AllowInProcessFallback: true})
	if err != nil {
		t.Fatalf("NewRuntime failed: %v", err)
	}
	sess, err := rt.NewSession(SessionOptions{ID: "test-session"})
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}

	res := sess.Execute(ExecutionRequest{Source: `let x = 40; x + 2;`})
	if !res.OK || res.ResultText != "42" {
		t.Fatalf("unexpected execution result: %#v", res)
	}
	if got := sess.Inspect().Variables["x"]; got != "40" {
		t.Fatalf("expected x in inspection, got %q", got)
	}

	if _, err := sess.Checkpoint("base"); err != nil {
		t.Fatalf("checkpoint failed: %v", err)
	}
	res = sess.Execute(ExecutionRequest{Source: `x = 99; x;`})
	if !res.OK || res.ResultText != "99" {
		t.Fatalf("unexpected mutation result: %#v", res)
	}
	if err := sess.Restore("base"); err != nil {
		t.Fatalf("restore failed: %v", err)
	}
	if got := sess.Inspect().Variables["x"]; got != "40" {
		t.Fatalf("expected restore to recover x=40, got %q", got)
	}

	results, err := sess.Replay()
	if err != nil {
		t.Fatalf("replay failed: %v", err)
	}
	if len(results) == 0 || !results[len(results)-1].OK {
		t.Fatalf("expected successful replay, got %#v", results)
	}
	events := sess.Events()
	if len(events) == 0 {
		t.Fatalf("expected event stream")
	}
}

func TestSessionAssistantIsDisabledByDefault(t *testing.T) {
	sess, err := NewSession(SessionOptions{ID: "assistant-disabled"})
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	_, err = sess.Ask("ask", "what happened?")
	if err == nil || !strings.Contains(err.Error(), "assistant provider is not configured") {
		t.Fatalf("expected disabled assistant error, got %v", err)
	}
}

func TestSessionDebugTraceAndArtifacts(t *testing.T) {
	sess, err := NewSession(SessionOptions{ID: "debug-artifacts"})
	if err != nil {
		t.Fatalf("NewSession failed: %v", err)
	}
	trace := sess.Debug("let x = 1; x + 2;", "<debug>")
	if !trace.OK || len(trace.Steps) != 2 || trace.Steps[1].Result != "3" {
		t.Fatalf("unexpected debug trace: %#v", trace)
	}
	res := sess.Execute(ExecutionRequest{Source: `render([{"name": "Ada"}], {"name": "people.json"});`})
	if !res.OK || len(res.Artifacts) != 1 || res.Artifacts[0].Name != "people.json" {
		t.Fatalf("expected render artifact summary, got %#v", res)
	}
}

func TestPooledEnvironmentCanBeReused(t *testing.T) {
	env := NewPooledEnvironment()
	env.Set("x", &Integer{Value: 1})
	ReleasePooledEnvironment(env)

	reused := NewPooledEnvironment()
	defer ReleasePooledEnvironment(reused)
	if _, ok := reused.Get("x"); ok {
		t.Fatalf("pooled environment retained previous binding")
	}
	reused.Set("y", &Integer{Value: 2})
	if got, ok := reused.Get("y"); !ok || got.(*Integer).Value != 2 {
		t.Fatalf("pooled environment is not usable after reset")
	}
}

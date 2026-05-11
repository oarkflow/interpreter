package interpreter_test

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"
	"time"

	. "github.com/oarkflow/interpreter"
	_ "github.com/oarkflow/interpreter/pkg/builtins/scheduler"
	_ "github.com/oarkflow/interpreter/pkg/builtins/server"
	_ "github.com/oarkflow/interpreter/pkg/builtins/watcher"
	"github.com/oarkflow/interpreter/pkg/eval"
)

func TestExecUntrustedDeniesHostCapabilitiesByDefault(t *testing.T) {
	cases := []string{
		`exec("sh", "-c", "echo hacked")`,
		`listen_async(server(0));`,
		`watch(".", function(files) { files; });`,
		`schedule_interval(1000, function() { 1; });`,
		`go_async(function() { 1; });`,
		`background(function() { 1; });`,
	}
	if eval.HasBuiltin("http_get") {
		cases = append(cases, `let _, err = http_get("https://example.com"); if (err != null) { throw err; }`)
	}
	if eval.HasBuiltin("db_connect") {
		cases = append(cases, `let db, err = db_connect("sqlite", ":memory:"); if (err != null) { throw err; }`)
	}

	for _, script := range cases {
		_, err := ExecUntrustedWithOptions(script, nil, UntrustedExecOptions{InProcess: true})
		if err == nil {
			t.Fatalf("expected untrusted script to be denied: %s", script)
		}
		msg := strings.ToLower(err.Error())
		if !strings.Contains(msg, "denied") && !strings.Contains(msg, "not allowed") {
			t.Fatalf("expected policy denial for %s, got %v", script, err)
		}
	}
}

func TestExecUntrustedDeniesFilesystemEscapeAndGlobEnumeration(t *testing.T) {
	res, err := ExecUntrustedWithOptions(`let _, err = read_file("/etc/passwd"); err;`, nil, UntrustedExecOptions{InProcess: true})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	msg, ok := res.(*String)
	if !ok || !strings.Contains(strings.ToLower(msg.Value), "outside") {
		t.Fatalf("expected filesystem escape denial, got %T %#v", res, res)
	}

	res, err = ExecUntrustedWithOptions(`let _, err = glob("/*"); err;`, nil, UntrustedExecOptions{InProcess: true})
	if err != nil {
		t.Fatalf("unexpected top-level error: %v", err)
	}
	msg, ok = res.(*String)
	if !ok || !strings.Contains(strings.ToLower(msg.Value), "outside") {
		t.Fatalf("expected glob escape denial, got %T %#v", res, res)
	}
}

func TestExecUntrustedRuntimeLimits(t *testing.T) {
	_, err := ExecUntrustedWithOptions(`while (true) { }`, nil, UntrustedExecOptions{
		InProcess: true,
		MaxSteps:  1000,
		Timeout:   500 * time.Millisecond,
	})
	if err == nil || !strings.Contains(strings.ToLower(err.Error()), "step limit") {
		t.Fatalf("expected step limit error, got %v", err)
	}
}

func TestRunUntrustedWorkerProtocol(t *testing.T) {
	req := map[string]any{
		"script": "40 + 2;",
		"options": map[string]any{
			"inprocess": true,
		},
	}
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := RunUntrustedWorker(bytes.NewReader(payload), &out)
	if code != 0 {
		t.Fatalf("worker returned %d: %s", code, out.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid worker json: %v\n%s", err, out.String())
	}
	if got := resp["result"]; got != float64(42) {
		t.Fatalf("unexpected worker result: %#v", resp)
	}
}

func TestRunUntrustedWorkerPreservesRenderArtifactProtocol(t *testing.T) {
	req := map[string]any{
		"script": `render("<html><body>Hello</body></html>", {"name": "hello.html", "alt": "Greeting"});`,
		"options": map[string]any{
			"inprocess": true,
		},
	}
	payload, err := json.Marshal(req)
	if err != nil {
		t.Fatal(err)
	}
	var out bytes.Buffer
	code := RunUntrustedWorker(bytes.NewReader(payload), &out)
	if code != 0 {
		t.Fatalf("worker returned %d: %s", code, out.String())
	}
	var resp map[string]any
	if err := json.Unmarshal(out.Bytes(), &resp); err != nil {
		t.Fatalf("invalid worker json: %v\n%s", err, out.String())
	}
	result, ok := resp["result"].(map[string]any)
	if !ok {
		t.Fatalf("expected structured render artifact, got %#v", resp["result"])
	}
	if result["__spl_type"] != "render_artifact" || result["kind"] != "html" || result["source_type"] != "data" || result["mime"] != "text/html" {
		t.Fatalf("unexpected render artifact protocol result: %#v", result)
	}
	if result["name"] != "hello.html" || result["alt"] != "Greeting" {
		t.Fatalf("missing render artifact metadata: %#v", result)
	}
}

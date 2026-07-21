package ide

import (
	"context"
	"net"
	"path/filepath"
	"strconv"
	"testing"
	"time"
)

// testRunnerConfig builds a RunnerConfig that compiles the real
// cmd/interpreter from this checkout - the same one the playground uses in
// production - so these tests exercise the actual Start/Stop/Restart
// lifecycle against a real SPL process, not a mock. cmd/interpreter is its
// own Go module, so RepoRoot must point at its own directory with
// BuildPackage ".", not the bare repo root with "./cmd/interpreter" (which
// would cross a module boundary).
func testRunnerConfig(t *testing.T, cacheDir string) RunnerConfig {
	t.Helper()
	root, err := DetectRepoRoot()
	if err != nil {
		t.Skipf("cannot detect repo root (likely a -trimpath build): %v", err)
	}
	return RunnerConfig{
		Variant:      "test",
		RepoRoot:     filepath.Join(root, "cmd", "interpreter"),
		BuildPackage: ".",
		CacheDir:     cacheDir,
		GraceStop:    2 * time.Second,
		StartupProbe: 20 * time.Second,
	}
}

func TestProcessManagerStartStopListenLifecycle(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process lifecycle test in -short mode (builds a real binary)")
	}
	workspace := t.TempDir()
	reg, err := NewRegistry(workspace)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	cfg := testRunnerConfig(t, t.TempDir())
	pm := NewProcessManager(cfg, reg)

	project, err := reg.Create("Lifecycle", ScaffoldMinimal, func(dir string) error {
		return Scaffold(ScaffoldMinimal, dir, "lifecycle")
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	status, err := pm.Start(project)
	if err != nil {
		t.Fatalf("Start: %v", err)
	}
	if status.State != StateStarting && status.State != StateRunning {
		t.Fatalf("expected starting/running right after Start, got %s", status.State)
	}
	if status.Port == 0 {
		t.Fatalf("expected a port to be assigned")
	}

	if !waitForState(pm, project.ID, StateRunning, 20*time.Second) {
		st := pm.Status(project.ID)
		t.Fatalf("process never reached StateRunning, last status=%+v", st)
	}

	running := pm.Status(project.ID)
	if !dialOK(running.Port, 2*time.Second) {
		t.Fatalf("expected assigned port %d to accept connections while running", running.Port)
	}

	backlog, ch, cancel := pm.Subscribe(project.ID)
	if len(backlog) == 0 {
		t.Fatalf("expected some backlogged log lines (main.spl prints on boot)")
	}
	cancel()
	_ = ch

	if _, err := pm.Stop(project.ID); err != nil {
		t.Fatalf("Stop: %v", err)
	}
	if !waitForState(pm, project.ID, StateStopped, 5*time.Second) {
		t.Fatalf("process never reached StateStopped after Stop")
	}
	if dialOK(running.Port, 500*time.Millisecond) {
		t.Fatalf("expected port %d to be released after Stop", running.Port)
	}

	status, err = pm.Restart(project)
	if err != nil {
		t.Fatalf("Restart: %v", err)
	}
	if !waitForState(pm, project.ID, StateRunning, 20*time.Second) {
		t.Fatalf("process never reached StateRunning after Restart, last=%+v", pm.Status(project.ID))
	}
	restarted := pm.Status(project.ID)
	if !dialOK(restarted.Port, 2*time.Second) {
		t.Fatalf("expected restarted port %d to accept connections", restarted.Port)
	}

	ctx, cancelCtx := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancelCtx()
	pm.StopAll(ctx)
	if !waitForState(pm, project.ID, StateStopped, 5*time.Second) {
		t.Fatalf("expected StopAll to stop the running project")
	}
}

func TestProcessManagerStartTwiceIsRejected(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping process lifecycle test in -short mode (builds a real binary)")
	}
	workspace := t.TempDir()
	reg, err := NewRegistry(workspace)
	if err != nil {
		t.Fatalf("NewRegistry: %v", err)
	}
	cfg := testRunnerConfig(t, t.TempDir())
	pm := NewProcessManager(cfg, reg)

	project, err := reg.Create("DoubleStart", ScaffoldMinimal, func(dir string) error {
		return Scaffold(ScaffoldMinimal, dir, "double-start")
	})
	if err != nil {
		t.Fatalf("Create: %v", err)
	}

	if _, err := pm.Start(project); err != nil {
		t.Fatalf("first Start: %v", err)
	}
	defer func() {
		ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		pm.StopAll(ctx)
	}()

	if _, err := pm.Start(project); err == nil {
		t.Fatalf("expected second concurrent Start to be rejected")
	}
}

func waitForState(pm *ProcessManager, projectID string, want ProcessState, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if pm.Status(projectID).State == want {
			return true
		}
		time.Sleep(100 * time.Millisecond)
	}
	return false
}

func dialOK(port int, timeout time.Duration) bool {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), timeout)
	if err != nil {
		return false
	}
	_ = conn.Close()
	return true
}

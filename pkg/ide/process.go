package ide

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"
)

// ProcessState is the lifecycle state of one project's managed subprocess.
type ProcessState string

const (
	StateIdle     ProcessState = "idle"
	StateStarting ProcessState = "starting"
	StateRunning  ProcessState = "running"
	StateStopping ProcessState = "stopping"
	StateStopped  ProcessState = "stopped"
	StateCrashed  ProcessState = "crashed"
)

// LogLine is one captured stdout/stderr line from a managed subprocess.
type LogLine struct {
	Stream string    `json:"stream"`
	Line   string    `json:"line"`
	Time   time.Time `json:"ts"`
}

const ringBufferCapacity = 4000

// ringBuffer is a bounded, mutex-guarded log line buffer used both to
// answer "give me the recent backlog" on a new SSE subscriber and, via
// subs, to fan new lines out to all currently-connected subscribers.
type ringBuffer struct {
	mu   sync.Mutex
	buf  []LogLine
	subs map[chan LogLine]struct{}
}

func newRingBuffer() *ringBuffer {
	return &ringBuffer{subs: make(map[chan LogLine]struct{})}
}

func (b *ringBuffer) add(l LogLine) {
	b.mu.Lock()
	b.buf = append(b.buf, l)
	if len(b.buf) > ringBufferCapacity {
		b.buf = b.buf[len(b.buf)-ringBufferCapacity:]
	}
	for ch := range b.subs {
		select {
		case ch <- l:
		default:
			// Slow subscriber: drop the line rather than blocking the
			// process's stdout/stderr reader goroutine.
		}
	}
	b.mu.Unlock()
}

// subscribe returns the current backlog plus a channel that receives new
// lines going forward, and a cancel func to unregister the channel.
func (b *ringBuffer) subscribe() ([]LogLine, chan LogLine, func()) {
	b.mu.Lock()
	backlog := make([]LogLine, len(b.buf))
	copy(backlog, b.buf)
	ch := make(chan LogLine, 256)
	b.subs[ch] = struct{}{}
	b.mu.Unlock()
	cancel := func() {
		b.mu.Lock()
		delete(b.subs, ch)
		b.mu.Unlock()
	}
	return backlog, ch, cancel
}

// ManagedProcess is one project's subprocess and its captured logs. Every
// project runs as its own OS process (never in-process via pkg/eval): the
// interpreter's StartCLI/listen() shutdown-hook registry is a process-wide
// global (see object.RegisterShutdownHook), so two projects' servers
// sharing one Go process would corrupt each other's graceful shutdown.
type ManagedProcess struct {
	mu        sync.Mutex
	projectID string
	cmd       *exec.Cmd
	state     ProcessState
	port      int
	pid       int
	startedAt time.Time
	exitErr   string
	logs      *ringBuffer
}

// ProcessStatus is the JSON-friendly snapshot returned by the status/start/
// stop/restart HTTP endpoints.
type ProcessStatus struct {
	State         ProcessState `json:"state"`
	Port          int          `json:"port,omitempty"`
	PID           int          `json:"pid,omitempty"`
	StartedAt     *time.Time   `json:"started_at,omitempty"`
	UptimeMS      int64        `json:"uptime_ms,omitempty"`
	LastExitError string       `json:"last_exit_error,omitempty"`
}

func (p *ManagedProcess) status() ProcessStatus {
	p.mu.Lock()
	defer p.mu.Unlock()
	st := ProcessStatus{State: p.state, Port: p.port, PID: p.pid, LastExitError: p.exitErr}
	if !p.startedAt.IsZero() {
		t := p.startedAt
		st.StartedAt = &t
		if p.state == StateRunning || p.state == StateStopping {
			st.UptimeMS = time.Since(p.startedAt).Milliseconds()
		}
	}
	return st
}

// RunnerConfig parameterizes ProcessManager: which interpreter binary to
// run projects with and which scaffold kind those projects default to.
//
// Projects are plain SPL scaffolds with no go.mod of their own, and the
// interpreter's own relative-path config resolution (VIEWS_DIR, PUBLIC_DIR,
// DB_PATH, ...) requires the child process's OS working directory to be
// the project directory - so `go run <import path>` with cmd.Dir set to
// the project dir does not work (Go's module resolution needs a go.mod at
// or above the process's cwd, which a scaffolded project doesn't have).
// Instead, ProcessManager builds a real binary once (via `go build`, run
// with cmd.Dir set to the interpreter repo) and caches it, then execs that
// binary directly with cmd.Dir set to the project directory.
type RunnerConfig struct {
	// Variant is a human-readable label (e.g. "full").
	Variant string
	// BinaryPath, if set, is used directly instead of building - an escape
	// hatch for deployments that ship a prebuilt cmd/interpreter.
	BinaryPath string
	// RepoRoot is used as cmd.Dir for the one-time `go build`. cmd/interpreter
	// is its own Go module, so this should point at its directory (e.g.
	// filepath.Join(DetectRepoRoot(), "cmd", "interpreter")) with
	// BuildPackage ".", not the bare repo root with "./cmd/interpreter". If
	// empty, it's auto-detected from this package's own compiled source
	// location (the bare repo root - only correct if BuildPackage is a
	// root-module package).
	RepoRoot string
	// BuildPackage is the package to build, relative to RepoRoot, e.g. "."
	// when RepoRoot already points at cmd/interpreter.
	BuildPackage string
	// CacheDir is where the built binary is placed. If empty, defaults to
	// <workspaceRoot>/.spl-ide/bin.
	CacheDir string
	// ScaffoldKind is the scaffold this binary's "create project" defaults
	// to (it may still allow the other kind explicitly).
	ScaffoldKind string
	// GraceStop is how long Stop waits after SIGTERM before SIGKILL.
	GraceStop time.Duration
	// StartupProbe is how long Start waits for the assigned port to accept
	// a connection before giving up and reporting StateCrashed.
	StartupProbe time.Duration
}

func (c RunnerConfig) graceStop() time.Duration {
	if c.GraceStop > 0 {
		return c.GraceStop
	}
	return 5 * time.Second
}

func (c RunnerConfig) startupProbe() time.Duration {
	if c.StartupProbe > 0 {
		return c.StartupProbe
	}
	return 10 * time.Second
}

// ProcessManager starts/stops/restarts one subprocess per project and
// fans out its logs to SSE subscribers.
type ProcessManager struct {
	mu        sync.Mutex
	cfg       RunnerConfig
	registry  *Registry
	processes map[string]*ManagedProcess

	binOnce sync.Once
	binPath string
	binErr  error
}

func NewProcessManager(cfg RunnerConfig, registry *Registry) *ProcessManager {
	return &ProcessManager{cfg: cfg, registry: registry, processes: make(map[string]*ManagedProcess)}
}

// resolveBinary returns the interpreter binary to exec, building it once
// (via `go build`, cached thereafter) unless cfg.BinaryPath overrides it.
func (m *ProcessManager) resolveBinary() (string, error) {
	m.binOnce.Do(func() {
		if strings.TrimSpace(m.cfg.BinaryPath) != "" {
			if _, err := os.Stat(m.cfg.BinaryPath); err != nil {
				m.binErr = fmt.Errorf("configured interpreter binary %q: %w", m.cfg.BinaryPath, err)
				return
			}
			m.binPath = m.cfg.BinaryPath
			return
		}
		repoRoot := m.cfg.RepoRoot
		if repoRoot == "" {
			root, err := DetectRepoRoot()
			if err != nil {
				m.binErr = fmt.Errorf("detect interpreter repo root: %w", err)
				return
			}
			repoRoot = root
		}
		cacheDir := m.cfg.CacheDir
		if cacheDir == "" {
			cacheDir = filepath.Join(m.registry.Root(), ".spl-ide", "bin")
		}
		if err := os.MkdirAll(cacheDir, 0o755); err != nil {
			m.binErr = fmt.Errorf("create binary cache dir: %w", err)
			return
		}
		binName := "interpreter-" + m.cfg.Variant
		if runtime.GOOS == "windows" {
			binName += ".exe"
		}
		out := filepath.Join(cacheDir, binName)
		buildCmd := exec.Command("go", "build", "-o", out, m.cfg.BuildPackage)
		buildCmd.Dir = repoRoot
		buildCmd.Env = os.Environ()
		var buildOutput strings.Builder
		buildCmd.Stdout = &buildOutput
		buildCmd.Stderr = &buildOutput
		if err := buildCmd.Run(); err != nil {
			m.binErr = fmt.Errorf("build %s: %w\n%s", m.cfg.BuildPackage, err, buildOutput.String())
			return
		}
		m.binPath = out
	})
	return m.binPath, m.binErr
}

func (m *ProcessManager) managed(projectID string) *ManagedProcess {
	m.mu.Lock()
	defer m.mu.Unlock()
	mp, ok := m.processes[projectID]
	if !ok {
		mp = &ManagedProcess{projectID: projectID, state: StateIdle, logs: newRingBuffer()}
		m.processes[projectID] = mp
	}
	return mp
}

// Status returns the current status for a project, StateIdle if it has
// never been started.
func (m *ProcessManager) Status(projectID string) ProcessStatus {
	return m.managed(projectID).status()
}

// Subscribe returns the log backlog plus a live channel for a project.
func (m *ProcessManager) Subscribe(projectID string) ([]LogLine, chan LogLine, func()) {
	return m.managed(projectID).logs.subscribe()
}

func freePort() (int, error) {
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer l.Close()
	return l.Addr().(*net.TCPAddr).Port, nil
}

// rewriteEnvPort sets (or inserts) APP_PORT=<port> in the project's .env
// file. config_load(".env","env") only ever reads the file, never OS env
// vars, so this is how the IDE hands the child process its assigned port
// without any interpreter-side changes.
func rewriteEnvPort(envPath string, port int) error {
	existing, err := os.ReadFile(envPath)
	if err != nil && !os.IsNotExist(err) {
		return err
	}
	lines := strings.Split(string(existing), "\n")
	found := false
	for i, line := range lines {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "APP_PORT=") || strings.HasPrefix(trimmed, "APP_PORT ") {
			lines[i] = fmt.Sprintf("APP_PORT=%d", port)
			found = true
			break
		}
	}
	if !found {
		if len(lines) > 0 && strings.TrimSpace(lines[len(lines)-1]) == "" {
			lines[len(lines)-1] = fmt.Sprintf("APP_PORT=%d", port)
		} else {
			lines = append(lines, fmt.Sprintf("APP_PORT=%d", port))
		}
	}
	return os.WriteFile(envPath, []byte(strings.Join(lines, "\n")), 0o644)
}

// Start launches project's main.spl as a subprocess. If already running,
// returns an error - call Restart instead.
func (m *ProcessManager) Start(project *Project) (ProcessStatus, error) {
	mp := m.managed(project.ID)
	mp.mu.Lock()
	if mp.state == StateStarting || mp.state == StateRunning {
		st := mp.status0()
		mp.mu.Unlock()
		return st, fmt.Errorf("project is already %s", mp.state)
	}
	mp.state = StateStarting
	mp.exitErr = ""
	mp.mu.Unlock()

	projectDir := m.registry.ProjectDir(project)
	port, err := freePort()
	if err != nil {
		mp.setState(StateCrashed, "")
		return mp.status(), fmt.Errorf("allocate port: %w", err)
	}
	if err := rewriteEnvPort(filepath.Join(projectDir, ".env"), port); err != nil {
		mp.setState(StateCrashed, "")
		return mp.status(), fmt.Errorf("write .env: %w", err)
	}
	_ = m.registry.UpdatePort(project.ID, port)

	binPath, err := m.resolveBinary()
	if err != nil {
		mp.setState(StateCrashed, err.Error())
		return mp.status(), fmt.Errorf("resolve interpreter binary: %w", err)
	}
	cmd := exec.Command(binPath, "main.spl")
	cmd.Dir = projectDir
	cmd.Env = os.Environ()
	if runtime.GOOS != "windows" {
		cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	}

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		mp.setState(StateCrashed, "")
		return mp.status(), err
	}
	stderr, err := cmd.StderrPipe()
	if err != nil {
		mp.setState(StateCrashed, "")
		return mp.status(), err
	}

	if err := cmd.Start(); err != nil {
		mp.setState(StateCrashed, err.Error())
		return mp.status(), fmt.Errorf("start process: %w", err)
	}

	mp.mu.Lock()
	mp.cmd = cmd
	mp.port = port
	mp.pid = cmd.Process.Pid
	mp.startedAt = time.Now()
	mp.mu.Unlock()

	go streamLines(mp.logs, "stdout", stdout)
	go streamLines(mp.logs, "stderr", stderr)
	go func() {
		waitErr := cmd.Wait()
		mp.mu.Lock()
		if mp.state == StateStopping {
			mp.state = StateStopped
		} else if waitErr != nil {
			mp.state = StateCrashed
			mp.exitErr = waitErr.Error()
		} else {
			mp.state = StateStopped
		}
		mp.mu.Unlock()
	}()

	// Probe for the listening port in the background so Start() returns
	// immediately (StateStarting) rather than blocking the HTTP request
	// for up to startupProbe()'s duration - the caller polls /run/status
	// (or watches the log stream) to see the StateStarting -> StateRunning
	// transition once the child actually starts accepting connections.
	go func() {
		if waitForPort("127.0.0.1", port, m.cfg.startupProbe()) {
			mp.mu.Lock()
			if mp.state == StateStarting {
				mp.state = StateRunning
			}
			mp.mu.Unlock()
		}
	}()

	return mp.status(), nil
}

func (mp *ManagedProcess) status0() ProcessStatus {
	// Caller already holds mp.mu; build the snapshot without re-locking.
	st := ProcessStatus{State: mp.state, Port: mp.port, PID: mp.pid, LastExitError: mp.exitErr}
	if !mp.startedAt.IsZero() {
		t := mp.startedAt
		st.StartedAt = &t
		st.UptimeMS = time.Since(mp.startedAt).Milliseconds()
	}
	return st
}

func (mp *ManagedProcess) setState(s ProcessState, exitErr string) {
	mp.mu.Lock()
	mp.state = s
	if exitErr != "" {
		mp.exitErr = exitErr
	}
	mp.mu.Unlock()
}

func waitForPort(host string, port int, timeout time.Duration) bool {
	deadline := time.Now().Add(timeout)
	addr := net.JoinHostPort(host, strconv.Itoa(port))
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			_ = conn.Close()
			return true
		}
		time.Sleep(150 * time.Millisecond)
	}
	return false
}

func streamLines(logs *ringBuffer, stream string, r io.Reader) {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 64*1024), 1024*1024)
	for scanner.Scan() {
		logs.add(LogLine{Stream: stream, Line: scanner.Text(), Time: time.Now()})
	}
}

// Stop sends SIGTERM to the process group, waits up to the configured
// grace period, then SIGKILLs it. Mirrors killProcessGroup's pattern
// elsewhere in this codebase (untrusted.go), adapted from a one-shot
// worker's hard timeout to a user-triggered graceful-then-forced stop.
func (m *ProcessManager) Stop(projectID string) (ProcessStatus, error) {
	mp := m.managed(projectID)
	mp.mu.Lock()
	cmd := mp.cmd
	state := mp.state
	if state != StateRunning && state != StateStarting {
		st := mp.status0()
		mp.mu.Unlock()
		if state == StateIdle {
			return st, fmt.Errorf("project has not been started")
		}
		return st, nil // already stopped/crashed - idempotent
	}
	mp.state = StateStopping
	mp.mu.Unlock()

	if cmd == nil || cmd.Process == nil {
		mp.setState(StateStopped, "")
		return mp.status(), nil
	}

	signalProcessGroup(cmd.Process, syscall.SIGTERM)

	done := make(chan struct{})
	go func() {
		for range 100 {
			mp.mu.Lock()
			s := mp.state
			mp.mu.Unlock()
			if s == StateStopped || s == StateCrashed {
				close(done)
				return
			}
			time.Sleep(50 * time.Millisecond)
		}
		close(done)
	}()

	select {
	case <-done:
	case <-time.After(m.cfg.graceStop()):
	}

	mp.mu.Lock()
	stillRunning := mp.state == StateStopping
	mp.mu.Unlock()
	if stillRunning {
		killProcessGroup(cmd.Process)
		// cmd.Wait()'s goroutine (started in Start) will observe the kill
		// and transition state to Stopped/Crashed; give it a brief moment.
		time.Sleep(200 * time.Millisecond)
	}
	return mp.status(), nil
}

// Restart stops (if running) then starts a project.
func (m *ProcessManager) Restart(project *Project) (ProcessStatus, error) {
	mp := m.managed(project.ID)
	mp.mu.Lock()
	state := mp.state
	mp.mu.Unlock()
	if state == StateRunning || state == StateStarting {
		if _, err := m.Stop(project.ID); err != nil {
			return mp.status(), err
		}
	}
	return m.Start(project)
}

// StopAll stops every currently-running managed process, used when the
// playground server itself is shutting down so no orphaned child SPL
// processes survive it.
func (m *ProcessManager) StopAll(ctx context.Context) {
	m.mu.Lock()
	ids := make([]string, 0, len(m.processes))
	for id := range m.processes {
		ids = append(ids, id)
	}
	m.mu.Unlock()
	var wg sync.WaitGroup
	for _, id := range ids {
		wg.Go(func() {
			_, _ = m.Stop(id)
		})
	}
	done := make(chan struct{})
	go func() {
		wg.Wait()
		close(done)
	}()
	select {
	case <-done:
	case <-ctx.Done():
	}
}

func signalProcessGroup(p *os.Process, sig syscall.Signal) {
	if p == nil {
		return
	}
	if runtime.GOOS != "windows" {
		_ = syscall.Kill(-p.Pid, sig)
		return
	}
	_ = p.Signal(sig)
}

func killProcessGroup(p *os.Process) {
	if p == nil {
		return
	}
	if runtime.GOOS != "windows" {
		_ = syscall.Kill(-p.Pid, syscall.SIGKILL)
		return
	}
	_ = p.Kill()
}

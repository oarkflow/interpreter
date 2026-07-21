// Package playgroundserver is the SPL browser playground's HTTP server,
// started by cmd/interpreter's --playground mode (see runPlayground in
// cmd/interpreter/main.go). Variant exists so the small pieces that once
// differed between separate lightweight/full playground binaries (linked
// builtins, example scripts, extra untrusted-profile capabilities) stay
// expressible as a single struct rather than duplicated main.go bodies.
package playgroundserver

import (
	"context"
	"embed"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/ide"
	"github.com/oarkflow/interpreter/pkg/render"
)

//go:embed static/*
var staticFS embed.FS

// Variant is the small set of parameters a --playground caller supplies.
// Everything else - config loading, auth, rate limiting, metrics, HTTP
// handlers, middleware, and static asset serving - lives in this package.
type Variant struct {
	// Name labels the variant in logs (e.g. "full").
	Name string
	// ExtraCapabilities are additional untrusted-profile capabilities
	// granted on top of the shared baseline (filesystem_read, async,
	// scheduler, server) - e.g. cmd/interpreter's --playground mode adds
	// CapabilityDB and CapabilityNetwork.
	ExtraCapabilities []string
	// ExampleOverrides replaces specific keys in the shared base example
	// set (see examples.go) - used for scripts whose content depends on
	// which builtins are linked (image-values, query-builder).
	ExampleOverrides map[string]string
	// ExtraExamples adds example keys unique to this variant (e.g.
	// "pdf-tools", only meaningful when builtins/pdf is linked).
	ExtraExamples map[string]string
	// IDERunner configures the Projects-mode process manager (which
	// interpreter binary to build/run projects with, and the default
	// scaffold kind) - see pkg/ide.RunnerConfig.
	IDERunner ide.RunnerConfig
}

type executeRequest struct {
	Code            string   `json:"code"`
	RenderMode      string   `json:"render_mode,omitempty"`
	RenderAllowURLs bool     `json:"render_allow_urls,omitempty"`
	RenderURLHosts  []string `json:"render_url_hosts,omitempty"`
	RenderMaxBytes  int64    `json:"render_max_bytes,omitempty"`
}

type executeResponse struct {
	Output      string                    `json:"output"`
	Result      string                    `json:"result"`
	ResultType  string                    `json:"result_type"`
	Error       string                    `json:"error"`
	ErrorKind   string                    `json:"error_kind"`
	Diagnostics []string                  `json:"diagnostics,omitempty"`
	Artifacts   []render.ResolvedArtifact `json:"artifacts,omitempty"`
	DurationMS  int64                     `json:"duration_ms"`
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

// Flush forwards to the underlying ResponseWriter's Flusher, if any.
// Wrapping http.ResponseWriter in this struct otherwise silently drops the
// Flush method (it isn't part of the http.ResponseWriter interface, so it
// isn't promoted by embedding), which would break SSE streaming endpoints
// such as the IDE's /api/projects/{id}/logs/stream.
func (w *statusWriter) Flush() {
	if f, ok := w.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// Run boots the playground HTTP server for the given Variant and blocks
// until it exits (on signal, via graceful shutdown) or fails to start (via
// os.Exit, matching the behavior main() had before this package existed).
func Run(v Variant) {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("invalid configuration", slog.String("error", err.Error()))
		os.Exit(2)
	}
	if err := applyCLIFlags(&cfg, os.Args[1:]); err != nil {
		logger.Error("invalid CLI flags", slog.String("error", err.Error()))
		os.Exit(2)
	}

	rl := newRateLimiter(cfg.RateLimit, cfg.RateWindow)
	loginRl := newRateLimiter(10, 5*time.Minute)
	var auth *authManager
	if cfg.AuthSecret != "" {
		auth = newAuthManager(cfg.AuthSecret, cfg.SessionTTL)
		go startAuthCleanup(auth, cfg.RateCleanup)
	}
	authEnabled := auth != nil
	metrics := newPlaygroundMetrics()
	go startRateLimiterCleanup(rl, cfg.RateCleanup)
	go startRateLimiterCleanup(loginRl, cfg.RateCleanup)
	mux := http.NewServeMux()

	mux.HandleFunc("/api/health", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()
		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		writeJSON(w, status, map[string]any{"ok": true, "service": "spl-playground"})
	})
	mux.HandleFunc("/api/ready", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()
		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		writeJSON(w, status, map[string]any{"ok": true, "ready": true})
	})

	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()

		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		if !authEnabled {
			writeJSON(w, status, map[string]any{
				"authenticated":  true,
				"auth_enabled":   false,
				"session_ttl_ms": cfg.SessionTTL.Milliseconds(),
				"render": map[string]any{
					"mode":            cfg.RenderMode,
					"max_bytes":       cfg.RenderMaxBytes,
					"allow_urls":      cfg.RenderAllowURLs,
					"allow_url_hosts": cfg.RenderAllowURLHosts,
				},
			})
			return
		}
		token := tokenFromRequest(r)
		authed := auth.validate(token)
		metrics.recordAuth("session_check")
		if authed {
			metrics.setActiveSessions(auth.activeSessions())
		}
		writeJSON(w, status, map[string]any{
			"authenticated":  authed,
			"auth_enabled":   true,
			"session_ttl_ms": cfg.SessionTTL.Milliseconds(),
			"render": map[string]any{
				"mode":            cfg.RenderMode,
				"max_bytes":       cfg.RenderMaxBytes,
				"allow_urls":      cfg.RenderAllowURLs,
				"allow_url_hosts": cfg.RenderAllowURLHosts,
			},
		})
	})

	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()

		if r.Method != http.MethodPost {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
			status = http.StatusUnsupportedMediaType
			writeJSON(w, status, map[string]any{"error": "content type must be application/json"})
			return
		}
		clientID := clientKey(r, cfg.TrustProxy)
		if !loginRl.allow(clientID, time.Now()) {
			metrics.recordRateLimited()
			status = http.StatusTooManyRequests
			writeJSON(w, status, map[string]any{"error": "rate limit exceeded"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
		var req struct {
			Secret string `json:"secret"`
		}
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "invalid json payload"})
			return
		}
		if !auth.verifySecret(req.Secret) {
			metrics.recordAuth("login_failure")
			status = http.StatusUnauthorized
			writeJSON(w, status, map[string]any{"error": "unauthorized"})
			return
		}
		token, _, err := auth.issue()
		if err != nil {
			status = http.StatusInternalServerError
			writeJSON(w, status, map[string]any{"error": "failed to create session"})
			return
		}
		writeSessionCookie(w, token, cfg.CookieSecure || r.TLS != nil, cfg.SessionTTL)
		metrics.recordAuth("login_success")
		metrics.setActiveSessions(auth.activeSessions())
		writeJSON(w, status, map[string]any{
			"ok":             true,
			"authenticated":  true,
			"token":          token,
			"token_type":     "bearer",
			"session_ttl_ms": cfg.SessionTTL.Milliseconds(),
		})
	})

	mux.HandleFunc("/api/logout", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()

		if r.Method != http.MethodPost {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		token := tokenFromRequest(r)
		auth.revoke(token)
		clearSessionCookie(w, cfg.CookieSecure || r.TLS != nil)
		metrics.recordAuth("logout")
		metrics.setActiveSessions(auth.activeSessions())
		writeJSON(w, status, map[string]any{"ok": true})
	})

	mux.HandleFunc("/metrics", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()

		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			http.Error(w, "method not allowed", status)
			return
		}
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = w.Write([]byte(metrics.renderPrometheus()))
	})

	mux.HandleFunc("/api/examples", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()
		if r.Method != http.MethodGet {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		writeJSON(w, status, map[string]any{"examples": buildExamples(v)})
	})

	mux.HandleFunc("/api/execute", func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		status := http.StatusOK
		defer func() {
			metrics.recordRequest(r.URL.Path, r.Method, status, time.Since(start))
		}()
		if r.Method != http.MethodPost {
			status = http.StatusMethodNotAllowed
			writeJSON(w, status, map[string]any{"error": "method not allowed"})
			return
		}
		if !isAuthenticated(r, auth) {
			metrics.recordAuth("unauthorized")
			status = http.StatusUnauthorized
			writeJSON(w, status, map[string]any{"error": "unauthorized"})
			return
		}
		clientID := clientKey(r, cfg.TrustProxy)
		if !rl.allow(clientID, time.Now()) {
			metrics.recordRateLimited()
			status = http.StatusTooManyRequests
			writeJSON(w, status, map[string]any{"error": "rate limit exceeded"})
			return
		}

		if ct := strings.TrimSpace(r.Header.Get("Content-Type")); ct != "" && !strings.HasPrefix(strings.ToLower(ct), "application/json") {
			status = http.StatusUnsupportedMediaType
			writeJSON(w, status, map[string]any{"error": "content type must be application/json"})
			return
		}

		r.Body = http.MaxBytesReader(w, r.Body, cfg.MaxBodyBytes)
		var req executeRequest
		dec := json.NewDecoder(r.Body)
		dec.DisallowUnknownFields()
		if err := dec.Decode(&req); err != nil {
			if errors.Is(err, io.EOF) {
				status = http.StatusBadRequest
				writeJSON(w, status, map[string]any{"error": "request body is empty"})
				return
			}
			var maxErr *http.MaxBytesError
			if errors.As(err, &maxErr) {
				status = http.StatusRequestEntityTooLarge
				writeJSON(w, status, map[string]any{"error": "payload too large"})
				return
			}
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "invalid json payload"})
			return
		}
		var trailing any
		if err := dec.Decode(&trailing); err != io.EOF {
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "invalid json payload"})
			return
		}
		if strings.TrimSpace(req.Code) == "" {
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "code is required"})
			return
		}

		cwd, err := os.Getwd()
		if err != nil {
			status = http.StatusInternalServerError
			writeJSON(w, status, map[string]any{"error": "failed to resolve working directory"})
			return
		}
		execStart := time.Now()
		renderMode := cfg.RenderMode
		switch strings.ToLower(strings.TrimSpace(req.RenderMode)) {
		case "auto", "off", "metadata", "inline":
			renderMode = strings.ToLower(strings.TrimSpace(req.RenderMode))
		}
		renderMaxBytes := cfg.RenderMaxBytes
		if req.RenderMaxBytes > 0 && req.RenderMaxBytes < renderMaxBytes {
			renderMaxBytes = req.RenderMaxBytes
		}
		renderAllowURLs := cfg.RenderAllowURLs && req.RenderAllowURLs
		renderURLHosts := append([]string(nil), cfg.RenderAllowURLHosts...)
		if len(req.RenderURLHosts) > 0 {
			renderURLHosts = intersectHostPatterns(cfg.RenderAllowURLHosts, req.RenderURLHosts)
		}
		if renderAllowURLs && len(req.RenderURLHosts) > 0 && len(cfg.RenderAllowURLHosts) > 0 && len(renderURLHosts) == 0 {
			status = http.StatusBadRequest
			writeJSON(w, status, map[string]any{"error": "requested render URL hosts are not allowed by server configuration"})
			return
		}
		securityPolicy := securityPolicyForProfile(cwd, cfg.ExecutionProfile, renderAllowURLs, renderURLHosts, v.ExtraCapabilities)
		result := interpreter.EvalForPlayground(req.Code, interpreter.PlaygroundOptions{
			Args:      []string{},
			MaxDepth:  cfg.EvalMaxDepth,
			MaxSteps:  cfg.EvalMaxSteps,
			MaxHeapMB: cfg.EvalMaxHeapMB,
			TimeoutMS: cfg.EvalTimeoutMS,
			ModuleDir: cwd,
			Security:  securityPolicy,
			RenderConfig: &interpreter.RenderConfig{
				Mode:          renderMode,
				MaxBytes:      renderMaxBytes,
				AllowURLs:     renderAllowURLs,
				AllowURLHosts: renderURLHosts,
			},
		})

		if result.Error != "" {
			status = http.StatusBadRequest
		}
		writeJSON(w, status, executeResponse{
			Output:      result.Output,
			Result:      result.Result,
			ResultType:  result.ResultTy,
			Error:       result.Error,
			ErrorKind:   result.ErrorKind,
			Diagnostics: result.Diagnostics,
			Artifacts:   result.Artifacts,
			DurationMS:  result.Duration,
		})
		metrics.recordExecution("execute", time.Since(execStart))
	})

	// PLAYGROUND_INTERPRETER_BIN/PLAYGROUND_INTERPRETER_REPO_ROOT are a
	// generic operator-level override, applied on top of whatever the
	// Variant's own IDERunner already set (see ide.RunnerConfig.RepoRoot's
	// doc comment on why cmd/interpreter's default is not simply "").
	runnerCfg := v.IDERunner
	if bin := envString("PLAYGROUND_INTERPRETER_BIN", ""); bin != "" {
		runnerCfg.BinaryPath = bin
	}
	if root := envString("PLAYGROUND_INTERPRETER_REPO_ROOT", ""); root != "" {
		runnerCfg.RepoRoot = root
	}
	ideServer, err := ide.NewServer(runnerCfg, envString("PLAYGROUND_WORKSPACE_ROOT", "./workspace"), func(r *http.Request) bool {
		return isAuthenticated(r, auth)
	})
	if err != nil {
		logger.Error("failed to initialize IDE workspace", slog.String("error", err.Error()))
		os.Exit(2)
	}
	ideServer.Routes(mux)

	fileServer, err := fsSub()
	if err != nil {
		logger.Error("failed to load embedded static files", slog.String("error", err.Error()))
		os.Exit(2)
	}
	mux.Handle("/", fileServer)

	handler := withRecovery(logger, withSecurityHeaders(loggingMiddleware(logger, cfg.TrustProxy, mux)))
	server := &http.Server{
		Addr:         cfg.Addr,
		Handler:      handler,
		ReadTimeout:  cfg.ReadTimeout,
		WriteTimeout: cfg.WriteTimeout,
		IdleTimeout:  cfg.IdleTimeout,
	}

	ctx, stop := signalNotifyContext()
	defer stop()

	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
		defer cancel()
		// Stop any running IDE project subprocesses first so no orphaned
		// child SPL servers survive this process exiting.
		ideServer.Processes.StopAll(shutdownCtx)
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
		}
	}()

	logger.Info("SPL Playground running",
		slog.String("variant", v.Name),
		slog.String("addr", cfg.Addr),
		slog.Int64("max_body_bytes", cfg.MaxBodyBytes),
		slog.Int("rate_limit", cfg.RateLimit),
		slog.String("rate_window", cfg.RateWindow.String()),
		slog.Bool("trust_proxy_headers", cfg.TrustProxy),
		slog.Int("eval_max_depth", cfg.EvalMaxDepth),
		slog.Int64("eval_max_steps", cfg.EvalMaxSteps),
		slog.Int64("eval_max_heap_mb", cfg.EvalMaxHeapMB),
		slog.Int64("eval_timeout_ms", cfg.EvalTimeoutMS),
	)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Error("server terminated", slog.String("error", err.Error()))
		os.Exit(1)
	}
}

func signalNotifyContext() (context.Context, context.CancelFunc) {
	return signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
}

func isAuthenticated(r *http.Request, auth *authManager) bool {
	if auth == nil {
		return true // no auth configured – open access
	}
	token := tokenFromRequest(r)
	return auth.validate(token)
}

func clientKey(r *http.Request, trustProxy bool) string {
	if trustProxy {
		if ip := strings.TrimSpace(strings.Split(strings.TrimSpace(r.Header.Get("X-Forwarded-For")), ",")[0]); ip != "" {
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed.String()
			}
		}
		if ip := strings.TrimSpace(r.Header.Get("X-Real-IP")); ip != "" {
			if parsed := net.ParseIP(ip); parsed != nil {
				return parsed.String()
			}
		}
	}
	host, _, err := net.SplitHostPort(strings.TrimSpace(r.RemoteAddr))
	if err == nil && host != "" {
		return host
	}
	if strings.TrimSpace(r.RemoteAddr) != "" {
		return strings.TrimSpace(r.RemoteAddr)
	}
	return "unknown"
}

func loggingMiddleware(logger *slog.Logger, trustProxy bool, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		sw := &statusWriter{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(sw, r)
		logger.Info("request",
			slog.String("method", r.Method),
			slog.String("path", r.URL.Path),
			slog.Int("status", sw.status),
			slog.String("remote", clientKey(r, trustProxy)),
			slog.Int64("duration_ms", time.Since(start).Milliseconds()),
		)
	})
}

func withSecurityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "no-referrer")
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

func withRecovery(logger *slog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		defer func() {
			if rec := recover(); rec != nil {
				logger.Error("panic recovered", slog.Any("panic", rec), slog.String("path", r.URL.Path))
				writeJSON(w, http.StatusInternalServerError, map[string]any{"error": "internal server error"})
			}
		}()
		next.ServeHTTP(w, r)
	})
}

func fsSub() (http.Handler, error) {
	fsys, err := staticFS.ReadFile("static/index.html")
	if err != nil || len(fsys) == 0 {
		return nil, err
	}

	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		path := strings.TrimPrefix(r.URL.Path, "/")
		if path == "" {
			path = "index.html"
		}
		path = filepath.Clean(path)
		if strings.Contains(path, "..") {
			http.NotFound(w, r)
			return
		}
		if path == "index.html" {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(fsys)
			return
		}
		content, err := staticFS.ReadFile("static/" + path)
		if err != nil {
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
			_, _ = w.Write(fsys)
			return
		}
		switch {
		case strings.HasSuffix(path, ".js"):
			w.Header().Set("Content-Type", "application/javascript; charset=utf-8")
		case strings.HasSuffix(path, ".css"):
			w.Header().Set("Content-Type", "text/css; charset=utf-8")
		case strings.HasSuffix(path, ".html"):
			w.Header().Set("Content-Type", "text/html; charset=utf-8")
		}
		_, _ = w.Write(content)
	}), nil
}

func writeJSON(w http.ResponseWriter, code int, payload any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

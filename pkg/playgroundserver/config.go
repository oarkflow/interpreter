package playgroundserver

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

// Config is the playground's env/CLI-driven runtime configuration, used by
// cmd/interpreter's --playground mode. It lives here rather than in main.go
// so it stays testable independent of process startup.
type Config struct {
	Addr                string
	AuthSecret          string
	ExecutionProfile    string
	ReadTimeout         time.Duration
	WriteTimeout        time.Duration
	IdleTimeout         time.Duration
	ShutdownTimeout     time.Duration
	MaxBodyBytes        int64
	RateLimit           int
	RateWindow          time.Duration
	RateCleanup         time.Duration
	TrustProxy          bool
	CookieSecure        bool
	SessionTTL          time.Duration
	EvalMaxDepth        int
	EvalMaxSteps        int64
	EvalMaxHeapMB       int64
	EvalTimeoutMS       int64
	RenderAllowURLs     bool
	RenderAllowURLHosts []string
	RenderMode          string
	RenderMaxBytes      int64
	// DevMode is an explicit opt-out of the production-safety guard below.
	// When true, the server is allowed to bind to a non-loopback address
	// without an authentication secret and/or secure cookies configured -
	// intended only for local development, CI, and tests. It defaults to
	// false, so production-style deployments (bound to a non-loopback
	// address) must either configure auth + secure cookies or explicitly
	// set PLAYGROUND_DEV_MODE=true to acknowledge the reduced security.
	DevMode bool
}

// isLoopbackAddr reports whether addr (an Addr-style "host:port", bare host,
// or ":port" string as accepted by net/http.Server.Addr) resolves to a
// loopback-only bind (127.0.0.0/8, ::1, or the literal host "localhost").
// An empty host (e.g. ":8080") binds all interfaces and is NOT loopback.
func isLoopbackAddr(addr string) bool {
	host := addr
	if h, _, err := net.SplitHostPort(addr); err == nil {
		host = h
	}
	host = strings.TrimSpace(host)
	host = strings.Trim(host, "[]")
	if host == "" {
		return false
	}
	if strings.EqualFold(host, "localhost") {
		return true
	}
	if ip := net.ParseIP(host); ip != nil {
		return ip.IsLoopback()
	}
	return false
}

func loadConfig() (Config, error) {
	cfg := Config{
		Addr:                envString("PLAYGROUND_ADDR", ":8080"),
		AuthSecret:          envString("PLAYGROUND_AUTH_SECRET", envString("PLAYGROUND_API_KEY", "")),
		ExecutionProfile:    envString("PLAYGROUND_EXECUTION_PROFILE", "untrusted"),
		ReadTimeout:         envDurationMS("PLAYGROUND_READ_TIMEOUT_MS", 15000),
		WriteTimeout:        envDurationMS("PLAYGROUND_WRITE_TIMEOUT_MS", 15000),
		IdleTimeout:         envDurationMS("PLAYGROUND_IDLE_TIMEOUT_MS", 30000),
		ShutdownTimeout:     envDurationMS("PLAYGROUND_SHUTDOWN_TIMEOUT_MS", 10000),
		MaxBodyBytes:        envInt64("PLAYGROUND_MAX_BODY_BYTES", 1<<20),
		RateLimit:           envInt("PLAYGROUND_RATE_LIMIT", 60),
		RateWindow:          envDurationMS("PLAYGROUND_RATE_WINDOW_MS", 60000),
		RateCleanup:         envDurationMS("PLAYGROUND_RATE_CLEANUP_MS", 120000),
		TrustProxy:          envBool("PLAYGROUND_TRUST_PROXY_HEADERS", false),
		CookieSecure:        envBool("PLAYGROUND_COOKIE_SECURE", false),
		SessionTTL:          envDurationMS("PLAYGROUND_SESSION_TTL_MS", 12*60*60*1000),
		EvalMaxDepth:        envInt("PLAYGROUND_EVAL_MAX_DEPTH", 200),
		EvalMaxSteps:        envInt64("PLAYGROUND_EVAL_MAX_STEPS", 2_000_000),
		EvalMaxHeapMB:       envInt64("PLAYGROUND_EVAL_MAX_HEAP_MB", 256),
		EvalTimeoutMS:       envInt64("PLAYGROUND_EVAL_TIMEOUT_MS", 8_000),
		RenderAllowURLs:     envBool("PLAYGROUND_RENDER_ALLOW_URLS", false),
		RenderAllowURLHosts: envCSV("PLAYGROUND_RENDER_ALLOW_URL_HOSTS"),
		RenderMode:          envString("PLAYGROUND_RENDER_MODE", "auto"),
		RenderMaxBytes:      envInt64("PLAYGROUND_RENDER_MAX_BYTES", 1<<20),
		DevMode:             envBool("PLAYGROUND_DEV_MODE", false),
	}

	if cfg.MaxBodyBytes <= 0 {
		return Config{}, errors.New("PLAYGROUND_MAX_BODY_BYTES must be > 0")
	}
	switch strings.ToLower(strings.TrimSpace(cfg.ExecutionProfile)) {
	case "trusted", "untrusted":
		cfg.ExecutionProfile = strings.ToLower(strings.TrimSpace(cfg.ExecutionProfile))
	default:
		return Config{}, errors.New("PLAYGROUND_EXECUTION_PROFILE must be trusted or untrusted")
	}
	if cfg.RateLimit <= 0 {
		return Config{}, errors.New("PLAYGROUND_RATE_LIMIT must be > 0")
	}
	if cfg.RateWindow <= 0 {
		return Config{}, errors.New("PLAYGROUND_RATE_WINDOW_MS must be > 0")
	}
	if cfg.RateCleanup <= 0 {
		return Config{}, errors.New("PLAYGROUND_RATE_CLEANUP_MS must be > 0")
	}
	if cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 || cfg.ShutdownTimeout <= 0 {
		return Config{}, errors.New("timeout values must be > 0")
	}
	if cfg.EvalMaxDepth <= 0 || cfg.EvalMaxSteps <= 0 || cfg.EvalMaxHeapMB <= 0 || cfg.EvalTimeoutMS <= 0 {
		return Config{}, errors.New("playground eval limits must be > 0")
	}
	if cfg.RenderMaxBytes <= 0 {
		return Config{}, errors.New("PLAYGROUND_RENDER_MAX_BYTES must be > 0")
	}
	// AuthSecret is optional – when unset the playground runs without authentication,
	// EXCEPT that a non-loopback bind requires it (see the production-safety guard
	// below) unless DevMode explicitly opts out.
	if cfg.SessionTTL <= 0 {
		return Config{}, errors.New("PLAYGROUND_SESSION_TTL_MS must be > 0")
	}
	if err := validateProductionBind(cfg); err != nil {
		return Config{}, err
	}
	return cfg, nil
}

// validateProductionBind is a safe-by-default guard against accidentally
// exposing the playground server on a non-loopback address without any
// authentication or transport protection. It only applies when both:
//   - the configured Addr does not resolve to a loopback-only bind
//     (127.0.0.0/8, ::1, or "localhost"), and
//   - DevMode is not explicitly enabled (PLAYGROUND_DEV_MODE=true).
//
// When it applies, it requires both an authentication secret and secure
// cookies to be configured, since a non-loopback bind is reachable by other
// hosts and must not run wide open.
func validateProductionBind(cfg Config) error {
	if cfg.DevMode {
		return nil
	}
	if isLoopbackAddr(cfg.Addr) {
		return nil
	}
	var problems []string
	if cfg.AuthSecret == "" {
		problems = append(problems, "no authentication secret is configured (set PLAYGROUND_AUTH_SECRET or PLAYGROUND_API_KEY to a non-empty value)")
	}
	if !cfg.CookieSecure {
		problems = append(problems, "secure cookies are disabled (set PLAYGROUND_COOKIE_SECURE=true; this requires the server to be reached over HTTPS/TLS, e.g. behind a TLS-terminating proxy)")
	}
	if len(problems) == 0 {
		return nil
	}
	return fmt.Errorf(
		"refusing to start: playground server is configured to bind to non-loopback address %q, which is unsafe with the current settings: %s. "+
			"Fix by addressing the setting(s) above, by binding to a loopback address instead (PLAYGROUND_ADDR=127.0.0.1:8080), "+
			"or, for local development/CI only, by explicitly opting into the reduced-security mode with PLAYGROUND_DEV_MODE=true",
		cfg.Addr, strings.Join(problems, "; "),
	)
}

func applyCLIFlags(cfg *Config, args []string) error {
	if cfg == nil {
		return nil
	}
	fs := flag.NewFlagSet("playground", flag.ContinueOnError)
	fs.SetOutput(io.Discard)
	renderAllowURLs := fs.Bool("render-allow-urls", cfg.RenderAllowURLs, "allow URL artifacts to be fetched by the playground server")
	renderURLHosts := fs.String("render-url-hosts", strings.Join(cfg.RenderAllowURLHosts, ","), "comma-separated URL artifact host allowlist")
	renderMode := fs.String("render-mode", cfg.RenderMode, "render mode: auto, inline, metadata, or off")
	renderMaxBytes := fs.Int64("render-max-bytes", cfg.RenderMaxBytes, "maximum render artifact bytes")
	profile := fs.String("profile", cfg.ExecutionProfile, "execution profile: trusted or untrusted")
	if err := fs.Parse(args); err != nil {
		return err
	}
	mode := strings.ToLower(strings.TrimSpace(*renderMode))
	switch mode {
	case "auto", "inline", "metadata", "off":
		cfg.RenderMode = mode
	default:
		return fmt.Errorf("invalid --render-mode %q", *renderMode)
	}
	if *renderMaxBytes <= 0 {
		return fmt.Errorf("--render-max-bytes must be > 0")
	}
	switch strings.ToLower(strings.TrimSpace(*profile)) {
	case "trusted", "untrusted":
		cfg.ExecutionProfile = strings.ToLower(strings.TrimSpace(*profile))
	default:
		return fmt.Errorf("invalid --profile %q", *profile)
	}
	cfg.RenderMaxBytes = *renderMaxBytes
	cfg.RenderAllowURLs = *renderAllowURLs
	cfg.RenderAllowURLHosts = parseCSV(*renderURLHosts)
	return nil
}

func envString(name, fallback string) string {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	return v
}

func envInt(name string, fallback int) int {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return fallback
	}
	return n
}

func envInt64(name string, fallback int64) int64 {
	v := strings.TrimSpace(os.Getenv(name))
	if v == "" {
		return fallback
	}
	n, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return fallback
	}
	return n
}

func envDurationMS(name string, fallbackMS int64) time.Duration {
	ms := envInt64(name, fallbackMS)
	if ms <= 0 {
		ms = fallbackMS
	}
	return time.Duration(ms) * time.Millisecond
}

func envBool(name string, fallback bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return fallback
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return fallback
	}
}

func envCSV(name string) []string {
	return parseCSV(os.Getenv(name))
}

func parseCSV(raw string) []string {
	raw = strings.TrimSpace(raw)
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if part != "" {
			out = append(out, part)
		}
	}
	return out
}

func intersectHostPatterns(serverAllowed, requested []string) []string {
	if len(requested) == 0 {
		return nil
	}
	if len(serverAllowed) == 0 {
		return nil
	}
	out := make([]string, 0, len(requested))
	for _, req := range requested {
		req = strings.TrimSpace(req)
		if req == "" {
			continue
		}
		for _, allow := range serverAllowed {
			if strings.EqualFold(strings.TrimSpace(allow), req) {
				out = append(out, req)
				break
			}
		}
	}
	return out
}

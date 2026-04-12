package main

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
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/oarkflow/interpreter"
)

//go:embed static/*
var staticFS embed.FS

type executeRequest struct {
	Code string `json:"code"`
}

type executeResponse struct {
	Output      string   `json:"output"`
	Result      string   `json:"result"`
	ResultType  string   `json:"result_type"`
	Error       string   `json:"error"`
	ErrorKind   string   `json:"error_kind"`
	Diagnostics []string `json:"diagnostics,omitempty"`
	DurationMS  int64    `json:"duration_ms"`
}

func builtinCodeExamples() map[string]string {
	return map[string]string{
		"hello": `print "Hello SPL Playground";`,
		"functions": `let add = function(a, b) {
	return a + b;
};

let makeMultiplier = function(factor) {
	return function(value) {
		return value * factor;
	};
};

let double = makeMultiplier(2);
print add(20, 22);
print double(21);`,
		"formatting": `let payload = {"name": "spl", "ok": true, "count": 3};
print sprintf("name=%s type=%T val=%v", payload.name, payload, payload);
print interpolate("Hello {name}, items={count}", {"name": "Playground", "count": 3});`,
		"modules": `import "testdata/modules/math.spl" as math;
import {label} from "testdata/modules/math.spl";

print label;
print math.base + math.increment;`,
		"collections": `let nums = [1, 2, 3, 4, 5, 6];
let doubled = nums.map(function(x) { return x * 2; });
let evens = nums.filter(function(x) { return x % 2 == 0; });
let total = nums.reduce(function(acc, x) { return acc + x; }, 0);

print doubled;
print evens;
print total;
print {"first_even": evens[0], "count": len(nums)};`,
		"error-handling": `let payload = {"name": "spl", "count": 3};
print sprintf("payload=%v type=%T", payload, payload);

let recovered = try {
	throw "demo error";
} catch (e) {
	"caught: " + e;
};

print recovered;`,
		"loops": `// for loops with range
for (let i = 0; i < 5; i = i + 1) {
	print sprintf("i = %d", i);
}

// while loop
let count = 3;
while (count > 0) {
	print sprintf("countdown: %d", count);
	count = count - 1;
}
print "liftoff!";`,
		"math": `// Math constants (called as functions)
print sprintf("PI = %v", PI());
print sprintf("E  = %v", E());

// Trigonometry
print sprintf("sin(PI/2) = %v", sin(PI() / 2));
print sprintf("cos(0)    = %v", cos(0));
print sprintf("atan2(1,1)= %v", atan2(1, 1));

// Logarithms and powers
print sprintf("log(E)    = %v", log(E()));
print sprintf("log2(8)   = %v", log2(8));
print sprintf("log10(100)= %v", log10(100));
print sprintf("sqrt(144) = %v", sqrt(144));
print sprintf("pow(2,10) = %v", pow(2, 10));
print sprintf("hypot(3,4)= %v", hypot(3, 4));

// Utility
print sprintf("abs(-42)  = %v", abs(-42));
print sprintf("round(3.7)= %v", round(3.7));
print sprintf("min(5,2)  = %v", min(5, 2));
print sprintf("max(5,2)  = %v", max(5, 2));
print sprintf("clamp(15, 0, 10) = %v", clamp(15, 0, 10));`,
		"strings": `// Case transforms
print upper("hello spl");
print lower("HELLO SPL");
print title("hello world");
print snake_case("helloWorld");
print kebab_case("helloWorld");
print camel_case("hello_world");
print pascal_case("hello_world");

// Search and manipulation
let text = "SPL is a scripting language";
print starts_with(text, "SPL");
print ends_with(text, "language");
print index_of(text, "scripting");
print count_substr(text, "a");
print replace(text, "scripting", "template");
print substring(text, 0, 3);

// Trim and pad
print trim("  hello  ");
print pad_left("42", 6, "0");
print pad_right("hi", 8, ".");
print truncate(text, 15, "...");
print repeat("ha", 3);

// Regex
print regex_match("abc123", "^[a-z]+[0-9]+$");
print regex_replace("hello 2024 world 42", "[0-9]+", "#");`,
		"collections-advanced": `// Array helpers
let nums = [5, 3, 1, 4, 2];
print sprintf("first=%v last=%v", first(nums), last(nums));
print sprintf("rest=%v", rest(nums));
print sprintf("sorted=%v", sort(nums));
print sprintf("reversed=%v", reverse(nums));
print sprintf("sum=%v avg=%v", sum(nums), avg(nums));
print sprintf("slice(1,3)=%v", slice(nums, 1, 3));

let nested = [[1, 2], [3, [4, 5]]];
print sprintf("flatten=%v", flatten(nested));

let mixed = [0, "", null, "ok", 42, false];
print sprintf("compact=%v", compact(mixed));

// group_by groups array of hashes by a string key
let people = [
	{"name": "alice", "dept": "eng"},
	{"name": "bob", "dept": "sales"},
	{"name": "carol", "dept": "eng"},
	{"name": "dave", "dept": "sales"}
];
let byDept = group_by(people, "dept");
print sprintf("grouped=%v", byDept);

// Hash helpers
let user = {"name": "alice", "role": "admin", "active": true};
print sprintf("keys=%v", keys(user));
print sprintf("values=%v", values(user));
print sprintf("has role=%v", has_key(user, "role"));

let defaults = {"theme": "light", "lang": "en"};
let prefs = {"theme": "dark"};
print sprintf("merged=%v", merge(defaults, prefs));

// any/all check truthiness of elements
print sprintf("any truthy=%v", any([0, false, 1]));
print sprintf("all truthy=%v", all([1, true, "ok"]));

// Method-style callbacks on arrays
let scores = [85, 92, 78, 95];
let high = scores.filter(function(s) { return s > 90; });
let found = scores.find(function(s) { return s > 80; });
print sprintf("high scores=%v", high);
print sprintf("first >80=%v", found);`,
		"crypto": `// Hashing
let msg = "Hello SPL";
print sprintf("sha256=%s", hash("sha256", msg));
print sprintf("md5=%s", hash("md5", msg));

// HMAC (algo, key, data)
let secret = "my-secret-key";
print sprintf("hmac=%s", hmac("sha256", secret, msg));

// Hex encoding
let encoded = hex_encode("SPL");
print sprintf("hex_encode=%s", encoded);
print sprintf("hex_decode=%s", hex_decode(encoded));

// UUID
print sprintf("uuid=%s", uuid());

// Random generation (length, alphabet)
print sprintf("random_string=%s", random_string(16, "abcdefghijklmnopqrstuvwxyz0123456789"));

// URL encoding
let url = "hello world&foo=bar";
let urlEnc = url_encode(url);
print sprintf("url_encode=%s", urlEnc);
print sprintf("url_decode=%s", url_decode(urlEnc));

// JSON round-trip
let data = {"key": "value", "nums": [1, 2, 3]};
let json = json_encode(data);
print sprintf("json=%s", json);
print sprintf("decoded=%v", json_decode(json));`,
		"time": `// Current time
print sprintf("now=%v", now());
print sprintf("now_iso=%s", now_iso());
print sprintf("time_ms=%v", time_ms());

// Parsing and formatting
let stamp = iso_to_unix("2024-06-15T14:30:00Z");
print sprintf("unix=%v", stamp);
print sprintf("formatted=%s", format_time(stamp, "YYYY-MM-DD HH:mm:ss"));
print sprintf("back_to_iso=%s", unix_to_iso(stamp));

// Time arithmetic (unix, amount, unit)
let future = time_add(stamp, 1, "h");
print sprintf("+1 hour=%s", format_time(future, "HH:mm:ss"));

let diff = time_diff(future, stamp);
print sprintf("diff=%v seconds", diff);

// Day boundaries
let dayStart = start_of_day(stamp);
let dayEnd = end_of_day(stamp);
print sprintf("day_start=%s", format_time(dayStart, "HH:mm:ss"));
print sprintf("day_end=%s", format_time(dayEnd, "HH:mm:ss"));

// Add months
let nextMonth = add_months(stamp, 1);
print sprintf("next_month=%s", format_time(nextMonth, "YYYY-MM-DD"));`,
		"testing": `// Built-in assertion functions
assert_true(1 + 1 == 2);
assert_eq(len("hello"), 5);
assert_neq("foo", "bar");
assert_contains([1, 2, 3], 2);

// Test that errors are thrown correctly
assert_throws(function() {
	throw "expected error";
});

// More assertions
assert_eq(upper("hi"), "HI");
assert_eq(abs(-42), 42);
assert_true(starts_with("hello", "hel"));
assert_contains("hello world", "world");

// View test summary
let summary = test_summary();
print sprintf("total=%v passed=%v failed=%v", summary.total, summary.passed, summary.failed);`,
		"complete-tour": `// Complete SPL playground tour: modules, closures, loops, collections,
// formatting, JSON, crypto/time helpers, and structured error handling.
import "testdata/modules/math.spl" as math;
import {label} from "testdata/modules/math.spl";

print "=== Complete SPL Playground Tour ===";
print sprintf("module=%s total=%d", label, math.base + math.increment);

const RATE = 1.13;
let prices = [12, 18, 25, 31];

let makeScaler = function(multiplier) {
	return function(value) {
		return value * multiplier;
	};
};

let scale = makeScaler(3);
print sprintf("closure scale(7) = %d", scale(7));

let adjusted = prices.map(function(price) {
	return round(price * RATE);
});
let premium = adjusted.filter(function(price) {
	return price >= 20;
});
let total = premium.reduce(function(acc, price) {
	return acc + price;
}, 0);

print sprintf("adjusted=%v", adjusted);
print sprintf("premium=%v", premium);
print sprintf("total=%d", total);

let summary = {
	"module": label,
	"items": len(prices),
	"premium_count": len(premium),
	"total": total,
	"ok": true
};

let encoded = json_encode(summary);
let decoded = json_decode(encoded);
print sprintf("json total=%d items=%d", decoded.total, decoded.items);

let stamp = iso_to_unix("2020-01-01T00:00:00Z");
print sprintf("time=%s", format_time(stamp, "YYYY-MM-DD HH:mm:ss"));
print sprintf("sha256=%s", hash("sha256", encoded));

let finalMessage = try {
	if (decoded.total < 50) {
		throw "total too small";
	}
	interpolate("{name} processed {count} premium items", {"name": "SPL", "count": decoded.premium_count});
} catch (e) {
	"recovered: " + e;
};

print finalMessage;
print "=== Tour Complete ===";`,
	}
}

type playgroundConfig struct {
	Addr            string
	AuthSecret      string
	ReadTimeout     time.Duration
	WriteTimeout    time.Duration
	IdleTimeout     time.Duration
	ShutdownTimeout time.Duration
	MaxBodyBytes    int64
	RateLimit       int
	RateWindow      time.Duration
	RateCleanup     time.Duration
	TrustProxy      bool
	CookieSecure    bool
	SessionTTL      time.Duration
	EvalMaxDepth    int
	EvalMaxSteps    int64
	EvalMaxHeapMB   int64
	EvalTimeoutMS   int64
}

func loadConfig() (playgroundConfig, error) {
	cfg := playgroundConfig{
		Addr:            envString("PLAYGROUND_ADDR", ":8080"),
		AuthSecret:      envString("PLAYGROUND_AUTH_SECRET", envString("PLAYGROUND_API_KEY", "")),
		ReadTimeout:     envDurationMS("PLAYGROUND_READ_TIMEOUT_MS", 15000),
		WriteTimeout:    envDurationMS("PLAYGROUND_WRITE_TIMEOUT_MS", 15000),
		IdleTimeout:     envDurationMS("PLAYGROUND_IDLE_TIMEOUT_MS", 30000),
		ShutdownTimeout: envDurationMS("PLAYGROUND_SHUTDOWN_TIMEOUT_MS", 10000),
		MaxBodyBytes:    envInt64("PLAYGROUND_MAX_BODY_BYTES", 1<<20),
		RateLimit:       envInt("PLAYGROUND_RATE_LIMIT", 60),
		RateWindow:      envDurationMS("PLAYGROUND_RATE_WINDOW_MS", 60000),
		RateCleanup:     envDurationMS("PLAYGROUND_RATE_CLEANUP_MS", 120000),
		TrustProxy:      envBool("PLAYGROUND_TRUST_PROXY_HEADERS", false),
		CookieSecure:    envBool("PLAYGROUND_COOKIE_SECURE", false),
		SessionTTL:      envDurationMS("PLAYGROUND_SESSION_TTL_MS", 12*60*60*1000),
		EvalMaxDepth:    envInt("PLAYGROUND_EVAL_MAX_DEPTH", 200),
		EvalMaxSteps:    envInt64("PLAYGROUND_EVAL_MAX_STEPS", 2_000_000),
		EvalMaxHeapMB:   envInt64("PLAYGROUND_EVAL_MAX_HEAP_MB", 256),
		EvalTimeoutMS:   envInt64("PLAYGROUND_EVAL_TIMEOUT_MS", 8_000),
	}

	if cfg.MaxBodyBytes <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_MAX_BODY_BYTES must be > 0")
	}
	if cfg.RateLimit <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_RATE_LIMIT must be > 0")
	}
	if cfg.RateWindow <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_RATE_WINDOW_MS must be > 0")
	}
	if cfg.RateCleanup <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_RATE_CLEANUP_MS must be > 0")
	}
	if cfg.ReadTimeout <= 0 || cfg.WriteTimeout <= 0 || cfg.IdleTimeout <= 0 || cfg.ShutdownTimeout <= 0 {
		return playgroundConfig{}, errors.New("timeout values must be > 0")
	}
	if cfg.EvalMaxDepth <= 0 || cfg.EvalMaxSteps <= 0 || cfg.EvalMaxHeapMB <= 0 || cfg.EvalTimeoutMS <= 0 {
		return playgroundConfig{}, errors.New("playground eval limits must be > 0")
	}
	// AuthSecret is optional – when unset the playground runs without authentication.
	if cfg.SessionTTL <= 0 {
		return playgroundConfig{}, errors.New("PLAYGROUND_SESSION_TTL_MS must be > 0")
	}
	return cfg, nil
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

type rateLimiter struct {
	mu      sync.Mutex
	clients map[string]*clientCounter
	limit   int
	window  time.Duration
}

type clientCounter struct {
	count int
	reset time.Time
}

func newRateLimiter(limit int, window time.Duration) *rateLimiter {
	return &rateLimiter{clients: make(map[string]*clientCounter), limit: limit, window: window}
}

func (rl *rateLimiter) allow(key string, now time.Time) bool {
	rl.mu.Lock()
	defer rl.mu.Unlock()

	cc, ok := rl.clients[key]
	if !ok || now.After(cc.reset) {
		rl.clients[key] = &clientCounter{count: 1, reset: now.Add(rl.window)}
		return true
	}
	if cc.count >= rl.limit {
		return false
	}
	cc.count++
	return true
}

func (rl *rateLimiter) prune(now time.Time) {
	rl.mu.Lock()
	defer rl.mu.Unlock()
	for key, cc := range rl.clients {
		if now.After(cc.reset) {
			delete(rl.clients, key)
		}
	}
}

type statusWriter struct {
	http.ResponseWriter
	status int
}

func (w *statusWriter) WriteHeader(code int) {
	w.status = code
	w.ResponseWriter.WriteHeader(code)
}

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	cfg, err := loadConfig()
	if err != nil {
		logger.Error("invalid configuration", slog.String("error", err.Error()))
		os.Exit(2)
	}

	rl := newRateLimiter(cfg.RateLimit, cfg.RateWindow)
	var auth *authManager
	if cfg.AuthSecret != "" {
		auth = newAuthManager(cfg.AuthSecret, cfg.SessionTTL)
		go startAuthCleanup(auth, cfg.RateCleanup)
	}
	authEnabled := auth != nil
	metrics := newPlaygroundMetrics()
	go startRateLimiterCleanup(rl, cfg.RateCleanup)
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
		examples := builtinCodeExamples()
		writeJSON(w, status, map[string]any{"examples": examples})
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
		result := interpreter.EvalForPlayground(req.Code, interpreter.PlaygroundOptions{
			Args:      []string{},
			MaxDepth:  cfg.EvalMaxDepth,
			MaxSteps:  cfg.EvalMaxSteps,
			MaxHeapMB: cfg.EvalMaxHeapMB,
			TimeoutMS: cfg.EvalTimeoutMS,
			ModuleDir: cwd,
			Security: &interpreter.SecurityPolicy{
				ProtectHost: true,
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
			DurationMS:  result.Duration,
		})
		metrics.recordExecution("execute", time.Since(execStart))
	})

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
		if err := server.Shutdown(shutdownCtx); err != nil {
			logger.Error("graceful shutdown failed", slog.String("error", err.Error()))
		}
	}()

	logger.Info("SPL Playground running",
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
		return
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

func startRateLimiterCleanup(rl *rateLimiter, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for now := range ticker.C {
		rl.prune(now)
	}
}

func startAuthCleanup(auth *authManager, every time.Duration) {
	ticker := time.NewTicker(every)
	defer ticker.Stop()
	for now := range ticker.C {
		auth.cleanup(now)
	}
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

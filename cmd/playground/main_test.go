package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestLoadConfigWithoutSecret(t *testing.T) {
	t.Setenv("PLAYGROUND_AUTH_SECRET", "")
	t.Setenv("PLAYGROUND_API_KEY", "")
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("expected config to load without auth secret, got error: %v", err)
	}
	if cfg.AuthSecret != "" {
		t.Fatalf("expected empty auth secret, got %q", cfg.AuthSecret)
	}
}

func TestAuthManagerLoginFlow(t *testing.T) {
	auth := newAuthManager("secret", time.Minute)
	if !auth.verifySecret("secret") {
		t.Fatalf("expected secret to validate")
	}
	if auth.verifySecret("nope") {
		t.Fatalf("expected wrong secret to fail")
	}
	token, _, err := auth.issue()
	if err != nil {
		t.Fatalf("issue failed: %v", err)
	}
	if !auth.validate(token) {
		t.Fatalf("expected issued token to validate")
	}
	auth.revoke(token)
	if auth.validate(token) {
		t.Fatalf("expected revoked token to fail")
	}
}

func TestRateLimiter(t *testing.T) {
	rl := newRateLimiter(2, time.Minute)
	now := time.Now()
	if !rl.allow("c1", now) {
		t.Fatalf("first request should pass")
	}
	if !rl.allow("c1", now.Add(time.Second)) {
		t.Fatalf("second request should pass")
	}
	if rl.allow("c1", now.Add(2*time.Second)) {
		t.Fatalf("third request should be rejected")
	}
	if !rl.allow("c1", now.Add(2*time.Minute)) {
		t.Fatalf("request after reset window should pass")
	}
}

func TestMaxBodyLimit(t *testing.T) {
	limit := int64(8)
	largeBody := strings.NewReader(`{"code":"01234567890123456789"}`)
	req := httptest.NewRequest(http.MethodPost, "/api/execute", largeBody)
	w := httptest.NewRecorder()
	req.Body = http.MaxBytesReader(w, req.Body, limit)

	_, err := io.ReadAll(req.Body)
	if err == nil {
		t.Fatalf("expected body read error due to size limit")
	}
}

func TestClientKeyProxyAware(t *testing.T) {
	req := httptest.NewRequest(http.MethodGet, "/api/health", nil)
	req.RemoteAddr = "10.0.0.2:12345"
	req.Header.Set("X-Forwarded-For", "203.0.113.10, 10.0.0.2")

	if got := clientKey(req, false); got != "10.0.0.2" {
		t.Fatalf("expected remote addr host when trust proxy disabled, got %q", got)
	}
	if got := clientKey(req, true); got != "203.0.113.10" {
		t.Fatalf("expected forwarded ip when trust proxy enabled, got %q", got)
	}
}

func TestMetricsRender(t *testing.T) {
	metrics := newPlaygroundMetrics()
	metrics.recordRequest("/api/login", http.MethodPost, http.StatusOK, 25*time.Millisecond)
	metrics.recordExecution("execute", 40*time.Millisecond)
	metrics.recordAuth("login_success")
	metrics.setActiveSessions(2)

	out := metrics.renderPrometheus()
	if !strings.Contains(out, "spl_playground_http_requests_total") {
		t.Fatalf("expected request metric output, got %q", out)
	}
	if !strings.Contains(out, "spl_playground_execution_duration_seconds") {
		t.Fatalf("expected execution metric output, got %q", out)
	}
	if !strings.Contains(out, "spl_playground_sessions_active 2") {
		t.Fatalf("expected sessions gauge, got %q", out)
	}
}

func TestLoginSessionEndpoints(t *testing.T) {
	auth := newAuthManager("secret", time.Minute)
	cfg := playgroundConfig{AuthSecret: "secret", SessionTTL: time.Minute, MaxBodyBytes: 1024}

	mux := http.NewServeMux()
	mux.HandleFunc("/api/login", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		var req struct {
			Secret string `json:"secret"`
		}
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		if !auth.verifySecret(req.Secret) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		token, _, err := auth.issue()
		if err != nil {
			http.Error(w, "error", http.StatusInternalServerError)
			return
		}
		writeSessionCookie(w, token, false, cfg.SessionTTL)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	mux.HandleFunc("/api/session", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		if auth.validate(tokenFromRequest(r)) {
			_, _ = w.Write([]byte(`{"authenticated":true}`))
			return
		}
		_, _ = w.Write([]byte(`{"authenticated":false}`))
	})

	loginReq := httptest.NewRequest(http.MethodPost, "/api/login", bytes.NewBufferString(`{"secret":"secret"}`))
	loginReq.Header.Set("Content-Type", "application/json")
	loginRec := httptest.NewRecorder()
	mux.ServeHTTP(loginRec, loginReq)
	if loginRec.Code != http.StatusOK {
		t.Fatalf("expected login to succeed, got %d", loginRec.Code)
	}
	cookies := loginRec.Result().Cookies()
	if len(cookies) == 0 {
		t.Fatalf("expected session cookie")
	}

	sessionReq := httptest.NewRequest(http.MethodGet, "/api/session", nil)
	sessionReq.AddCookie(cookies[0])
	sessionRec := httptest.NewRecorder()
	mux.ServeHTTP(sessionRec, sessionReq)
	if sessionRec.Code != http.StatusOK {
		t.Fatalf("expected session check to succeed, got %d", sessionRec.Code)
	}
	if !strings.Contains(sessionRec.Body.String(), `"authenticated":true`) {
		t.Fatalf("expected authenticated session, got %s", sessionRec.Body.String())
	}
}

func TestIndexNoEmbeddedSecret(t *testing.T) {
	handler, err := fsSub()
	if err != nil {
		t.Fatalf("fsSub failed: %v", err)
	}
	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)
	body := rec.Body.String()
	if strings.Contains(strings.ToLower(body), "api-key") {
		t.Fatalf("expected no embedded api key in html")
	}
	if !strings.Contains(body, `sandbox="allow-scripts"`) {
		t.Fatalf("expected preview iframe to allow scripts for hydrated templates")
	}
}

func TestBuiltinCodeExamplesContainCompleteExamples(t *testing.T) {
	examples := builtinCodeExamples()
	for _, name := range []string{"hello", "functions", "formatting", "modules", "collections", "error-handling", "loops", "math", "strings", "collections-advanced", "crypto", "time", "testing", "complete-tour"} {
		content, ok := examples[name]
		if !ok {
			t.Fatalf("expected code example %q", name)
		}
		if strings.TrimSpace(content) == "" {
			t.Fatalf("expected code example %q to have content", name)
		}
	}
	if !strings.Contains(examples["complete-tour"], `json_encode(summary)`) {
		t.Fatalf("expected complete-tour example to include json roundtrip, got %q", examples["complete-tour"])
	}
	if !strings.Contains(examples["complete-tour"], `hash("sha256", encoded)`) {
		t.Fatalf("expected complete-tour example to include hashing, got %q", examples["complete-tour"])
	}
	if !strings.Contains(examples["collections"], `.reduce(`) {
		t.Fatalf("expected collections example to include reduce, got %q", examples["collections"])
	}
	if !strings.Contains(examples["collections"], `.filter(`) || !strings.Contains(examples["collections"], `.map(`) {
		t.Fatalf("expected collections example to include chained collection methods, got %q", examples["collections"])
	}
	if !strings.Contains(examples["functions"], `function add(a, b)`) {
		t.Fatalf("expected functions example to include named function declaration, got %q", examples["functions"])
	}
}

func TestExamplesAPIContainsCompleteCodeExamples(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/examples", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"examples": builtinCodeExamples(),
		})
	})
	req := httptest.NewRequest(http.MethodGet, "/api/examples", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	var payload struct {
		Examples map[string]string `json:"examples"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &payload); err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{"collections", "error-handling", "complete-tour"} {
		if strings.TrimSpace(payload.Examples[name]) == "" {
			t.Fatalf("expected API to include non-empty example %q", name)
		}
	}
}

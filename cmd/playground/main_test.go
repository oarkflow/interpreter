package main

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/oarkflow/interpreter/pkg/security"
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

func TestApplyCLIFlagsForRenderURLSettings(t *testing.T) {
	cfg, err := loadConfig()
	if err != nil {
		t.Fatalf("loadConfig: %v", err)
	}
	err = applyCLIFlags(&cfg, []string{
		"--render-allow-urls",
		"--render-url-hosts", "nikonrumors.com,cdn.example.com",
		"--render-mode", "inline",
		"--render-max-bytes", "2048",
	})
	if err != nil {
		t.Fatalf("applyCLIFlags: %v", err)
	}
	if !cfg.RenderAllowURLs {
		t.Fatalf("expected render URLs to be enabled")
	}
	if cfg.RenderMode != "inline" {
		t.Fatalf("expected inline render mode, got %q", cfg.RenderMode)
	}
	if cfg.RenderMaxBytes != 2048 {
		t.Fatalf("expected render max bytes 2048, got %d", cfg.RenderMaxBytes)
	}
	if got := strings.Join(cfg.RenderAllowURLHosts, ","); got != "nikonrumors.com,cdn.example.com" {
		t.Fatalf("unexpected render URL hosts: %q", got)
	}
}

func TestPlaygroundSecurityPolicyAllowsReadsButNotWrites(t *testing.T) {
	policy := playgroundSecurityPolicy("/workspace/project", true, []string{"example.com"})
	if !policy.ProtectHost {
		t.Fatalf("expected host protection to remain enabled")
	}
	if !security.ContainsToken(policy.AllowedCapabilities, security.CapabilityFilesystemRead) {
		t.Fatalf("expected filesystem_read to be allowed, got %#v", policy.AllowedCapabilities)
	}
	if security.ContainsToken(policy.AllowedCapabilities, security.CapabilityFilesystemWrite) {
		t.Fatalf("did not expect filesystem_write to be allowed, got %#v", policy.AllowedCapabilities)
	}
	if !security.ContainsToken(policy.AllowedCapabilities, security.CapabilityNetwork) {
		t.Fatalf("expected network to be allowed when URL rendering is enabled")
	}
	if got := strings.Join(policy.AllowedFileReadPaths, ","); got != "/workspace/project" {
		t.Fatalf("unexpected allowed read roots: %q", got)
	}
	if got := strings.Join(policy.AllowedNetworkHosts, ","); got != "example.com" {
		t.Fatalf("unexpected network hosts: %q", got)
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
	for _, name := range []string{"hello", "functions", "formatting", "artifacts", "file-values", "image-values", "json-csv-values", "write-ops", "modules", "collections", "error-handling", "loops", "math", "strings", "collections-advanced", "crypto", "time", "testing", "complete-tour"} {
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
	if !strings.Contains(examples["file-values"], `file_load("testdata/test_io.txt")`) {
		t.Fatalf("expected file-values example to demonstrate file_load, got %q", examples["file-values"])
	}
	if !strings.Contains(examples["image-values"], `image_resize(`) || !strings.Contains(examples["image-values"], `image_render(`) {
		t.Fatalf("expected image-values example to demonstrate image transforms, got %q", examples["image-values"])
	}
	if !strings.Contains(examples["json-csv-values"], `table_filter(`) || !strings.Contains(examples["json-csv-values"], `csv_decode(`) {
		t.Fatalf("expected json-csv-values example to demonstrate table helpers, got %q", examples["json-csv-values"])
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
	for _, name := range []string{"collections", "error-handling", "complete-tour", "file-values", "image-values", "json-csv-values", "write-ops"} {
		if strings.TrimSpace(payload.Examples[name]) == "" {
			t.Fatalf("expected API to include non-empty example %q", name)
		}
	}
}

func TestDataOperationExampleFilesExist(t *testing.T) {
	for _, rel := range []string{
		filepath.Join("..", "..", "testdata", "examples_file_values.spl"),
		filepath.Join("..", "..", "testdata", "examples_image_values.spl"),
		filepath.Join("..", "..", "testdata", "examples_json_csv_values.spl"),
		filepath.Join("..", "..", "testdata", "examples_write_ops.spl"),
		filepath.Join("..", "..", "testdata", "data", "profile.json"),
		filepath.Join("..", "..", "testdata", "data", "people.csv"),
	} {
		data, err := os.ReadFile(rel)
		if err != nil {
			t.Fatalf("read %s: %v", rel, err)
		}
		if strings.TrimSpace(string(data)) == "" {
			t.Fatalf("expected %s to be non-empty", rel)
		}
	}
}

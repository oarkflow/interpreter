package playgroundserver

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

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/ide"
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
	if cfg.ExecutionProfile != "untrusted" {
		t.Fatalf("expected untrusted execution profile by default, got %q", cfg.ExecutionProfile)
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
		"--profile", "trusted",
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
	if cfg.ExecutionProfile != "trusted" {
		t.Fatalf("expected trusted profile, got %q", cfg.ExecutionProfile)
	}
}

func TestPlaygroundSecurityPolicyAllowsReadsButNotWrites(t *testing.T) {
	policy := securityPolicyForProfile("/workspace/project", "untrusted", true, []string{"example.com"}, nil)
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

func TestPlaygroundSecurityPolicyExtraCapabilities(t *testing.T) {
	policy := securityPolicyForProfile("/workspace/project", "untrusted", false, nil, []string{security.CapabilityDB, security.CapabilityNetwork})
	if !security.ContainsToken(policy.AllowedCapabilities, security.CapabilityDB) {
		t.Fatalf("expected extra capability db to be present, got %#v", policy.AllowedCapabilities)
	}
	if !security.ContainsToken(policy.AllowedCapabilities, security.CapabilityNetwork) {
		t.Fatalf("expected extra capability network to be present, got %#v", policy.AllowedCapabilities)
	}
}

func TestPlaygroundTrustedProfileAllowsHostPolicy(t *testing.T) {
	policy := securityPolicyForProfile("/workspace/project", "trusted", true, []string{"example.com"}, nil)
	if policy.ProtectHost {
		t.Fatalf("expected trusted profile to disable host protection")
	}
	if !policy.AllowEnvWrite {
		t.Fatalf("expected trusted profile to allow env writes")
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
	cfg := Config{AuthSecret: "secret", SessionTTL: time.Minute, MaxBodyBytes: 1024}

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
	examples := buildExamples(Variant{})
	for _, name := range []string{"hello", "functions", "formatting", "artifacts", "file-values", "image-values", "json-csv-values", "write-ops", "tools-files", "tools-images", "tools-media", "tools-secrets", "modules", "collections", "error-handling", "loops", "math", "strings", "collections-advanced", "crypto", "time", "testing", "complete-tour"} {
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
	if !strings.Contains(examples["image-values"], `image(`) || !strings.Contains(examples["image-values"], `file_load(rendered)`) {
		t.Fatalf("expected default (lightweight) image-values example to demonstrate core image artifacts, got %q", examples["image-values"])
	}
	if !strings.Contains(examples["json-csv-values"], `table_filter(`) || !strings.Contains(examples["json-csv-values"], `csv_decode(`) {
		t.Fatalf("expected json-csv-values example to demonstrate table helpers, got %q", examples["json-csv-values"])
	}
	if !strings.Contains(examples["tools-files"], `bulk_rename(`) || !strings.Contains(examples["tools-files"], `file_finder(`) || !strings.Contains(examples["tools-files"], `content_regex(`) || !strings.Contains(examples["tools-images"], `image_convert_batch(`) || !strings.Contains(examples["tools-media"], `ffmpeg_status(`) || !strings.Contains(examples["tools-secrets"], `secret_generate(`) {
		t.Fatalf("expected tools examples to surface daily tools builtins")
	}
	if !strings.Contains(examples["query-builder"], `.where_in(`) || !strings.Contains(examples["query-builder"], `.where_like(`) {
		t.Fatalf("expected default (lightweight) query-builder example to surface common query filters")
	}
}

func TestBuiltinCodeExamplesVariantOverridesAndExtras(t *testing.T) {
	examples := buildExamples(Variant{
		ExampleOverrides: map[string]string{"image-values": "OVERRIDDEN"},
		ExtraExamples:    map[string]string{"pdf-tools": "EXTRA"},
	})
	if examples["image-values"] != "OVERRIDDEN" {
		t.Fatalf("expected variant override to replace image-values, got %q", examples["image-values"])
	}
	if examples["pdf-tools"] != "EXTRA" {
		t.Fatalf("expected variant extra example pdf-tools to be present, got %q", examples["pdf-tools"])
	}
	// Everything else should be untouched.
	if strings.TrimSpace(examples["hello"]) == "" {
		t.Fatalf("expected base examples to remain present alongside overrides/extras")
	}
}

func TestExamplesAPIContainsCompleteCodeExamples(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/examples", func(w http.ResponseWriter, r *http.Request) {
		writeJSON(w, http.StatusOK, map[string]any{
			"examples": buildExamples(Variant{}),
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
	for _, name := range []string{"collections", "error-handling", "complete-tour", "file-values", "image-values", "json-csv-values", "write-ops", "tools-files", "tools-images", "tools-media", "tools-secrets"} {
		if strings.TrimSpace(payload.Examples[name]) == "" {
			t.Fatalf("expected API to include non-empty example %q", name)
		}
	}
}

func TestReactiveHTMLExampleUsesRawHTMLAndRuns(t *testing.T) {
	src := buildExamples(Variant{})["reactive-html"]
	if strings.Contains(src, `\"`) {
		t.Fatalf("reactive-html example should use raw HTML content, got escaped quotes")
	}
	res := interpreter.EvalForPlayground(src, interpreter.PlaygroundOptions{
		MaxDepth:  200,
		MaxSteps:  2_000_000,
		TimeoutMS: 5000,
		ModuleDir: filepath.Join("..", ".."),
		Security:  securityPolicyForProfile(filepath.Join("..", ".."), "untrusted", false, nil, nil),
		RenderConfig: &interpreter.RenderConfig{
			Mode:     "auto",
			MaxBytes: 1 << 20,
		},
	})
	if res.Error != "" {
		t.Fatalf("reactive-html example failed: %s\n%s", res.Error, strings.Join(res.Diagnostics, "\n"))
	}
	if !strings.Contains(res.Output, `<div class="app">`) {
		t.Fatalf("expected rendered output to contain raw HTML attributes, got %q", res.Output)
	}
}

func TestDataOperationExampleFilesExist(t *testing.T) {
	for _, rel := range []string{
		filepath.Join("..", "..", "examples", "all_in_one.spl"),
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

// newTestIDEMux wires an ide.Server the same way Run does, backed by a
// temp workspace, mounted on a fresh mux alongside an auth check so gating
// behavior matches production.
func newTestIDEMux(t *testing.T, auth *authManager) *http.ServeMux {
	t.Helper()
	workspace := t.TempDir()
	ideServer, err := ide.NewServer(ide.RunnerConfig{
		Variant:      "lightweight",
		ScaffoldKind: ide.ScaffoldMinimal,
	}, workspace, func(r *http.Request) bool {
		return isAuthenticated(r, auth)
	})
	if err != nil {
		t.Fatalf("ide.NewServer: %v", err)
	}
	mux := http.NewServeMux()
	ideServer.Routes(mux)
	return mux
}

func TestProjectsRequireAuthWhenConfigured(t *testing.T) {
	auth := newAuthManager("secret", time.Minute)
	mux := newTestIDEMux(t, auth)

	req := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401 without a session, got %d: %s", rec.Code, rec.Body.String())
	}

	token, _, err := auth.issue()
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	req = httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	req.AddCookie(&http.Cookie{Name: "spl_playground_session", Value: token})
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 with a valid session, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestProjectsCRUDOverHTTP(t *testing.T) {
	mux := newTestIDEMux(t, nil) // no auth configured -> open access, like production with no PLAYGROUND_AUTH_SECRET

	createReq := httptest.NewRequest(http.MethodPost, "/api/projects", bytes.NewBufferString(`{"name":"HTTP Test","scaffold_kind":"minimal"}`))
	createRec := httptest.NewRecorder()
	mux.ServeHTTP(createRec, createReq)
	if createRec.Code != http.StatusCreated {
		t.Fatalf("expected 201 creating a project, got %d: %s", createRec.Code, createRec.Body.String())
	}
	var created struct {
		Project struct {
			ID string `json:"id"`
		} `json:"project"`
	}
	if err := json.Unmarshal(createRec.Body.Bytes(), &created); err != nil {
		t.Fatalf("decode create response: %v", err)
	}
	id := created.Project.ID
	if id == "" {
		t.Fatalf("expected a project id, got %s", createRec.Body.String())
	}

	listReq := httptest.NewRequest(http.MethodGet, "/api/projects", nil)
	listRec := httptest.NewRecorder()
	mux.ServeHTTP(listRec, listReq)
	if !strings.Contains(listRec.Body.String(), id) {
		t.Fatalf("expected list to contain created project, got %s", listRec.Body.String())
	}

	treeReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+id+"/files", nil)
	treeRec := httptest.NewRecorder()
	mux.ServeHTTP(treeRec, treeReq)
	if treeRec.Code != http.StatusOK || !strings.Contains(treeRec.Body.String(), "main.spl") {
		t.Fatalf("expected file tree to contain main.spl, got %d: %s", treeRec.Code, treeRec.Body.String())
	}

	readReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+id+"/files/main.spl", nil)
	readRec := httptest.NewRecorder()
	mux.ServeHTTP(readRec, readReq)
	if readRec.Code != http.StatusOK {
		t.Fatalf("expected 200 reading main.spl, got %d: %s", readRec.Code, readRec.Body.String())
	}

	// A traversal attempt must never resolve outside the project directory:
	// SafeJoin collapses ".."-heavy paths back inside the project root, so
	// this 404s (file doesn't exist there) rather than ever leaking a real
	// host file.
	traversalReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+id+"/files/%2e%2e/%2e%2e/etc/passwd", nil)
	traversalRec := httptest.NewRecorder()
	mux.ServeHTTP(traversalRec, traversalReq)
	if traversalRec.Code == http.StatusOK {
		t.Fatalf("expected traversal attempt to fail, got 200: %s", traversalRec.Body.String())
	}

	writeReq := httptest.NewRequest(http.MethodPut, "/api/projects/"+id+"/files/app/controllers/home_controller.spl", bytes.NewBufferString(`{"content":"print 1;"}`))
	writeRec := httptest.NewRecorder()
	mux.ServeHTTP(writeRec, writeReq)
	if writeRec.Code != http.StatusOK {
		t.Fatalf("expected 200 writing a file, got %d: %s", writeRec.Code, writeRec.Body.String())
	}

	diagReq := httptest.NewRequest(http.MethodPost, "/api/projects/"+id+"/tooling/diagnostics", bytes.NewBufferString(`{"path":"main.spl","content":"let x = ;"}`))
	diagRec := httptest.NewRecorder()
	mux.ServeHTTP(diagRec, diagReq)
	if diagRec.Code != http.StatusOK || !strings.Contains(diagRec.Body.String(), "diagnostics") {
		t.Fatalf("expected diagnostics response, got %d: %s", diagRec.Code, diagRec.Body.String())
	}

	statusReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+id+"/run/status", nil)
	statusRec := httptest.NewRecorder()
	mux.ServeHTTP(statusRec, statusReq)
	if !strings.Contains(statusRec.Body.String(), `"idle"`) {
		t.Fatalf("expected idle status before start, got %s", statusRec.Body.String())
	}

	deleteReq := httptest.NewRequest(http.MethodDelete, "/api/projects/"+id, nil)
	deleteRec := httptest.NewRecorder()
	mux.ServeHTTP(deleteRec, deleteReq)
	if deleteRec.Code != http.StatusOK {
		t.Fatalf("expected 200 deleting a project, got %d: %s", deleteRec.Code, deleteRec.Body.String())
	}

	getAfterDeleteReq := httptest.NewRequest(http.MethodGet, "/api/projects/"+id+"/files", nil)
	getAfterDeleteRec := httptest.NewRecorder()
	mux.ServeHTTP(getAfterDeleteRec, getAfterDeleteReq)
	if getAfterDeleteRec.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for a deleted project, got %d", getAfterDeleteRec.Code)
	}
}

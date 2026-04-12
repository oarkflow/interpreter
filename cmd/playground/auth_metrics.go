package main

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"fmt"
	"net/http"
	"sort"
	"strings"
	"sync"
	"time"
)

const (
	playgroundSessionCookie = "spl_playground_session"
)

type authManager struct {
	mu       sync.RWMutex
	secret   string
	sessions map[string]time.Time
	ttl      time.Duration
}

func newAuthManager(secret string, ttl time.Duration) *authManager {
	return &authManager{
		secret:   secret,
		sessions: make(map[string]time.Time),
		ttl:      ttl,
	}
}

func (a *authManager) verifySecret(candidate string) bool {
	if a == nil || a.secret == "" {
		return false
	}
	return subtle.ConstantTimeCompare([]byte(candidate), []byte(a.secret)) == 1
}

func (a *authManager) issue() (string, time.Time, error) {
	token, err := randomToken(32)
	if err != nil {
		return "", time.Time{}, err
	}
	expires := time.Now().Add(a.ttl)
	a.mu.Lock()
	a.sessions[token] = expires
	a.mu.Unlock()
	return token, expires, nil
}

func (a *authManager) validate(token string) bool {
	if a == nil || token == "" {
		return false
	}
	a.mu.RLock()
	expires, ok := a.sessions[token]
	a.mu.RUnlock()
	if !ok {
		return false
	}
	if time.Now().After(expires) {
		a.mu.Lock()
		delete(a.sessions, token)
		a.mu.Unlock()
		return false
	}
	return true
}

func (a *authManager) revoke(token string) {
	if a == nil || token == "" {
		return
	}
	a.mu.Lock()
	delete(a.sessions, token)
	a.mu.Unlock()
}

func (a *authManager) cleanup(now time.Time) {
	if a == nil {
		return
	}
	a.mu.Lock()
	for token, exp := range a.sessions {
		if now.After(exp) {
			delete(a.sessions, token)
		}
	}
	a.mu.Unlock()
}

func (a *authManager) activeSessions() int {
	if a == nil {
		return 0
	}
	a.mu.RLock()
	defer a.mu.RUnlock()
	return len(a.sessions)
}

func tokenFromRequest(r *http.Request) string {
	if c, err := r.Cookie(playgroundSessionCookie); err == nil {
		if v := strings.TrimSpace(c.Value); v != "" {
			return v
		}
	}
	auth := strings.TrimSpace(r.Header.Get("Authorization"))
	if strings.HasPrefix(strings.ToLower(auth), "bearer ") {
		return strings.TrimSpace(auth[7:])
	}
	return ""
}

func writeSessionCookie(w http.ResponseWriter, token string, secure bool, ttl time.Duration) {
	http.SetCookie(w, &http.Cookie{
		Name:     playgroundSessionCookie,
		Value:    token,
		Path:     "/",
		MaxAge:   int(ttl.Seconds()),
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	})
}

func clearSessionCookie(w http.ResponseWriter, secure bool) {
	http.SetCookie(w, &http.Cookie{
		Name:     playgroundSessionCookie,
		Value:    "",
		Path:     "/",
		MaxAge:   -1,
		HttpOnly: true,
		SameSite: http.SameSiteStrictMode,
		Secure:   secure,
	})
}

func randomToken(n int) (string, error) {
	if n <= 0 {
		n = 32
	}
	buf := make([]byte, n)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

type durationMetric struct {
	counts []uint64
	sum    float64
	total  uint64
}

type playgroundMetrics struct {
	mu            sync.Mutex
	buckets       []float64
	httpRequests  map[string]uint64
	httpDurations map[string]*durationMetric
	execDurations map[string]*durationMetric
	authEvents    map[string]uint64
	rateLimited   uint64
	loginLatency  *durationMetric
}

func newPlaygroundMetrics() *playgroundMetrics {
	buckets := []float64{0.005, 0.01, 0.025, 0.05, 0.1, 0.25, 0.5, 1, 2, 5, 10}
	return &playgroundMetrics{
		buckets:       buckets,
		httpRequests:  make(map[string]uint64),
		httpDurations: make(map[string]*durationMetric),
		execDurations: make(map[string]*durationMetric),
		authEvents:    make(map[string]uint64),
		loginLatency:  &durationMetric{counts: make([]uint64, len(buckets)+1)},
	}
}

func routeLabel(path string) string {
	switch {
	case path == "/", path == "/index.html":
		return "ui"
	case strings.HasPrefix(path, "/api/execute"):
		return "execute"
	case strings.HasPrefix(path, "/api/render"):
		return "render"
	case strings.HasPrefix(path, "/api/login"):
		return "login"
	case strings.HasPrefix(path, "/api/logout"):
		return "logout"
	case strings.HasPrefix(path, "/api/session"):
		return "session"
	case strings.HasPrefix(path, "/api/health"):
		return "health"
	case strings.HasPrefix(path, "/api/ready"):
		return "ready"
	case strings.HasPrefix(path, "/metrics"):
		return "metrics"
	default:
		return "static"
	}
}

func metricKey(parts ...string) string {
	return strings.Join(parts, "|")
}

func ensureMetric(m map[string]*durationMetric, key string, bucketCount int) *durationMetric {
	if metric, ok := m[key]; ok {
		return metric
	}
	metric := &durationMetric{counts: make([]uint64, bucketCount+1)}
	m[key] = metric
	return metric
}

func recordDurationMetric(metric *durationMetric, buckets []float64, valueSeconds float64) {
	if metric == nil {
		return
	}
	metric.total++
	metric.sum += valueSeconds
	for i, bound := range buckets {
		if valueSeconds <= bound {
			metric.counts[i]++
		}
	}
	metric.counts[len(metric.counts)-1]++
}

func (m *playgroundMetrics) recordRequest(path, method string, status int, duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	key := metricKey(routeLabel(path), method, fmt.Sprintf("%d", status))
	m.httpRequests[key]++
	reqKey := metricKey(routeLabel(path), method)
	recordDurationMetric(ensureMetric(m.httpDurations, reqKey, len(m.buckets)), m.buckets, duration.Seconds())
}

func (m *playgroundMetrics) recordExecution(kind string, duration time.Duration) {
	if m == nil {
		return
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	recordDurationMetric(ensureMetric(m.execDurations, kind, len(m.buckets)), m.buckets, duration.Seconds())
}

func (m *playgroundMetrics) recordAuth(event string) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.authEvents[event]++
	m.mu.Unlock()
}

func (m *playgroundMetrics) recordRateLimited() {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.rateLimited++
	m.mu.Unlock()
}

func (m *playgroundMetrics) setActiveSessions(n int) {
	if m == nil {
		return
	}
	m.mu.Lock()
	m.authEvents["sessions_active"] = uint64(n)
	m.mu.Unlock()
}

func sanitizeLabel(v string) string {
	return strings.ReplaceAll(strings.ReplaceAll(v, `\`, `\\`), `"`, `\"`)
}

func renderDurationMetric(name, help string, buckets []float64, metrics map[string]*durationMetric, labelName string) string {
	var out strings.Builder
	out.WriteString("# HELP ")
	out.WriteString(name)
	out.WriteString(" ")
	out.WriteString(help)
	out.WriteString("\n# TYPE ")
	out.WriteString(name)
	out.WriteString(" histogram\n")

	keys := make([]string, 0, len(metrics))
	for key := range metrics {
		keys = append(keys, key)
	}
	sort.Strings(keys)

	for _, key := range keys {
		metric := metrics[key]
		labelValue := sanitizeLabel(key)
		routeValue := labelValue
		methodValue := ""
		if labelName == "route_method" {
			parts := strings.SplitN(key, "|", 2)
			if len(parts) == 2 {
				routeValue = sanitizeLabel(parts[0])
				methodValue = sanitizeLabel(parts[1])
			}
		}
		for i, bucket := range buckets {
			if labelName == "route_method" {
				out.WriteString(fmt.Sprintf("%s_bucket{route=\"%s\",method=\"%s\",le=\"%.3f\"} %d\n", name, routeValue, methodValue, bucket, metric.counts[i]))
				continue
			}
			out.WriteString(fmt.Sprintf("%s_bucket{%s=\"%s\",le=\"%.3f\"} %d\n", name, labelName, labelValue, bucket, metric.counts[i]))
		}
		if labelName == "route_method" {
			out.WriteString(fmt.Sprintf("%s_bucket{route=\"%s\",method=\"%s\",le=\"+Inf\"} %d\n", name, routeValue, methodValue, metric.counts[len(metric.counts)-1]))
			out.WriteString(fmt.Sprintf("%s_sum{route=\"%s\",method=\"%s\"} %.6f\n", name, routeValue, methodValue, metric.sum))
			out.WriteString(fmt.Sprintf("%s_count{route=\"%s\",method=\"%s\"} %d\n", name, routeValue, methodValue, metric.total))
			continue
		}
		out.WriteString(fmt.Sprintf("%s_bucket{%s=\"%s\",le=\"+Inf\"} %d\n", name, labelName, labelValue, metric.counts[len(metric.counts)-1]))
		out.WriteString(fmt.Sprintf("%s_sum{%s=\"%s\"} %.6f\n", name, labelName, labelValue, metric.sum))
		out.WriteString(fmt.Sprintf("%s_count{%s=\"%s\"} %d\n", name, labelName, labelValue, metric.total))
	}

	return out.String()
}

func (m *playgroundMetrics) renderPrometheus() string {
	if m == nil {
		return ""
	}
	m.mu.Lock()
	defer m.mu.Unlock()

	var out strings.Builder
	out.WriteString("# HELP spl_playground_http_requests_total Total HTTP requests by route, method, and status.\n")
	out.WriteString("# TYPE spl_playground_http_requests_total counter\n")

	requestKeys := make([]string, 0, len(m.httpRequests))
	for key := range m.httpRequests {
		requestKeys = append(requestKeys, key)
	}
	sort.Strings(requestKeys)
	for _, key := range requestKeys {
		parts := strings.Split(key, "|")
		if len(parts) != 3 {
			continue
		}
		out.WriteString(fmt.Sprintf("spl_playground_http_requests_total{route=\"%s\",method=\"%s\",status=\"%s\"} %d\n",
			sanitizeLabel(parts[0]), sanitizeLabel(parts[1]), sanitizeLabel(parts[2]), m.httpRequests[key]))
	}

	out.WriteString("# HELP spl_playground_auth_events_total Authentication events.\n")
	out.WriteString("# TYPE spl_playground_auth_events_total counter\n")
	authKeys := make([]string, 0, len(m.authEvents))
	for key := range m.authEvents {
		authKeys = append(authKeys, key)
	}
	sort.Strings(authKeys)
	for _, key := range authKeys {
		if key == "sessions_active" {
			continue
		}
		out.WriteString(fmt.Sprintf("spl_playground_auth_events_total{event=\"%s\"} %d\n", sanitizeLabel(key), m.authEvents[key]))
	}
	out.WriteString(fmt.Sprintf("spl_playground_sessions_active %d\n", m.authEvents["sessions_active"]))
	out.WriteString(fmt.Sprintf("spl_playground_rate_limited_total %d\n", m.rateLimited))

	out.WriteString(renderDurationMetric(
		"spl_playground_http_request_duration_seconds",
		"HTTP request duration by route and method.",
		m.buckets,
		m.httpDurations,
		"route_method",
	))
	out.WriteString(renderDurationMetric(
		"spl_playground_execution_duration_seconds",
		"Interpreter execution duration by endpoint.",
		m.buckets,
		m.execDurations,
		"kind",
	))
	return out.String()
}

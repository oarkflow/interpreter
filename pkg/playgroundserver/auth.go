package playgroundserver

import (
	"crypto/rand"
	"crypto/subtle"
	"encoding/base64"
	"net/http"
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


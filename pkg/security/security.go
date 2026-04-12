package security

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/oarkflow/interpreter/pkg/object"
)

type SecurityPolicy = object.SecurityPolicy

var policyOverride struct {
	mu     sync.RWMutex
	policy *SecurityPolicy
}

func LoadSecurityPolicyFromEnv() *SecurityPolicy {
	strict := strings.EqualFold(strings.TrimSpace(os.Getenv("SPL_SECURITY_MODE")), "strict")
	protectHost := parseBoolEnvDefault("SPL_PROTECT_HOST", false)
	allowEnvWrite := parseBoolEnvDefault("SPL_ALLOW_ENV_WRITE", !strict && !protectHost)
	return &SecurityPolicy{
		StrictMode:            strict,
		ProtectHost:           protectHost,
		AllowEnvWrite:         allowEnvWrite,
		AllowedExecCommands:   parseCSVEnv("SPL_EXEC_ALLOW_CMDS"),
		DeniedExecCommands:    parseCSVEnv("SPL_EXEC_DENY_CMDS"),
		AllowedNetworkHosts:   parseCSVEnv("SPL_NETWORK_ALLOW"),
		DeniedNetworkHosts:    parseCSVEnv("SPL_NETWORK_DENY"),
		AllowedDBDrivers:      parseCSVEnv("SPL_DB_ALLOW_DRIVERS"),
		DeniedDBDrivers:       parseCSVEnv("SPL_DB_DENY_DRIVERS"),
		AllowedDBDSNPatterns:  parseCSVEnv("SPL_DB_DSN_ALLOW"),
		DeniedDBDSNPatterns:   parseCSVEnv("SPL_DB_DSN_DENY"),
		AllowedFileReadPaths:  parseCSVEnv("SPL_FILE_READ_ALLOW"),
		DeniedFileReadPaths:   parseCSVEnv("SPL_FILE_READ_DENY"),
		AllowedFileWritePaths: parseCSVEnv("SPL_FILE_WRITE_ALLOW"),
		DeniedFileWritePaths:  parseCSVEnv("SPL_FILE_WRITE_DENY"),
	}
}

// WithSecurityPolicyOverride temporarily sets the given policy as the active
// override, calls fn, then restores the previous policy. The callback uses
// `any` return types as a placeholder for the interpreter's Object type to
// avoid circular imports. Callers in the interpreter package should type-assert
// the returned value back to Object.
func WithSecurityPolicyOverride(policy *SecurityPolicy, fn func() (any, error)) (any, error) {
	if policy == nil {
		return fn()
	}
	policyOverride.mu.Lock()
	prev := policyOverride.policy
	policyOverride.policy = policy
	policyOverride.mu.Unlock()

	defer func() {
		policyOverride.mu.Lock()
		policyOverride.policy = prev
		policyOverride.mu.Unlock()
	}()

	return fn()
}

func ActiveSecurityPolicy() *SecurityPolicy {
	policyOverride.mu.RLock()
	p := policyOverride.policy
	policyOverride.mu.RUnlock()
	if p != nil {
		return p
	}
	return LoadSecurityPolicyFromEnv()
}

// ---------------------------------------------------------------------------
// Environment-variable helpers
// ---------------------------------------------------------------------------

func parseCSVEnv(name string) []string {
	raw := strings.TrimSpace(os.Getenv(name))
	if raw == "" {
		return nil
	}
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(strings.ToLower(p))
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}

func parseBoolEnvDefault(name string, def bool) bool {
	v := strings.TrimSpace(strings.ToLower(os.Getenv(name)))
	if v == "" {
		return def
	}
	switch v {
	case "1", "true", "yes", "on":
		return true
	case "0", "false", "no", "off":
		return false
	default:
		return def
	}
}

// ---------------------------------------------------------------------------
// Generic helpers
// ---------------------------------------------------------------------------

func ContainsToken(list []string, item string) bool {
	item = strings.ToLower(strings.TrimSpace(item))
	for _, v := range list {
		if strings.ToLower(strings.TrimSpace(v)) == item {
			return true
		}
	}
	return false
}

// ---------------------------------------------------------------------------
// Policy check functions
// ---------------------------------------------------------------------------

func CheckExecAllowed(cmd string) error {
	p := ActiveSecurityPolicy()
	if p.ProtectHost {
		return fmt.Errorf("exec denied by host protection policy")
	}
	name := strings.ToLower(strings.TrimSpace(cmd))
	if ContainsToken(p.DeniedExecCommands, name) {
		return fmt.Errorf("exec denied for command %q", cmd)
	}
	if len(p.AllowedExecCommands) > 0 {
		if !ContainsToken(p.AllowedExecCommands, name) {
			return fmt.Errorf("exec not allowed for command %q", cmd)
		}
		return nil
	}
	if p.StrictMode {
		return fmt.Errorf("exec denied in strict security mode")
	}
	return nil
}

func MatchHostPattern(host string, pattern string) bool {
	host = strings.ToLower(strings.TrimSpace(host))
	pattern = strings.ToLower(strings.TrimSpace(pattern))
	if host == "" || pattern == "" {
		return false
	}
	if pattern == "*" {
		return true
	}
	if strings.HasPrefix(pattern, "*.") {
		suffix := strings.TrimPrefix(pattern, "*.")
		return strings.HasSuffix(host, "."+suffix) || host == suffix
	}
	return host == pattern
}

func HostFromTarget(target string) (string, error) {
	target = strings.TrimSpace(target)
	if target == "" {
		return "", fmt.Errorf("empty target")
	}
	if strings.Contains(target, "://") {
		u, err := url.Parse(target)
		if err != nil {
			return "", err
		}
		h := strings.TrimSpace(u.Hostname())
		if h == "" {
			return "", fmt.Errorf("missing host")
		}
		return h, nil
	}
	h, _, err := net.SplitHostPort(target)
	if err == nil {
		return h, nil
	}
	return target, nil
}

func CheckNetworkAllowed(target string) error {
	p := ActiveSecurityPolicy()
	host, err := HostFromTarget(target)
	if err != nil {
		return fmt.Errorf("invalid network target: %w", err)
	}
	for _, deny := range p.DeniedNetworkHosts {
		if MatchHostPattern(host, deny) {
			return fmt.Errorf("network target denied: %s", host)
		}
	}
	if len(p.AllowedNetworkHosts) > 0 {
		for _, allow := range p.AllowedNetworkHosts {
			if MatchHostPattern(host, allow) {
				return nil
			}
		}
		return fmt.Errorf("network target not allowed: %s", host)
	}
	if p.StrictMode {
		return fmt.Errorf("network access denied in strict security mode")
	}
	return nil
}

func CheckDBAllowed(driver string, dsn string) error {
	p := ActiveSecurityPolicy()
	d := strings.ToLower(strings.TrimSpace(driver))
	if ContainsToken(p.DeniedDBDrivers, d) {
		return fmt.Errorf("db driver denied: %s", driver)
	}
	if len(p.AllowedDBDrivers) > 0 && !ContainsToken(p.AllowedDBDrivers, d) {
		return fmt.Errorf("db driver not allowed: %s", driver)
	}
	for _, deny := range p.DeniedDBDSNPatterns {
		if strings.Contains(strings.ToLower(dsn), deny) {
			return fmt.Errorf("db dsn denied by policy")
		}
	}
	if len(p.AllowedDBDSNPatterns) > 0 {
		ok := false
		for _, allow := range p.AllowedDBDSNPatterns {
			if strings.Contains(strings.ToLower(dsn), allow) {
				ok = true
				break
			}
		}
		if !ok {
			return fmt.Errorf("db dsn not allowed")
		}
	}
	if p.StrictMode && len(p.AllowedDBDrivers) == 0 {
		return fmt.Errorf("db access denied in strict security mode")
	}
	return nil
}

// ---------------------------------------------------------------------------
// File-path helpers and checks
// ---------------------------------------------------------------------------

func CleanAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func PathMatches(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	cp := CleanAbs(path)
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		pp := CleanAbs(p)
		if cp == pp || strings.HasPrefix(cp+string(os.PathSeparator), pp+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func CheckFileReadAllowed(path string) error {
	p := ActiveSecurityPolicy()
	if PathMatches(path, p.DeniedFileReadPaths) {
		return fmt.Errorf("file read denied by policy")
	}
	if len(p.AllowedFileReadPaths) > 0 && !PathMatches(path, p.AllowedFileReadPaths) {
		return fmt.Errorf("file read not allowed by policy")
	}
	if p.StrictMode && len(p.AllowedFileReadPaths) == 0 {
		return fmt.Errorf("file read denied in strict security mode")
	}
	return nil
}

func CheckFileWriteAllowed(path string) error {
	p := ActiveSecurityPolicy()
	if p.ProtectHost {
		return fmt.Errorf("file mutation denied by host protection policy")
	}
	if PathMatches(path, p.DeniedFileWritePaths) {
		return fmt.Errorf("file write denied by policy")
	}
	if len(p.AllowedFileWritePaths) > 0 && !PathMatches(path, p.AllowedFileWritePaths) {
		return fmt.Errorf("file write not allowed by policy")
	}
	if p.StrictMode && len(p.AllowedFileWritePaths) == 0 {
		return fmt.Errorf("file write denied in strict security mode")
	}
	return nil
}

func ExitAllowed() error {
	p := ActiveSecurityPolicy()
	if p.ProtectHost {
		return fmt.Errorf("process exit denied by host protection policy")
	}
	return nil
}

func EnvWriteAllowed(key string) error {
	p := ActiveSecurityPolicy()
	if p.ProtectHost {
		return fmt.Errorf("environment writes are disabled by host protection policy")
	}
	if strings.HasPrefix(strings.ToUpper(strings.TrimSpace(key)), "SPL_") {
		return fmt.Errorf("refusing to mutate protected SPL_* environment variable")
	}
	if !p.AllowEnvWrite {
		return fmt.Errorf("environment writes are disabled by security policy")
	}
	return nil
}

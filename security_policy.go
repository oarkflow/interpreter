package interpreter

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
)

type SecurityPolicy struct {
	StrictMode            bool
	ProtectHost           bool
	AllowEnvWrite         bool
	AllowedExecCommands   []string
	DeniedExecCommands    []string
	AllowedNetworkHosts   []string
	DeniedNetworkHosts    []string
	AllowedDBDrivers      []string
	DeniedDBDrivers       []string
	AllowedFileReadPaths  []string
	DeniedFileReadPaths   []string
	AllowedFileWritePaths []string
	DeniedFileWritePaths  []string
	AllowedDBDSNPatterns  []string
	DeniedDBDSNPatterns   []string
}

var securityPolicyOverride struct {
	mu     sync.RWMutex
	policy *SecurityPolicy
}

func loadSecurityPolicyFromEnv() *SecurityPolicy {
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

func withSecurityPolicyOverride(policy *SecurityPolicy, fn func() (Object, error)) (Object, error) {
	if policy == nil {
		return fn()
	}
	securityPolicyOverride.mu.Lock()
	prev := securityPolicyOverride.policy
	securityPolicyOverride.policy = policy
	securityPolicyOverride.mu.Unlock()

	defer func() {
		securityPolicyOverride.mu.Lock()
		securityPolicyOverride.policy = prev
		securityPolicyOverride.mu.Unlock()
	}()

	return fn()
}

func activeSecurityPolicy() *SecurityPolicy {
	securityPolicyOverride.mu.RLock()
	p := securityPolicyOverride.policy
	securityPolicyOverride.mu.RUnlock()
	if p != nil {
		return p
	}
	return loadSecurityPolicyFromEnv()
}

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

func containsToken(list []string, item string) bool {
	item = strings.ToLower(strings.TrimSpace(item))
	for _, v := range list {
		if strings.ToLower(strings.TrimSpace(v)) == item {
			return true
		}
	}
	return false
}

func checkExecAllowed(cmd string) error {
	p := activeSecurityPolicy()
	if p.ProtectHost {
		return fmt.Errorf("exec denied by host protection policy")
	}
	name := strings.ToLower(strings.TrimSpace(cmd))
	if containsToken(p.DeniedExecCommands, name) {
		return fmt.Errorf("exec denied for command %q", cmd)
	}
	if len(p.AllowedExecCommands) > 0 {
		if !containsToken(p.AllowedExecCommands, name) {
			return fmt.Errorf("exec not allowed for command %q", cmd)
		}
		return nil
	}
	if p.StrictMode {
		return fmt.Errorf("exec denied in strict security mode")
	}
	return nil
}

func matchHostPattern(host string, pattern string) bool {
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

func hostFromTarget(target string) (string, error) {
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

func checkNetworkAllowed(target string) error {
	p := activeSecurityPolicy()
	host, err := hostFromTarget(target)
	if err != nil {
		return fmt.Errorf("invalid network target: %w", err)
	}
	for _, deny := range p.DeniedNetworkHosts {
		if matchHostPattern(host, deny) {
			return fmt.Errorf("network target denied: %s", host)
		}
	}
	if len(p.AllowedNetworkHosts) > 0 {
		for _, allow := range p.AllowedNetworkHosts {
			if matchHostPattern(host, allow) {
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

func checkDBAllowed(driver string, dsn string) error {
	p := activeSecurityPolicy()
	d := strings.ToLower(strings.TrimSpace(driver))
	if containsToken(p.DeniedDBDrivers, d) {
		return fmt.Errorf("db driver denied: %s", driver)
	}
	if len(p.AllowedDBDrivers) > 0 && !containsToken(p.AllowedDBDrivers, d) {
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

func cleanAbs(path string) string {
	abs, err := filepath.Abs(path)
	if err != nil {
		return filepath.Clean(path)
	}
	return filepath.Clean(abs)
}

func pathMatches(path string, patterns []string) bool {
	if len(patterns) == 0 {
		return false
	}
	cp := cleanAbs(path)
	for _, raw := range patterns {
		p := strings.TrimSpace(raw)
		if p == "" {
			continue
		}
		pp := cleanAbs(p)
		if cp == pp || strings.HasPrefix(cp+string(os.PathSeparator), pp+string(os.PathSeparator)) {
			return true
		}
	}
	return false
}

func checkFileReadAllowed(path string) error {
	p := activeSecurityPolicy()
	if pathMatches(path, p.DeniedFileReadPaths) {
		return fmt.Errorf("file read denied by policy")
	}
	if len(p.AllowedFileReadPaths) > 0 && !pathMatches(path, p.AllowedFileReadPaths) {
		return fmt.Errorf("file read not allowed by policy")
	}
	if p.StrictMode && len(p.AllowedFileReadPaths) == 0 {
		return fmt.Errorf("file read denied in strict security mode")
	}
	return nil
}

func checkFileWriteAllowed(path string) error {
	p := activeSecurityPolicy()
	if p.ProtectHost {
		return fmt.Errorf("file mutation denied by host protection policy")
	}
	if pathMatches(path, p.DeniedFileWritePaths) {
		return fmt.Errorf("file write denied by policy")
	}
	if len(p.AllowedFileWritePaths) > 0 && !pathMatches(path, p.AllowedFileWritePaths) {
		return fmt.Errorf("file write not allowed by policy")
	}
	if p.StrictMode && len(p.AllowedFileWritePaths) == 0 {
		return fmt.Errorf("file write denied in strict security mode")
	}
	return nil
}

func exitAllowed() error {
	p := activeSecurityPolicy()
	if p.ProtectHost {
		return fmt.Errorf("process exit denied by host protection policy")
	}
	return nil
}

func envWriteAllowed(key string) error {
	p := activeSecurityPolicy()
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

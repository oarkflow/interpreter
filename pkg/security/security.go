package security

import (
	"fmt"
	"net"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/oarkflow/interpreter/pkg/object"
)

type SecurityPolicy = object.SecurityPolicy

// policyOverride holds the process-wide active policy override. `current` is
// an atomic pointer so ActiveSecurityPolicy (called repeatedly by every
// capability check, from within the same goroutine that may be holding `mu`
// for the duration of WithSecurityPolicyOverride's fn() call) can read it
// lock-free without risking self-deadlock on a non-reentrant sync.Mutex. `mu`
// is used purely to serialize/queue concurrent WithSecurityPolicyOverride
// callers so overlapping override scopes don't interleave.
var policyOverride struct {
	mu      sync.Mutex
	current atomic.Pointer[SecurityPolicy]
}

func LoadSecurityPolicyFromEnv() *SecurityPolicy {
	strict := strings.EqualFold(strings.TrimSpace(os.Getenv("SPL_SECURITY_MODE")), "strict")
	protectHost := parseBoolEnvDefault("SPL_PROTECT_HOST", false)
	allowEnvWrite := parseBoolEnvDefault("SPL_ALLOW_ENV_WRITE", !strict && !protectHost)
	return &SecurityPolicy{
		StrictMode:            strict,
		ProtectHost:           protectHost,
		AllowEnvWrite:         allowEnvWrite,
		AllowedCapabilities:   parseCSVEnv("SPL_CAP_ALLOW"),
		DeniedCapabilities:    parseCSVEnv("SPL_CAP_DENY"),
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
		AllowedImportPaths:    parseCSVEnv("SPL_IMPORT_PATH_ALLOW"),
		DeniedImportPaths:     parseCSVEnv("SPL_IMPORT_PATH_DENY"),
		AllowedImportPackages: parseCSVEnv("SPL_IMPORT_PACKAGE_ALLOW"),
		DeniedImportPackages:  parseCSVEnv("SPL_IMPORT_PACKAGE_DENY"),
		AllowedNativeModules:  parseCSVEnv("SPL_NATIVE_ALLOW"),
		DeniedNativeModules:   parseCSVEnv("SPL_NATIVE_DENY"),
		DenyDynamicImports:    parseBoolEnvDefault("SPL_IMPORT_DENY_DYNAMIC", false),
		BlockHardcodedSecrets: parseBoolEnvDefault("SPL_BLOCK_HARDCODED_SECRETS", false),
	}
}

// WithSecurityPolicyOverride temporarily sets the given policy as the active
// override, calls fn, then restores the previous policy. The callback uses
// `any` return types as a placeholder for the interpreter's Object type to
// avoid circular imports. Callers in the interpreter package should type-assert
// the returned value back to Object.
//
// The mutex is held for the FULL DURATION of fn(), not just for the swap, so
// that concurrent in-process callers (e.g. the playground's per-request
// EvalForPlayground calls) are fully serialized and can never observe or be
// affected by another request's policy during the overlap window.
func WithSecurityPolicyOverride(policy *SecurityPolicy, fn func() (any, error)) (any, error) {
	if policy == nil {
		return fn()
	}
	policyOverride.mu.Lock()
	defer policyOverride.mu.Unlock()

	prev := policyOverride.current.Load()
	policyOverride.current.Store(policy)
	defer policyOverride.current.Store(prev)

	return fn()
}

// DenialHook, if set, is invoked whenever a Check*Allowed function (or
// ExitAllowed/EnvWriteAllowed/EnvReadAllowed) denies an operation. category is
// one of the Capability* constants (or a more specific string such as "exec",
// "network", "db", "file_read", "file_write", "import", "native_module",
// "env_read", "env_write" for checks that layer additional rules on top of a
// capability check); detail is a short human-readable reason. This is a
// single process-wide hook (see NewRuntime in runtime.go for the tradeoff
// this implies when multiple Runtimes exist concurrently).
var DenialHook func(category, detail string)

func notifyDenial(category, detail string) {
	if DenialHook != nil {
		DenialHook(category, detail)
	}
}

func ActiveSecurityPolicy() *SecurityPolicy {
	p := policyOverride.current.Load()
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

const (
	CapabilityAsync           = "async"
	CapabilityDB              = "db"
	CapabilityEnvRead         = "env_read"
	CapabilityEnvWrite        = "env_write"
	CapabilityExec            = "exec"
	CapabilityFilesystemRead  = "filesystem_read"
	CapabilityFilesystemWrite = "filesystem_write"
	CapabilityNetwork         = "network"
	CapabilityPolicy          = "policy"
	CapabilityProcessExit     = "process_exit"
	CapabilityScheduler       = "scheduler"
	CapabilityServer          = "server"
	CapabilitySecrets         = "secrets"
	CapabilitySystem          = "system"
	CapabilityWatch           = "watch"
)

func CheckCapabilityAllowed(capability string) error {
	p := ActiveSecurityPolicy()
	capability = strings.ToLower(strings.TrimSpace(capability))
	if capability == "" {
		return fmt.Errorf("empty capability")
	}
	if p.ProtectHost && hostProtectedCapability(capability) && !ContainsToken(p.AllowedCapabilities, capability) {
		if capability == CapabilityExec {
			detail := "exec denied by host protection policy"
			notifyDenial(capability, detail)
			return fmt.Errorf("%s", detail)
		}
		detail := fmt.Sprintf("capability denied by host protection policy: %s", capability)
		notifyDenial(capability, detail)
		return fmt.Errorf("%s", detail)
	}
	if ContainsToken(p.DeniedCapabilities, capability) {
		detail := fmt.Sprintf("capability denied by policy: %s", capability)
		notifyDenial(capability, detail)
		return fmt.Errorf("%s", detail)
	}
	if len(p.AllowedCapabilities) > 0 && !ContainsToken(p.AllowedCapabilities, capability) {
		detail := fmt.Sprintf("capability not allowed by policy: %s", capability)
		notifyDenial(capability, detail)
		return fmt.Errorf("%s", detail)
	}
	return nil
}

func hostProtectedCapability(capability string) bool {
	switch capability {
	case CapabilityAsync,
		CapabilityDB,
		CapabilityEnvRead,
		CapabilityEnvWrite,
		CapabilityExec,
		CapabilityFilesystemWrite,
		CapabilityNetwork,
		CapabilityPolicy,
		CapabilityProcessExit,
		CapabilityScheduler,
		CapabilityServer,
		CapabilitySecrets,
		CapabilitySystem,
		CapabilityWatch:
		return true
	default:
		return false
	}
}

// ---------------------------------------------------------------------------
// Pluggable hardcoded-secret source scanning
// ---------------------------------------------------------------------------

// secretScanner is supplied by an optional plugin (e.g. builtins/secretr) via
// RegisterSecretScanner. It receives raw SPL source text and returns a short,
// human-readable description of each apparent hardcoded secret it found
// (already redacted by the scanner - callers must not assume the strings are
// safe to log verbatim beyond what the scanner itself returns).
var secretScanner func(src string) ([]string, error)

// RegisterSecretScanner installs the hardcoded-secret detector consulted by
// ScanForHardcodedSecrets. Intended to be called from an optional plugin's
// init(); the last registration wins. Passing nil clears the scanner.
func RegisterSecretScanner(fn func(src string) ([]string, error)) {
	secretScanner = fn
}

// HasSecretScanner reports whether a scanner has been registered.
func HasSecretScanner() bool {
	return secretScanner != nil
}

// ScanForHardcodedSecrets runs the registered secret scanner over src, but
// only when scanning is opted into - via policy.BlockHardcodedSecrets, or
// via SPL_BLOCK_HARDCODED_SECRETS=true as a global operator-level toggle -
// so merely linking a scanner plugin never changes behavior for callers who
// haven't asked for it. Returns (nil, nil) when scanning is off or no
// scanner is registered.
//
// policy is taken as an explicit parameter rather than read from
// ActiveSecurityPolicy() because callers typically need to run this check
// before the ambient policy override for a request is installed (e.g. the
// interpreter package checks source text before handing off to
// sandbox.RunProgramSandboxed, which is what actually installs the
// per-request override) - pass the request's already-resolved effective
// policy explicitly instead.
//
// The env var is consulted independently of policy (rather than only via
// LoadSecurityPolicyFromEnv's own BlockHardcodedSecrets field) because
// callers such as the CLI's trusted/untrusted sandbox profiles build their
// own SecurityPolicy internally without necessarily going through
// LoadSecurityPolicyFromEnv - so without this, an operator's
// SPL_BLOCK_HARDCODED_SECRETS=true would silently do nothing for those
// callers even though it works when a caller passes ExecOptions.Security
// explicitly.
func ScanForHardcodedSecrets(policy *SecurityPolicy, src string) ([]string, error) {
	if policy == nil {
		policy = ActiveSecurityPolicy()
	}
	enabled := (policy != nil && policy.BlockHardcodedSecrets) || parseBoolEnvDefault("SPL_BLOCK_HARDCODED_SECRETS", false)
	if !enabled || secretScanner == nil {
		return nil, nil
	}
	return secretScanner(src)
}

// ---------------------------------------------------------------------------
// Policy check functions
// ---------------------------------------------------------------------------

func CheckExecAllowed(cmd string) error {
	if err := CheckCapabilityAllowed(CapabilityExec); err != nil {
		return err
	}
	p := ActiveSecurityPolicy()
	if p.ProtectHost && !ContainsToken(p.AllowedCapabilities, CapabilityExec) {
		detail := "exec denied by host protection policy"
		notifyDenial("exec", detail)
		return fmt.Errorf("%s", detail)
	}
	name := strings.ToLower(strings.TrimSpace(cmd))
	base := strings.ToLower(strings.TrimSpace(filepath.Base(cmd)))
	if ContainsToken(p.DeniedExecCommands, name) || ContainsToken(p.DeniedExecCommands, base) {
		detail := fmt.Sprintf("exec denied for command %q", cmd)
		notifyDenial("exec", detail)
		return fmt.Errorf("%s", detail)
	}
	if len(p.AllowedExecCommands) > 0 {
		if !ContainsToken(p.AllowedExecCommands, name) && !ContainsToken(p.AllowedExecCommands, base) {
			detail := fmt.Sprintf("exec not allowed for command %q", cmd)
			notifyDenial("exec", detail)
			return fmt.Errorf("%s", detail)
		}
		return nil
	}
	if p.StrictMode {
		detail := "exec denied in strict security mode"
		notifyDenial("exec", detail)
		return fmt.Errorf("%s", detail)
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
	if err := CheckCapabilityAllowed(CapabilityNetwork); err != nil {
		return err
	}
	p := ActiveSecurityPolicy()
	host, err := HostFromTarget(target)
	if err != nil {
		return fmt.Errorf("invalid network target: %w", err)
	}
	for _, deny := range p.DeniedNetworkHosts {
		if MatchHostPattern(host, deny) {
			detail := fmt.Sprintf("network target denied: %s", host)
			notifyDenial("network", detail)
			return fmt.Errorf("%s", detail)
		}
	}
	if len(p.AllowedNetworkHosts) > 0 {
		for _, allow := range p.AllowedNetworkHosts {
			if MatchHostPattern(host, allow) {
				return nil
			}
		}
		detail := fmt.Sprintf("network target not allowed: %s", host)
		notifyDenial("network", detail)
		return fmt.Errorf("%s", detail)
	}
	if p.StrictMode {
		detail := "network access denied in strict security mode"
		notifyDenial("network", detail)
		return fmt.Errorf("%s", detail)
	}
	return nil
}

func CheckDBAllowed(driver string, dsn string) error {
	if err := CheckCapabilityAllowed(CapabilityDB); err != nil {
		return err
	}
	p := ActiveSecurityPolicy()
	d := strings.ToLower(strings.TrimSpace(driver))
	if ContainsToken(p.DeniedDBDrivers, d) {
		detail := fmt.Sprintf("db driver denied: %s", driver)
		notifyDenial(CapabilityDB, detail)
		return fmt.Errorf("%s", detail)
	}
	if len(p.AllowedDBDrivers) > 0 && !ContainsToken(p.AllowedDBDrivers, d) {
		detail := fmt.Sprintf("db driver not allowed: %s", driver)
		notifyDenial(CapabilityDB, detail)
		return fmt.Errorf("%s", detail)
	}
	for _, deny := range p.DeniedDBDSNPatterns {
		if strings.Contains(strings.ToLower(dsn), deny) {
			detail := "db dsn denied by policy"
			notifyDenial(CapabilityDB, detail)
			return fmt.Errorf("%s", detail)
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
			detail := "db dsn not allowed"
			notifyDenial(CapabilityDB, detail)
			return fmt.Errorf("%s", detail)
		}
	}
	if p.StrictMode && len(p.AllowedDBDrivers) == 0 {
		detail := "db access denied in strict security mode"
		notifyDenial(CapabilityDB, detail)
		return fmt.Errorf("%s", detail)
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
	if err := CheckCapabilityAllowed(CapabilityFilesystemRead); err != nil {
		return err
	}
	p := ActiveSecurityPolicy()
	if PathMatches(path, p.DeniedFileReadPaths) {
		detail := "file read denied by policy"
		notifyDenial("file_read", detail)
		return fmt.Errorf("%s", detail)
	}
	if len(p.AllowedFileReadPaths) > 0 && !PathMatches(path, p.AllowedFileReadPaths) {
		detail := "file read not allowed by policy"
		notifyDenial("file_read", detail)
		return fmt.Errorf("%s", detail)
	}
	if p.StrictMode && len(p.AllowedFileReadPaths) == 0 {
		detail := "file read denied in strict security mode"
		notifyDenial("file_read", detail)
		return fmt.Errorf("%s", detail)
	}
	return nil
}

func CheckFileWriteAllowed(path string) error {
	if err := CheckCapabilityAllowed(CapabilityFilesystemWrite); err != nil {
		return err
	}
	p := ActiveSecurityPolicy()
	if p.ProtectHost && !ContainsToken(p.AllowedCapabilities, CapabilityFilesystemWrite) {
		detail := "file mutation denied by host protection policy"
		notifyDenial("file_write", detail)
		return fmt.Errorf("%s", detail)
	}
	if PathMatches(path, p.DeniedFileWritePaths) {
		detail := "file write denied by policy"
		notifyDenial("file_write", detail)
		return fmt.Errorf("%s", detail)
	}
	if len(p.AllowedFileWritePaths) > 0 && !PathMatches(path, p.AllowedFileWritePaths) {
		detail := "file write not allowed by policy"
		notifyDenial("file_write", detail)
		return fmt.Errorf("%s", detail)
	}
	if p.StrictMode && len(p.AllowedFileWritePaths) == 0 {
		detail := "file write denied in strict security mode"
		notifyDenial("file_write", detail)
		return fmt.Errorf("%s", detail)
	}
	return nil
}

func CheckImportAllowed(importPath string, resolvedPath string) error {
	p := ActiveSecurityPolicy()
	if p == nil {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(importPath))
	if ContainsToken(p.DeniedImportPackages, name) {
		detail := fmt.Sprintf("import package denied by policy: %s", importPath)
		notifyDenial("import", detail)
		return fmt.Errorf("%s", detail)
	}
	if len(p.AllowedImportPackages) > 0 && !ContainsToken(p.AllowedImportPackages, name) {
		detail := fmt.Sprintf("import package not allowed by policy: %s", importPath)
		notifyDenial("import", detail)
		return fmt.Errorf("%s", detail)
	}
	if PathMatches(resolvedPath, p.DeniedImportPaths) {
		detail := "import path denied by policy"
		notifyDenial("import", detail)
		return fmt.Errorf("%s", detail)
	}
	if len(p.AllowedImportPaths) > 0 && !PathMatches(resolvedPath, p.AllowedImportPaths) {
		detail := "import path not allowed by policy"
		notifyDenial("import", detail)
		return fmt.Errorf("%s", detail)
	}
	return CheckFileReadAllowed(resolvedPath)
}

func CheckNativeModuleAllowed(moduleName string) error {
	p := ActiveSecurityPolicy()
	if p == nil {
		return nil
	}
	name := strings.ToLower(strings.TrimSpace(moduleName))
	if name == "" {
		return fmt.Errorf("empty native module")
	}
	if ContainsToken(p.DeniedNativeModules, name) {
		detail := fmt.Sprintf("native module denied by policy: %s", moduleName)
		notifyDenial("native_module", detail)
		return fmt.Errorf("%s", detail)
	}
	if len(p.AllowedNativeModules) > 0 && !ContainsToken(p.AllowedNativeModules, name) {
		detail := fmt.Sprintf("native module not allowed by policy: %s", moduleName)
		notifyDenial("native_module", detail)
		return fmt.Errorf("%s", detail)
	}
	return nil
}

func ExitAllowed() error {
	if err := CheckCapabilityAllowed(CapabilityProcessExit); err != nil {
		return err
	}
	p := ActiveSecurityPolicy()
	if p.ProtectHost && !ContainsToken(p.AllowedCapabilities, CapabilityProcessExit) {
		detail := "process exit denied by host protection policy"
		notifyDenial(CapabilityProcessExit, detail)
		return fmt.Errorf("%s", detail)
	}
	return nil
}

func EnvWriteAllowed(key string) error {
	if err := CheckCapabilityAllowed(CapabilityEnvWrite); err != nil {
		return err
	}
	p := ActiveSecurityPolicy()
	if p.ProtectHost && !ContainsToken(p.AllowedCapabilities, CapabilityEnvWrite) {
		detail := "environment writes are disabled by host protection policy"
		notifyDenial(CapabilityEnvWrite, detail)
		return fmt.Errorf("%s", detail)
	}
	upperKey := strings.ToUpper(strings.TrimSpace(key))
	if strings.HasPrefix(upperKey, "SPL_") || isProtectedEnvVar(upperKey) {
		detail := fmt.Sprintf("refusing to mutate protected %s environment variable", key)
		notifyDenial(CapabilityEnvWrite, detail)
		return fmt.Errorf("%s", detail)
	}
	if !p.AllowEnvWrite {
		detail := "environment writes are disabled by security policy"
		notifyDenial(CapabilityEnvWrite, detail)
		return fmt.Errorf("%s", detail)
	}
	return nil
}

// protectedEnvVarNames are environment variables that, if mutated, could be
// used to bypass exec allow/deny lists or other security controls (e.g. PATH
// hijacking, dynamic-linker preloading). They are protected regardless of the
// SPL_ prefix check.
var protectedEnvVarNames = map[string]bool{
	"PATH":                  true,
	"LD_PRELOAD":            true,
	"LD_LIBRARY_PATH":       true,
	"DYLD_LIBRARY_PATH":     true,
	"DYLD_INSERT_LIBRARIES": true,
}

func isProtectedEnvVar(upperKey string) bool {
	return protectedEnvVarNames[upperKey]
}

// secretEnvKeyPatterns are case-insensitive substrings that, when present in
// an environment variable name, mark it as likely to hold a credential.
var secretEnvKeyPatterns = []string{"SECRET", "TOKEN", "PASSWORD", "API_KEY", "PRIVATE_KEY"}

func looksLikeSecretEnvKey(key string) bool {
	upperKey := strings.ToUpper(strings.TrimSpace(key))
	if strings.HasPrefix(upperKey, "SPL_") {
		return true
	}
	for _, pattern := range secretEnvKeyPatterns {
		if strings.Contains(upperKey, pattern) {
			return true
		}
	}
	return false
}

// EnvReadAllowed reports whether reading the given environment variable is
// permitted under the active security policy. Under ProtectHost/StrictMode
// (the untrusted/hardened profiles), keys that look like they hold secrets
// are denied by default; an embedding host must explicitly allowlist
// CapabilityEnvRead AND disable StrictMode to read such keys. The default
// trusted profile (ProtectHost=false, StrictMode=false) keeps prior
// behavior and allows all reads, for backward compatibility.
func EnvReadAllowed(key string) error {
	if err := CheckCapabilityAllowed(CapabilityEnvRead); err != nil {
		return err
	}
	p := ActiveSecurityPolicy()
	if p.ProtectHost && !ContainsToken(p.AllowedCapabilities, CapabilityEnvRead) {
		detail := "environment reads are disabled by host protection policy"
		notifyDenial(CapabilityEnvRead, detail)
		return fmt.Errorf("%s", detail)
	}
	if (p.ProtectHost || p.StrictMode) && looksLikeSecretEnvKey(key) {
		allowlisted := ContainsToken(p.AllowedCapabilities, CapabilityEnvRead)
		if p.StrictMode || !allowlisted {
			detail := fmt.Sprintf("refusing to read potentially sensitive environment variable %q", key)
			notifyDenial(CapabilityEnvRead, detail)
			return fmt.Errorf("%s", detail)
		}
	}
	return nil
}

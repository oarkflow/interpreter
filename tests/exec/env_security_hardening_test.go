package interpreter_test

import (
	"strings"
	"testing"

	. "github.com/oarkflow/interpreter"
)

// TestSPLProtectHostEnvVarRestrictsDefaultTrustedExec is a regression test
// for a previously-documented gap (docs/features/44-security-policy-and-
// sandboxed-execution.md): a normal script run under the default trusted
// profile (no --profile flag, no explicit ExecOptions.Security) always
// constructed its own permissive sandbox SecurityPolicy override
// (AllowEnvWrite: true, ProtectHost: false) regardless of SPL_PROTECT_HOST/
// SPL_SECURITY_MODE, since DefaultExecSandboxConfig hardcoded those fields
// without consulting the environment. Setting SPL_PROTECT_HOST=1 therefore
// had no observable effect on a default run. DefaultExecSandboxConfig (and
// CapabilityPreset's "trusted" branch, kept in sync) now apply
// applyEnvSecurityHardening on top of their defaults.
func TestSPLProtectHostEnvVarRestrictsDefaultTrustedExec(t *testing.T) {
	// Baseline: with no env var set, a default trusted run still allows
	// exec (unaffected by this fix - it must stay permissive by default).
	_, err := ExecWithOptions(`exec("echo", "hi")`, nil, ExecOptions{})
	if err != nil {
		t.Fatalf("expected exec to succeed under default trusted profile with no env hardening, got: %v", err)
	}

	t.Setenv("SPL_PROTECT_HOST", "1")
	_, err = ExecWithOptions(`exec("echo", "hi")`, nil, ExecOptions{})
	if err == nil {
		t.Fatalf("expected exec to be denied under default trusted profile once SPL_PROTECT_HOST=1 is set")
	}
	if !strings.Contains(err.Error(), "exec denied") && !strings.Contains(err.Error(), "host protection") {
		t.Fatalf("expected a host-protection denial error, got: %v", err)
	}
}

// TestSPLProtectHostEnvVarDoesNotAffectExplicitSecurityPolicy confirms an
// embedder-supplied ExecOptions.Security still takes precedence over
// ambient env vars, unchanged by this fix - CapabilityPreset (and the env
// hardening inside it) is only consulted when opts.Security is nil.
func TestSPLProtectHostEnvVarDoesNotAffectExplicitSecurityPolicy(t *testing.T) {
	t.Setenv("SPL_PROTECT_HOST", "1")
	_, err := ExecWithOptions(`exec("echo", "hi")`, nil, ExecOptions{
		Security: &SecurityPolicy{AllowEnvWrite: true},
	})
	if err != nil {
		t.Fatalf("expected explicit ExecOptions.Security to override env hardening, got: %v", err)
	}
}

// TestCapabilityPresetTrustedSyncsPolicyWithEnvHardening is a narrower unit
// check on CapabilityPreset itself: the returned *SecurityPolicy (not just
// the SandboxConfig) must reflect SPL_SECURITY_MODE/SPL_PROTECT_HOST, since
// the policy - not the sandbox config - is what capability checks actually
// enforce.
func TestCapabilityPresetTrustedSyncsPolicyWithEnvHardening(t *testing.T) {
	policy, cfg, err := CapabilityPreset("trusted", ".")
	if err != nil {
		t.Fatalf("CapabilityPreset failed: %v", err)
	}
	if policy.StrictMode || policy.ProtectHost || cfg.StrictMode || cfg.ProtectHost {
		t.Fatalf("expected no hardening with no env vars set, got policy=%+v cfg.StrictMode=%v cfg.ProtectHost=%v", policy, cfg.StrictMode, cfg.ProtectHost)
	}

	t.Setenv("SPL_SECURITY_MODE", "strict")
	t.Setenv("SPL_PROTECT_HOST", "1")
	policy, cfg, err = CapabilityPreset("trusted", ".")
	if err != nil {
		t.Fatalf("CapabilityPreset failed: %v", err)
	}
	if !policy.StrictMode || !policy.ProtectHost {
		t.Fatalf("expected policy to reflect env hardening, got StrictMode=%v ProtectHost=%v", policy.StrictMode, policy.ProtectHost)
	}
	if !cfg.StrictMode || !cfg.ProtectHost {
		t.Fatalf("expected sandbox config to reflect env hardening, got StrictMode=%v ProtectHost=%v", cfg.StrictMode, cfg.ProtectHost)
	}
}

package playgroundserver

import (
	"strings"

	"github.com/oarkflow/interpreter"
	"github.com/oarkflow/interpreter/pkg/security"
)

// securityPolicyForProfile builds the execution policy for one /api/execute
// request. extraCapabilities lets a Variant grant additional untrusted-
// profile capabilities beyond the shared baseline (e.g. cmd/interpreter's
// --playground mode adds CapabilityDB, CapabilityNetwork, and
// CapabilityPolicy so its "query-builder"/"image-values"/"rules"/
// "tcpguard" examples can actually touch SQLite/image codecs/policy
// engines) without duplicating this whole function per Variant.
func securityPolicyForProfile(root, profile string, renderAllowURLs bool, renderURLHosts []string, extraCapabilities []string) *interpreter.SecurityPolicy {
	if strings.EqualFold(strings.TrimSpace(profile), "trusted") {
		policy := &interpreter.SecurityPolicy{AllowEnvWrite: true}
		if renderAllowURLs {
			policy.AllowedNetworkHosts = append(policy.AllowedNetworkHosts, renderURLHosts...)
		}
		return policy
	}
	capabilities := []string{security.CapabilityFilesystemRead, security.CapabilityAsync, security.CapabilityScheduler, security.CapabilityServer}
	capabilities = append(capabilities, extraCapabilities...)
	policy := &interpreter.SecurityPolicy{
		ProtectHost:         true,
		AllowedCapabilities: capabilities,
	}
	if strings.TrimSpace(root) != "" {
		policy.AllowedFileReadPaths = append(policy.AllowedFileReadPaths, root)
	}
	if renderAllowURLs {
		policy.AllowedCapabilities = append(policy.AllowedCapabilities, security.CapabilityNetwork)
		policy.AllowedNetworkHosts = append(policy.AllowedNetworkHosts, renderURLHosts...)
	}
	return policy
}

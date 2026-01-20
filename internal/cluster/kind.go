package cluster

import (
	"strings"

	"github.com/stackrox/roxie/internal/env"
)

// IsKindCluster returns true if the current kubectl context is a kind cluster.
func IsKindCluster() bool {
	contextName := env.GetCurrentContext()
	return isKindContext(contextName)
}

// isKindContext checks if the given context name indicates a kind cluster.
// Exported for testing.
func isKindContext(contextName string) bool {
	if contextName == "" {
		return false
	}
	// Kind clusters have contexts starting with "kind" (case-insensitive)
	return strings.HasPrefix(strings.ToLower(contextName), "kind")
}

// ExtractKindClusterName extracts the cluster name from a kind context.
// For context "kind-acs", returns "acs".
// For context "kind", returns "kind".
func ExtractKindClusterName(contextName string) string {
	// Remove "kind-" prefix if present
	if len(contextName) > 5 && strings.HasPrefix(strings.ToLower(contextName), "kind-") {
		return contextName[5:]
	}
	// If just "kind", return as-is
	return contextName
}

// GetKindClusterName returns the kind cluster name for the current context.
// Returns empty string if not a kind cluster.
func GetKindClusterName() string {
	if !IsKindCluster() {
		return ""
	}
	return ExtractKindClusterName(env.GetCurrentContext())
}

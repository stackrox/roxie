package deployer

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/stackrox/roxie/internal/helpers"
)

const (
	operatorNamespaceSystem  = "rhacs-operator-system"
	operatorNamespaceCentral = "rhacs-operator-central"
	operatorNamespaceSensor  = "rhacs-operator-sensor"

	envCentralReconcilerEnabled        = "CENTRAL_RECONCILER_ENABLED"
	envSecuredClusterReconcilerEnabled = "SECURED_CLUSTER_RECONCILER_ENABLED"
)

// AllOperatorNamespaces lists every namespace where roxie may deploy an operator.
var AllOperatorNamespaces = []string{
	operatorNamespaceSystem,
	operatorNamespaceCentral,
	operatorNamespaceSensor,
}

// OperatorInstance describes a single operator instance (single- or mixed-version mode).
type OperatorInstance struct {
	Version   string
	Namespace string
	EnvVars   map[string]string
	// RoleNameSuffix is appended to cluster-scoped RBAC resource names.
	// Empty for the single-operator (rhacs-operator-system) case.
	RoleNameSuffix string
}

// ClusterRoleName returns the ClusterRole name for this operator instance.
func (o OperatorInstance) ClusterRoleName() string {
	if o.RoleNameSuffix == "" {
		return "rhacs-operator-manager-role"
	}
	return "rhacs-operator-manager-role-" + o.RoleNameSuffix
}

// ClusterRoleBindingName returns the ClusterRoleBinding name for this operator instance.
func (o OperatorInstance) ClusterRoleBindingName() string {
	if o.RoleNameSuffix == "" {
		return "rhacs-operator-manager-rolebinding"
	}
	return "rhacs-operator-manager-rolebinding-" + o.RoleNameSuffix
}

// CentralVersion returns the main image tag used for Central.
// Uses central.operator.version if set, otherwise falls back to roxie.version.
func (c *Config) CentralVersion() string {
	if c.Central.Operator.Version != "" {
		return c.Central.Operator.Version
	}
	return c.Roxie.Version
}

// SecuredClusterVersion returns the main image tag used for SecuredCluster.
// Uses securedCluster.operator.version if set, otherwise falls back to roxie.version.
func (c *Config) SecuredClusterVersion() string {
	if c.SecuredCluster.Operator.Version != "" {
		return c.SecuredCluster.Operator.Version
	}
	return c.Roxie.Version
}

// HasMixedVersions reports whether Central and SecuredCluster use different operator versions.
// This is true when at least one component has a per-component operator config with a version
// that differs from the other component's effective version.
func (c *Config) HasMixedVersions() bool {
	return c.CentralVersion() != c.SecuredClusterVersion()
}

// OperatorInstances builds the operator deployment plan for this config.
// When versions match, a single operator is deployed to rhacs-operator-system.
// When they differ, two operators are deployed with reconciler toggles.
func (c *Config) OperatorInstances() []OperatorInstance {
	if !c.HasMixedVersions() {
		return []OperatorInstance{{
			Version:   helpers.ConvertMainTagToOperatorTag(c.CentralVersion()),
			Namespace: operatorNamespaceSystem,
			EnvVars:   maps.Clone(c.Operator.EnvVars),
		}}
	}

	centralEnvVars := make(map[string]string, len(c.Central.Operator.EnvVars)+1)
	maps.Copy(centralEnvVars, c.Central.Operator.EnvVars)
	centralEnvVars[envSecuredClusterReconcilerEnabled] = "false"

	sensorEnvVars := make(map[string]string, len(c.SecuredCluster.Operator.EnvVars)+1)
	maps.Copy(sensorEnvVars, c.SecuredCluster.Operator.EnvVars)
	sensorEnvVars[envCentralReconcilerEnabled] = "false"

	return []OperatorInstance{
		{
			Version:        helpers.ConvertMainTagToOperatorTag(c.CentralVersion()),
			Namespace:      operatorNamespaceCentral,
			EnvVars:        centralEnvVars,
			RoleNameSuffix: "central",
		},
		{
			Version:        helpers.ConvertMainTagToOperatorTag(c.SecuredClusterVersion()),
			Namespace:      operatorNamespaceSensor,
			EnvVars:        sensorEnvVars,
			RoleNameSuffix: "sensor",
		},
	}
}

// NewestOperatorVersion returns the highest operator version among planned instances.
// CRDs should always be installed from this version so an older companion operator
// cannot leave the cluster on a stale (or downgraded) CRD schema.
//
// Ordering uses the leading semver of each tag (suffix after "-" is ignored), which
// is sufficient for release-vs-release compat testing (e.g. 4.8.x vs 4.9.x).
func (c *Config) NewestOperatorVersion() string {
	newest := slices.MaxFunc(c.OperatorInstances(), func(a, b OperatorInstance) int {
		av, aerr := parseOperatorSemver(a.Version)
		bv, berr := parseOperatorSemver(b.Version)
		if aerr == nil && berr == nil {
			return av.Compare(bv)
		}
		return cmp.Compare(a.Version, b.Version)
	})
	return newest.Version
}

func parseOperatorSemver(version string) (*semver.Version, error) {
	// Leading semver only; see NewestOperatorVersion.
	version, _, _ = strings.Cut(version, "-")
	return semver.NewVersion(version)
}

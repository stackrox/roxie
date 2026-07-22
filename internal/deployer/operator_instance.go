package deployer

import (
	"maps"
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

// OperatorInstance describes one operator deployment (single- or mixed-version).
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

// EffectiveCentralVersion returns the main image tag used for Central.
// If central.operator.version is set, it is converted back to a main tag;
// otherwise falls back to roxie.version.
func (c *Config) EffectiveCentralVersion() string {
	if c.Central.Operator != nil && c.Central.Operator.Version != "" {
		return c.Central.Operator.Version
	}
	return c.Roxie.Version
}

// EffectiveSecuredClusterVersion returns the main image tag used for SecuredCluster.
// If securedCluster.operator.version is set, it is converted back to a main tag;
// otherwise falls back to roxie.version.
func (c *Config) EffectiveSecuredClusterVersion() string {
	if c.SecuredCluster.Operator != nil && c.SecuredCluster.Operator.Version != "" {
		return c.SecuredCluster.Operator.Version
	}
	return c.Roxie.Version
}

// HasMixedVersions reports whether Central and SecuredCluster use different operator versions.
// This is true when at least one component has a per-component operator config with a version
// that differs from the other component's effective version.
func (c *Config) HasMixedVersions() bool {
	return c.EffectiveCentralVersion() != c.EffectiveSecuredClusterVersion()
}

// OperatorInstances builds the operator deployment plan for this config.
// When versions match, a single operator is deployed to rhacs-operator-system.
// When they differ, two operators are deployed with reconciler toggles.
func (c *Config) OperatorInstances() []OperatorInstance {
	if !c.HasMixedVersions() {
		version := helpers.ConvertMainTagToOperatorTag(c.EffectiveCentralVersion())
		if version == "" {
			version = c.Operator.Version
		}
		envVars := copyStringMap(c.Operator.EnvVars)
		if c.Central.Operator != nil {
			for k, v := range c.Central.Operator.EnvVars {
				envVars[k] = v
			}
		}
		return []OperatorInstance{{
			Version:   version,
			Namespace: operatorNamespaceSystem,
			EnvVars:   envVars,
		}}
	}

	centralEnvVars := copyStringMap(c.Operator.EnvVars)
	if c.Central.Operator != nil {
		for k, v := range c.Central.Operator.EnvVars {
			centralEnvVars[k] = v
		}
	}
	centralEnvVars[envSecuredClusterReconcilerEnabled] = "false"

	sensorEnvVars := copyStringMap(c.Operator.EnvVars)
	if c.SecuredCluster.Operator != nil {
		for k, v := range c.SecuredCluster.Operator.EnvVars {
			sensorEnvVars[k] = v
		}
	}
	sensorEnvVars[envCentralReconcilerEnabled] = "false"

	return []OperatorInstance{
		{
			Version:        helpers.ConvertMainTagToOperatorTag(c.EffectiveCentralVersion()),
			Namespace:      operatorNamespaceCentral,
			EnvVars:        centralEnvVars,
			RoleNameSuffix: "central",
		},
		{
			Version:        helpers.ConvertMainTagToOperatorTag(c.EffectiveSecuredClusterVersion()),
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
	instances := c.OperatorInstances()
	if len(instances) == 0 {
		return c.Operator.Version
	}
	newest := instances[0].Version
	for _, inst := range instances[1:] {
		if operatorVersionGreater(inst.Version, newest) {
			newest = inst.Version
		}
	}
	return newest
}

// operatorVersionGreater reports whether a is a newer operator tag than b.
func operatorVersionGreater(a, b string) bool {
	av, aerr := parseOperatorSemver(a)
	bv, berr := parseOperatorSemver(b)
	if aerr == nil && berr == nil {
		return av.GreaterThan(bv)
	}
	// Fall back to lexicographic compare when tags are not parseable as semver.
	return a > b
}

func parseOperatorSemver(version string) (*semver.Version, error) {
	// Leading semver only; see NewestOperatorVersion.
	version, _, _ = strings.Cut(version, "-")
	return semver.NewVersion(version)
}

func copyStringMap(m map[string]string) map[string]string {
	if m == nil {
		return make(map[string]string)
	}
	return maps.Clone(m)
}

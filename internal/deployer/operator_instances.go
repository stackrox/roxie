package deployer

import (
	"cmp"
	"maps"
	"slices"
	"strings"

	"github.com/Masterminds/semver/v3"
	"github.com/stackrox/roxie/internal/imagetag"
)

const (
	operatorNamespaceSystem  = "rhacs-operator-system"
	operatorNamespaceCentral = "rhacs-operator-central"
	operatorNamespaceSensor  = "rhacs-operator-sensor"

	roleNameSuffixCentral = "central"
	roleNameSuffixSensor  = "sensor"

	envCentralReconcilerEnabled        = "CENTRAL_RECONCILER_ENABLED"
	envSecuredClusterReconcilerEnabled = "SECURED_CLUSTER_RECONCILER_ENABLED"
)

// AllOperatorNamespaces lists every namespace where roxie may deploy an operator.
var AllOperatorNamespaces = []string{
	operatorNamespaceSystem,
	operatorNamespaceCentral,
	operatorNamespaceSensor,
}

// CentralVersion returns the main image tag for Central.
// Uses central.operator.version if set, otherwise falls back to operator.version.
func (c *Config) CentralVersion() imagetag.MainTag {
	if c.Central.Operator.Version != "" {
		return c.Central.Operator.Version
	}
	return c.Operator.Version
}

// SecuredClusterVersion returns the main image tag for SecuredCluster.
// Uses securedCluster.operator.version if set, otherwise falls back to operator.version.
func (c *Config) SecuredClusterVersion() imagetag.MainTag {
	if c.SecuredCluster.Operator.Version != "" {
		return c.SecuredCluster.Operator.Version
	}
	return c.Operator.Version
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
func (c *Config) OperatorInstances() []OperatorInstanceConfig {
	if !c.HasMixedVersions() {
		return []OperatorInstanceConfig{{
			Version:       c.CentralVersion(), // Per-component overrides may both equal each other but differ from Operator.Version.
			Namespace:     operatorNamespaceSystem,
			EnvVars:       maps.Clone(c.Operator.EnvVars),
			KonfluxImages: c.resolveKonfluxImages(&c.Operator.OperatorInstanceConfig),
		}}
	}

	centralOperatorEnvVars := make(map[string]string, len(c.Central.Operator.EnvVars)+1)
	maps.Copy(centralOperatorEnvVars, c.Central.Operator.EnvVars)
	centralOperatorEnvVars[envSecuredClusterReconcilerEnabled] = "false"

	sensorOperatorEnvVars := make(map[string]string, len(c.SecuredCluster.Operator.EnvVars)+1)
	maps.Copy(sensorOperatorEnvVars, c.SecuredCluster.Operator.EnvVars)
	sensorOperatorEnvVars[envCentralReconcilerEnabled] = "false"

	return []OperatorInstanceConfig{
		{
			Version:        c.CentralVersion(),
			Namespace:      operatorNamespaceCentral,
			EnvVars:        centralOperatorEnvVars,
			RoleNameSuffix: roleNameSuffixCentral,
			KonfluxImages:  c.resolveKonfluxImages(&c.Central.Operator),
		},
		{
			Version:        c.SecuredClusterVersion(),
			Namespace:      operatorNamespaceSensor,
			EnvVars:        sensorOperatorEnvVars,
			RoleNameSuffix: roleNameSuffixSensor,
			KonfluxImages:  c.resolveKonfluxImages(&c.SecuredCluster.Operator),
		},
	}
}

// resolveKonfluxImages returns the effective KonfluxImages setting for a per-component
// operator config, falling back to Operator.KonfluxImages if not set at the component level.
func (c *Config) resolveKonfluxImages(instanceCfg *OperatorInstanceConfig) *bool {
	if instanceCfg.KonfluxImagesSet() {
		return instanceCfg.KonfluxImages
	}
	return c.Operator.KonfluxImages
}

// NewestOperatorInstance returns the operator instance with the highest version.
// This is used to determine which bundle to use for CRD installation, since CRDs
// are cluster-scoped and must be at the newest version.
//
// Comparison uses the leading semver (everything before the first "-" in the operator
// tag), which is sufficient for release-vs-release compat testing (e.g. 4.8.x vs 4.9.x).
func (c *Config) NewestOperatorInstance() OperatorInstanceConfig {
	instances := c.OperatorInstances()
	return slices.MaxFunc(instances, func(a, b OperatorInstanceConfig) int {
		av, aerr := parseLeadingSemver(a.Version)
		bv, berr := parseLeadingSemver(b.Version)
		if aerr == nil && berr == nil {
			return av.Compare(bv)
		}
		return cmp.Compare(a.Version.ToOperatorTag().String(), b.Version.ToOperatorTag().String())
	})
}

// NewestOperatorVersion returns the highest operator tag across all planned instances.
func (c *Config) NewestOperatorVersion() imagetag.OperatorTag {
	return c.NewestOperatorInstance().Version.ToOperatorTag()
}

func parseLeadingSemver(version imagetag.MainTag) (*semver.Version, error) {
	tag := version.ToOperatorTag()
	base, _, _ := strings.Cut(tag.String(), "-")
	return semver.NewVersion(base)
}

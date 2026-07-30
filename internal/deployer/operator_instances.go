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

// CentralVersion returns the main image tag for Central.
// Uses central.operator.version if set, otherwise falls back to roxie.version.
func (c *Config) CentralVersion() string {
	if c.Central.Operator.Version != "" {
		return c.Central.Operator.Version
	}
	return c.Roxie.Version
}

// SecuredClusterVersion returns the main image tag for SecuredCluster.
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
func (c *Config) OperatorInstances() []OperatorInstanceConfig {
	if !c.HasMixedVersions() {
		return []OperatorInstanceConfig{{
			Version:       c.CentralVersion(),
			Namespace:     operatorNamespaceSystem,
			EnvVars:       maps.Clone(c.Operator.EnvVars),
			KonfluxImages: c.resolveKonfluxImages(&c.Operator.OperatorInstanceConfig),
		}}
	}

	centralEnvVars := make(map[string]string, len(c.Central.Operator.EnvVars)+1)
	maps.Copy(centralEnvVars, c.Central.Operator.EnvVars)
	centralEnvVars[envSecuredClusterReconcilerEnabled] = "false"

	sensorEnvVars := make(map[string]string, len(c.SecuredCluster.Operator.EnvVars)+1)
	maps.Copy(sensorEnvVars, c.SecuredCluster.Operator.EnvVars)
	sensorEnvVars[envCentralReconcilerEnabled] = "false"

	return []OperatorInstanceConfig{
		{
			Version:        c.CentralVersion(),
			Namespace:      operatorNamespaceCentral,
			EnvVars:        centralEnvVars,
			RoleNameSuffix: "central",
			KonfluxImages:  c.resolveKonfluxImages(&c.Central.Operator),
		},
		{
			Version:        c.SecuredClusterVersion(),
			Namespace:      operatorNamespaceSensor,
			EnvVars:        sensorEnvVars,
			RoleNameSuffix: "sensor",
			KonfluxImages:  c.resolveKonfluxImages(&c.SecuredCluster.Operator),
		},
	}
}

// resolveKonfluxImages returns the effective KonfluxImages setting for a per-component
// operator config, falling back to Roxie.KonfluxImages if not set at the component level.
func (c *Config) resolveKonfluxImages(instanceCfg *OperatorInstanceConfig) *bool {
	if instanceCfg.KonfluxImagesSet() {
		return instanceCfg.KonfluxImages
	}
	return c.Roxie.KonfluxImages
}

// NewestOperatorVersion returns the highest operator version among planned instances.
// CRDs should always be installed from this version so an older companion operator
// cannot leave the cluster on a stale (or downgraded) CRD schema.
//
// Comparison uses the leading semver (everything before the first "-" in the operator
// tag), which is sufficient for release-vs-release compat testing (e.g. 4.8.x vs 4.9.x).
func (c *Config) NewestOperatorVersion() string {
	instances := c.OperatorInstances()
	newest := slices.MaxFunc(instances, func(a, b OperatorInstanceConfig) int {
		av, aerr := parseLeadingSemver(a.Version)
		bv, berr := parseLeadingSemver(b.Version)
		if aerr == nil && berr == nil {
			return av.Compare(bv)
		}
		return cmp.Compare(a.Version, b.Version)
	})
	return helpers.ConvertToOperatorTag(newest.Version)
}

func parseLeadingSemver(version string) (*semver.Version, error) {
	tag := helpers.ConvertToOperatorTag(version)
	base, _, _ := strings.Cut(tag, "-")
	return semver.NewVersion(base)
}

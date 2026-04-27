package deployer

import (
	"fmt"
	"time"

	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/types"
)

// This is the self-contained configuration for deployments.
type Config struct {
	Roxie          RoxieConfig          `yaml:"roxie"`
	Operator       OperatorConfig       `yaml:"operator"`
	Central        CentralConfig        `yaml:"central"`
	SecuredCluster SecuredClusterConfig `yaml:"securedCluster"`
}

func (c *CentralConfig) CustomResource() (map[string]interface{}, error) {
	cr := map[string]interface{}{
		"apiVersion": "platform.stackrox.io/v1alpha1",
		"kind":       "Central",
		"metadata": map[string]interface{}{
			"name":      "stackrox-central-services",
			"namespace": c.Namespace,
			"labels": map[string]string{
				"app": "stackrox-central",
			},
		},
		"spec": map[string]interface{}{
			"central": map[string]interface{}{
				"adminPasswordSecret": map[string]interface{}{
					"name": adminPasswordSecretName,
				},
			},
		},
	}
	if c.ResourceProfile == types.ResourceProfileAuto {
		return nil, fmt.Errorf("resource profile 'auto' must have been resolved before building the CR")
	}
	if c.ResourceProfile != types.ResourceProfileAcsDefaults {
		if err := helpers.DeepMerge(cr, getCentralResourcesOperator(c.ResourceProfile)); err != nil {
			return nil, fmt.Errorf("merging resource profile into Central CR: %w", err)
		}
	}
	if err := helpers.DeepMerge(cr, map[string]interface{}{
		"spec": c.Spec,
	}); err != nil {
		return nil, fmt.Errorf("merging spec into Central CR: %w", err)
	}
	return cr, nil
}

func (s *SecuredClusterConfig) CustomResource() (map[string]interface{}, error) {
	cr := map[string]interface{}{
		"apiVersion": "platform.stackrox.io/v1alpha1",
		"kind":       "SecuredCluster",
		"metadata": map[string]interface{}{
			"name":      "stackrox-secured-cluster-services",
			"namespace": s.Namespace,
			"labels": map[string]string{
				"app": "stackrox-secured-cluster",
			},
		},
		"spec": map[string]interface{}{
			"clusterName": generateClusterName(),
			"imagePullSecrets": []map[string]string{
				{"name": "stackrox"},
			},
		},
	}
	if s.ResourceProfile == types.ResourceProfileAuto {
		return nil, fmt.Errorf("resource profile 'auto' must have been resolved before building the CR")
	}
	if s.ResourceProfile != types.ResourceProfileAcsDefaults {
		if err := helpers.DeepMerge(cr, getSecuredClusterResourcesOperator(s.ResourceProfile)); err != nil {
			return nil, fmt.Errorf("merging resource profile into SecuredCluster CR: %w", err)
		}
	}

	if err := helpers.DeepMerge(cr, map[string]interface{}{
		"spec": s.Spec,
	}); err != nil {
		return nil, fmt.Errorf("merging spec into SecuredCluster CR: %w", err)
	}
	return cr, nil
}

type RoxieConfig struct {
	Version       string          `yaml:"version"`
	KonfluxImages bool            `yaml:"konfluxImages"`
	FeatureFlags  map[string]bool `yaml:"featureFlags"`
}

func (c *CentralConfig) ConfigureSpec(roxieConfig *RoxieConfig) error {
	err := helpers.DeepMerge(c.Spec, featureFlagsToOverrides(roxieConfig.FeatureFlags))
	if err != nil {
		return err
	}
	if err = helpers.DeepMerge(c.Spec, map[string]interface{}{
		"central": map[string]interface{}{
			"exposure": c.Exposure.ToUnstructuredConfig(),
		},
	}); err != nil {
		return err
	}
	return nil
}

func (c *SecuredClusterConfig) ConfigureSpec(roxieConfig *RoxieConfig, centralConfig *CentralConfig) error {
	if err := helpers.DeepMerge(c.Spec, featureFlagsToOverrides(roxieConfig.FeatureFlags)); err != nil {
		return err
	}

	if err := helpers.DeepMerge(c.Spec, map[string]interface{}{
		"centralEndpoint": internalCentralEndpoint(centralConfig.Namespace),
	}); err != nil {
		return err
	}

	return nil
}

func (c *Config) MergeInUnstructured(m map[string]interface{}) error {
	asMap, err := helpers.StructToMap(c)
	if err != nil {
		return err
	}
	if err := helpers.DeepMerge(asMap, m); err != nil {
		return err
	}
	return helpers.MapToStruct(asMap, c)
}

func (c *Config) MergeIn(other *Config) error {
	if other == nil {
		return nil
	}
	otherAsMap, err := helpers.StructToMap(other)
	if err != nil {
		return err
	}
	return c.MergeInUnstructured(otherAsMap)
}

type OperatorConfig struct {
	SkipDeployment bool   `yaml:"skipDeployment"`
	DeployViaOlm   bool   `yaml:"deployViaOlm"`
	Version        string `yaml:"version"`
}

func (c *OperatorConfig) Configure(roxieConfig *RoxieConfig) error {
	c.Version = helpers.ConvertMainTagToOperatorTag(roxieConfig.Version)
	return nil
}

type CentralConfig struct {
	Namespace           string                 `yaml:"namespace"`
	ResourceProfile     types.ResourceProfile  `yaml:"resourceProfile"`
	PauseReconciliation bool                   `yaml:"pauseReconciliation"`
	Exposure            types.Exposure         `yaml:"exposure"`
	DeployTimeout       time.Duration          `yaml:"deployTimeout"`
	PortForwarding      *bool                  `yaml:"portForwarding"`
	EarlyReadiness      bool                   `yaml:"earlyReadiness"`
	Spec                map[string]interface{} `yaml:"spec"`
}

func (c *CentralConfig) PortForwardingSet() bool {
	return c.PortForwarding != nil
}

func (c *CentralConfig) PortForwardingEnabled() bool {
	return c.PortForwarding != nil && *c.PortForwarding
}

type SecuredClusterConfig struct {
	Namespace           string                 `yaml:"namespace"`
	ResourceProfile     types.ResourceProfile  `yaml:"resourceProfile"`
	PauseReconciliation bool                   `yaml:"pauseReconciliation"`
	DeployTimeout       time.Duration          `yaml:"deployTimeout"`
	EarlyReadiness      bool                   `yaml:"earlyReadiness"`
	Spec                map[string]interface{} `yaml:"spec"`
}

func NewConfig() Config {
	return Config{
		Roxie:          NewRoxieConfig(),
		Central:        DefaultCentralConfig(),
		SecuredCluster: DefaultSecuredClusterConfig(),
	}
}

func NewRoxieConfig() RoxieConfig {
	return RoxieConfig{
		FeatureFlags: make(map[string]bool),
	}
}

func DefaultCentralConfig() CentralConfig {
	return CentralConfig{
		DeployTimeout: DefaultCentralWaitTimeout,
		Namespace:     "acs-central",
		Exposure:      types.ExposureLoadBalancer,
		Spec: map[string]interface{}{
			"central": map[string]interface{}{
				"telemetry": map[string]interface{}{
					"enabled": false,
				},
			},
		},
	}
}

func DefaultSecuredClusterConfig() SecuredClusterConfig {
	return SecuredClusterConfig{
		DeployTimeout: DefaultSecuredClusterWaitTimeout,
		Namespace:     "acs-sensor",
		Spec:          make(map[string]interface{}),
	}
}

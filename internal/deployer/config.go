package deployer

import (
	"fmt"
	"time"

	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/types"
	"gopkg.in/yaml.v3"
)

// Config is the top-level deployment configuration, combining settings for
// roxie itself, the operator, Central, and SecuredCluster.
type Config struct {
	Roxie          RoxieConfig          `yaml:"roxie,omitempty"`
	Operator       OperatorConfig       `yaml:"operator,omitempty"`
	Central        CentralConfig        `yaml:"central,omitempty"`
	SecuredCluster SecuredClusterConfig `yaml:"securedCluster,omitempty"`
}

// DefaultConfig returns a Config populated with default values.
func DefaultConfig() Config {
	return Config{
		Roxie:          NewRoxieConfig(),
		Central:        DefaultCentralConfig(),
		SecuredCluster: DefaultSecuredClusterConfig(),
	}
}

// NewConfig returns a Config populated with default values.
func NewConfig() Config {
	return Config{
		Roxie:          NewRoxieConfig(),
		Central:        NewCentralConfig(),
		SecuredCluster: NewSecuredClusterConfig(),
	}
}

// DeepCopy creates a deep-copy of the provided config using a YAML marshaling/unmarshaling roundtrip.
// Due the `omitempty`, this causes empty values (e.g. empty maps) from being discarded (replace with nil
// in the resulting copy).
func (c *Config) DeepCopy() (*Config, error) {
	data, err := yaml.Marshal(c)
	if err != nil {
		return nil, err
	}
	var copy Config
	if err := yaml.Unmarshal(data, &copy); err != nil {
		return nil, err
	}
	return &copy, nil
}

// RoxieConfig holds roxie-level settings such as version and feature flags.
type RoxieConfig struct {
	Version       string            `yaml:"version,omitempty"`
	KonfluxImages *bool             `yaml:"konfluxImages,omitempty"`
	FeatureFlags  map[string]bool   `yaml:"featureFlags,omitempty"`
	ClusterType   types.ClusterType `yaml:"clusterType,omitempty"`
}

func (c *RoxieConfig) KonfluxImagesSet() bool {
	return c.KonfluxImages != nil
}

func (c *RoxieConfig) KonfluxImagesEnabled() bool {
	return c.KonfluxImages != nil && *c.KonfluxImages
}

// NewRoxieConfig returns a RoxieConfig with initialized defaults.
func NewRoxieConfig() RoxieConfig {
	return RoxieConfig{
		FeatureFlags: make(map[string]bool),
	}
}

// OperatorConfig controls how the ACS operator is deployed.
type OperatorConfig struct {
	SkipDeployment *bool  `yaml:"skipDeployment,omitempty"`
	DeployViaOlm   *bool  `yaml:"deployViaOlm,omitempty"`
	Version        string `yaml:"version,omitempty"`
}

func (c *OperatorConfig) SkipDeploymentSet() bool {
	return c.SkipDeployment != nil
}

func (c *OperatorConfig) SkipDeploymentEnabled() bool {
	return c.SkipDeployment != nil && *c.SkipDeployment
}

func (c *OperatorConfig) DeployViaOlmSet() bool {
	return c.DeployViaOlm != nil
}

func (c *OperatorConfig) DeployViaOlmEnabled() bool {
	return c.DeployViaOlm != nil && *c.DeployViaOlm
}

// Configure derives the operator version from the roxie configuration.
func (c *OperatorConfig) Configure(roxieConfig *RoxieConfig) error {
	c.Version = helpers.ConvertMainTagToOperatorTag(roxieConfig.Version)
	return nil
}

// WaitConfig describes how to wait for a component to become ready.
type WaitConfig struct {
	Namespace      string
	EarlyReadiness bool
	WaitFor        string
	Timeout        time.Duration
}

// CentralConfig holds deployment settings for the Central component.
type CentralConfig struct {
	Namespace           string                 `yaml:"namespace,omitempty"`
	ResourceProfile     types.ResourceProfile  `yaml:"resourceProfile,omitempty"`
	PauseReconciliation *bool                  `yaml:"pauseReconciliation,omitempty"`
	Exposure            *types.Exposure        `yaml:"exposure,omitempty"`
	DeployTimeout       time.Duration          `yaml:"deployTimeout,omitempty"`
	PortForwarding      *bool                  `yaml:"portForwarding,omitempty"`
	EarlyReadiness      *bool                  `yaml:"earlyReadiness,omitempty"`
	Spec                map[string]interface{} `yaml:"spec,omitempty"`
}

func (c *CentralConfig) EarlyReadinessEnabled() bool {
	return c.EarlyReadiness != nil && *c.EarlyReadiness
}

// NewCentralConfig returns an emptry CentralConfig, with deep initialization of data structures.
func NewCentralConfig() CentralConfig {
	return CentralConfig{
		Spec: make(map[string]interface{}),
	}
}

// DefaultCentralConfig returns a CentralConfig with sensible defaults.
func DefaultCentralConfig() CentralConfig {
	cfg := NewCentralConfig()
	cfg.DeployTimeout = DefaultCentralWaitTimeout
	cfg.Namespace = "acs-central"
	cfg.EarlyReadiness = new(true)
	cfg.Spec["central"] = map[string]interface{}{
		"telemetry": map[string]interface{}{
			"enabled": false,
		},
	}
	return cfg
}

func (c *CentralConfig) GetWaitConfig() WaitConfig {
	// Without earlyReadiness we wait for the Available condition of component's CR to be True.
	// This indicates all deployments are ready.
	// With earlyReadiness we just wait for the Available condition of that component's core
	// Deployment to be True.
	waitFor := "central/" + centralCrName
	if c.EarlyReadinessEnabled() {
		waitFor = "deployment/central"
	}
	return WaitConfig{
		Namespace:      c.Namespace,
		EarlyReadiness: c.EarlyReadinessEnabled(),
		WaitFor:        waitFor,
		Timeout:        c.DeployTimeout,
	}
}

func (c *CentralConfig) PauseReconciliationSet() bool {
	return c.PauseReconciliation != nil
}

func (c *CentralConfig) PauseReconciliationEnabled() bool {
	return c.PauseReconciliation != nil && *c.PauseReconciliation
}

func (c *CentralConfig) EarlyReadinessSet() bool {
	return c.EarlyReadiness != nil
}

func (c *CentralConfig) PortForwardingSet() bool {
	return c.PortForwarding != nil
}

func (c *CentralConfig) PortForwardingEnabled() bool {
	return c.PortForwarding != nil && *c.PortForwarding
}

func (c *CentralConfig) ExposureSet() bool {
	return c.Exposure != nil
}

func (c *CentralConfig) ExposureEnabled() bool {
	return c.Exposure != nil && *c.Exposure != types.ExposureNone
}

func (c *CentralConfig) GetExposure() types.Exposure {
	if c.Exposure == nil {
		return types.ExposureNone
	}
	return *c.Exposure
}

// ConfigureSpec applies feature flags and exposure settings to the Central spec.
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

// CustomResource builds an unstructured Central custom resource from this config.
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
		return nil, fmt.Errorf("resource profile 'auto' should have been resolved before building the CR")
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

// SecuredClusterConfig holds deployment settings for the SecuredCluster component.
type SecuredClusterConfig struct {
	Namespace           string                 `yaml:"namespace,omitempty"`
	ResourceProfile     types.ResourceProfile  `yaml:"resourceProfile,omitempty"`
	PauseReconciliation *bool                  `yaml:"pauseReconciliation,omitempty"`
	DeployTimeout       time.Duration          `yaml:"deployTimeout,omitempty"`
	EarlyReadiness      *bool                  `yaml:"earlyReadiness,omitempty"`
	Spec                map[string]interface{} `yaml:"spec,omitempty"`
}

func (s *SecuredClusterConfig) EarlyReadinessEnabled() bool {
	return s.EarlyReadiness != nil && *s.EarlyReadiness
}

// NewSecuredClusterConfig returns an emptry SecuredClusterConfig, with deep initialization of data structures.
func NewSecuredClusterConfig() SecuredClusterConfig {
	return SecuredClusterConfig{
		Spec: make(map[string]interface{}),
	}
}

// DefaultSecuredClusterConfig returns a SecuredClusterConfig with sensible defaults.
func DefaultSecuredClusterConfig() SecuredClusterConfig {
	cfg := NewSecuredClusterConfig()
	cfg.DeployTimeout = DefaultSecuredClusterWaitTimeout
	cfg.Namespace = "acs-sensor"
	cfg.EarlyReadiness = new(true)
	return cfg
}

func (s *SecuredClusterConfig) PauseReconciliationSet() bool {
	return s.PauseReconciliation != nil
}

func (s *SecuredClusterConfig) PauseReconciliationEnabled() bool {
	return s.PauseReconciliation != nil && *s.PauseReconciliation
}

func (s *SecuredClusterConfig) EarlyReadinessSet() bool {
	return s.EarlyReadiness != nil
}

func (s *SecuredClusterConfig) GetWaitConfig() WaitConfig {
	waitFor := "securedcluster/" + securedClusterCrName
	if s.EarlyReadinessEnabled() {
		waitFor = "deployment/sensor"
	}
	return WaitConfig{
		Namespace:      s.Namespace,
		EarlyReadiness: s.EarlyReadinessEnabled(),
		WaitFor:        waitFor,
		Timeout:        s.DeployTimeout,
	}
}

// ConfigureSpec applies feature flags and the central endpoint to the SecuredCluster spec.
func (s *SecuredClusterConfig) ConfigureSpec(roxieConfig *RoxieConfig, centralConfig *CentralConfig) error {
	if err := helpers.DeepMerge(s.Spec, featureFlagsToOverrides(roxieConfig.FeatureFlags)); err != nil {
		return err
	}

	if _, exists := s.Spec["centralEndpoint"]; !exists {
		s.Spec["centralEndpoint"] = internalCentralEndpoint(centralConfig.Namespace)
	}

	return nil
}

// CustomResource builds an unstructured SecuredCluster custom resource from this config.
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

package clusterdefaults

import (
	"fmt"

	"dario.cat/mergo"
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/types"
	"k8s.io/utils/ptr"
)

// ApplyClusterDefaults detects the cluster type and applies appropriate defaults to the
// provided deployer.Config.
// Returns *just* the assembled defaults for the given cluster type for logging purposes.
func ApplyClusterDefaults(
	config *deployer.Config,
) (*deployer.Config, error) {
	if config == nil {
		panic("applying cluster defaults to nil config")
	}
	clusterType := config.Roxie.ClusterType
	defaults := getDefaultsForClusterType(clusterType)
	if defaults == nil {
		return nil, nil
	}

	// Make a copy.
	defaultsCopy, err := defaults.DeepCopy()
	if err != nil {
		return nil, fmt.Errorf("deep-copying cluster defaults for cluster type %s: %w", clusterType, err)
	}

	if err := mergo.Merge(config, defaultsCopy, mergo.WithoutDereference); err != nil {
		return nil, fmt.Errorf("merging-in cluster defaults for cluster type %s: %w", clusterType, err)
	}

	return defaultsCopy, nil
}

// getDefaultsForClusterType returns a deployer.Config filled with the defaults for the given
// cluster type.
// Note that to be able to differentiate "not set" from "set specifically to the empty value",
// any fields set specifically to the empty value (e.g. false booleans), must be of pointer type
// in the Config struct. Otherwise, `ApplyClusterDefaults` would not apply those to the caller-provided
// configuration.
func getDefaultsForClusterType(clusterType types.ClusterType) *deployer.Config {
	switch {
	case clusterType.IsLocal():
		return &deployer.Config{
			Central: deployer.CentralConfig{
				Exposure:       ptr.To(types.ExposureNone),
				PortForwarding: ptr.To(true),
			},
		}

	case clusterType.IsGKE() || clusterType.IsOpenShift():
		return &deployer.Config{
			Central: deployer.CentralConfig{
				Exposure:       ptr.To(types.ExposureLoadBalancer),
				PortForwarding: ptr.To(false),
			},
		}

	default:
		return nil
	}
}

// ResolveAutoResourceProfile resolves the "auto" resource profile depending on the cluster type.
func ResolveAutoResourceProfile(clusterType types.ClusterType) types.ResourceProfile {
	switch {
	case clusterType.IsLocal():
		return types.ResourceProfileSmall

	case clusterType.IsGKE() || clusterType.IsOpenShift():
		return types.ResourceProfileMedium

	default:
		return types.ResourceProfileAcsDefaults
	}
}

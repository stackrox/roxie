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
	clusterType types.ClusterType,
	config *deployer.Config,
) (*deployer.Config, error) {
	if config == nil {
		panic("applying cluster defaults to nil config")
	}
	defaults := getDefaultsForClusterType(clusterType)
	if defaults == nil {
		return nil, nil
	}

	// Make a copy.
	defaultsCopy, err := defaults.DeepCopy()
	if err != nil {
		return nil, fmt.Errorf("deep-copying cluster defaults: %w", err)
	}

	if err := mergo.Merge(config, defaultsCopy, mergo.WithoutDereference); err != nil {
		return nil, fmt.Errorf("merging-in cluster defaults: %w", err)
	}

	return defaultsCopy, nil
}

func getDefaultsForClusterType(clusterType types.ClusterType) *deployer.Config {
	switch clusterType {
	case types.ClusterTypeKind, types.ClusterTypeMinikube, types.ClusterTypeK3s, types.ClusterTypeCRC:
		return &deployer.Config{
			Central: deployer.CentralConfig{
				Exposure:       ptr.To(types.ExposureNone),
				PortForwarding: ptr.To(true),
			},
		}

	case types.ClusterTypeInfraGKE, types.ClusterTypeInfraOpenShift4:
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
	switch clusterType {
	case types.ClusterTypeKind:
		return types.ResourceProfileSmall

	case types.ClusterTypeMinikube:
		return types.ResourceProfileSmall

	case types.ClusterTypeK3s:
		return types.ResourceProfileSmall

	case types.ClusterTypeCRC:
		return types.ResourceProfileSmall

	case types.ClusterTypeInfraOpenShift4:
		return types.ResourceProfileMedium

	case types.ClusterTypeInfraGKE:
		return types.ResourceProfileMedium

	default:
		return types.ResourceProfileAcsDefaults
	}
}

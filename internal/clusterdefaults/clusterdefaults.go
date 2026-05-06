package clusterdefaults

import (
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/types"
	"k8s.io/apimachinery/pkg/runtime"
)

// ApplyClusterDefaults detects the cluster type and applies appropriate defaults to the
// provided deployer.Config.
// Returns *just* the assembled defaults for the given cluster type for logging purposes.
func ApplyClusterDefaults(
	clusterType types.ClusterType,
	config *deployer.Config,
) (map[string]interface{}, error) {
	if config == nil {
		panic("applying cluster defaults to nil config")
	}
	defaults := getDefaultsForClusterType(clusterType)
	if defaults == nil {
		return nil, nil
	}

	// Make a copy.
	defaultsCopy := runtime.DeepCopyJSON(defaults)

	configMap, err := helpers.StructToMap(config)
	if err != nil {
		return nil, err
	}
	mergeResult := make(map[string]interface{})
	if err = helpers.DeepMerge(mergeResult, defaults); err != nil {
		return nil, err
	}
	if err := helpers.DeepMerge(mergeResult, configMap); err != nil {
		return nil, err
	}

	if err := helpers.MapToStruct(mergeResult, config); err != nil {
		return nil, err
	}
	return defaultsCopy, nil
}

// getDefaultsForClusterType returns the recommended defaults for a given cluster type.
func getDefaultsForClusterType(clusterType types.ClusterType) map[string]interface{} {
	switch clusterType {
	case types.ClusterTypeKind:
		// Kind clusters are local, lightweight, and don't support LoadBalancer.
		return map[string]interface{}{
			"central": map[string]interface{}{
				"exposure":       types.ExposureNone.String(),
				"portForwarding": true,
			},
		}

	case types.ClusterTypeMinikube:
		return map[string]interface{}{
			"central": map[string]interface{}{
				"exposure":       types.ExposureNone.String(),
				"portForwarding": true,
			},
		}

	case types.ClusterTypeK3s:
		return map[string]interface{}{
			"central": map[string]interface{}{
				"exposure":       types.ExposureNone.String(),
				"portForwarding": true,
			},
		}

	case types.ClusterTypeCRC:
		return map[string]interface{}{
			"central": map[string]interface{}{
				"exposure":       types.ExposureNone.String(),
				"portForwarding": true,
			},
		}

	case types.ClusterTypeInfraGKE:
		return map[string]interface{}{
			"central": map[string]interface{}{
				"exposure":       types.ExposureLoadBalancer.String(),
				"portForwarding": false,
			},
		}

	case types.ClusterTypeInfraOpenShift4:
		return map[string]interface{}{
			"central": map[string]interface{}{
				"exposure":       types.ExposureLoadBalancer.String(),
				"portForwarding": false,
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

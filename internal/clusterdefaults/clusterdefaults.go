package clusterdefaults

import (
	"github.com/stackrox/roxie/internal/deployer"
	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/logger"
	"github.com/stackrox/roxie/internal/types"
)

// ApplyClusterDefaults detects the cluster type and applies appropriate defaults.
func ApplyClusterDefaults(
	log *logger.Logger,
	clusterType types.ClusterType,
	config *deployer.Config,
) error {
	if config == nil {
		panic("applying cluster defaults to nil config")
	}
	configWithDefaults := getDefaultsForClusterType(clusterType)
	if configWithDefaults == nil {
		return nil
	}
	log.Dimf("Applying the following defaults based on detected cluster type %v:", clusterType)
	helpers.LogMultilineYaml(log, configWithDefaults)
	configMap, err := helpers.StructToMap(config)
	if err != nil {
		return err
	}
	err = helpers.DeepMerge(configWithDefaults, configMap)
	if err != nil {
		return err
	}

	if err := helpers.MapToStruct(configWithDefaults, config); err != nil {
		return err
	}
	return nil
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

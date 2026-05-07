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

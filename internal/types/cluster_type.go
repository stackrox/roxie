package types

import "fmt"

// ClusterType represents different types of Kubernetes clusters
type ClusterType string

const (
	// ClusterTypeUnknown represents an unidentified cluster type
	ClusterTypeUnknown ClusterType = "Unknown"
	// ClusterTypeGKE represents a generic GKE cluster.
	ClusterTypeGKE ClusterType = "GKE"
	// ClusterTypeInfraGKE represents a GKE cluster created via Infra.
	ClusterTypeInfraGKE ClusterType = "InfraGKE"
	// ClusterTypeInfraOpenShift4 represents an OpenShift 4 cluster
	ClusterTypeInfraOpenShift4 ClusterType = "InfraOpenShift4"
	// Generic OpenShift4 cluster (e.g. for prow CI)
	ClusterTypeOpenShift4 ClusterType = "OpenShift4"
	// ClusterTypeKind represents a Kind (Kubernetes in Docker) cluster
	ClusterTypeKind ClusterType = "Kind"
	// ClusterTypeMinikube represents a Minikube cluster
	ClusterTypeMinikube ClusterType = "Minikube"
	// ClusterTypeK3s represents a K3s cluster
	ClusterTypeK3s ClusterType = "K3s"
	// ClusterTypeCRC represents a CRC (CodeReady Containers) cluster
	ClusterTypeCRC ClusterType = "CRC"
	// ClusterTypeAKS represents a generic AKS (Azure Kubernetes Service) cluster.
	ClusterTypeAKS ClusterType = "AKS"
	// ClusterTypeInfraAKS represents an AKS cluster created via Infra.
	ClusterTypeInfraAKS ClusterType = "InfraAKS"
)

func (ct ClusterType) IsGKE() bool {
	return ct == ClusterTypeInfraGKE || ct == ClusterTypeGKE
}

func (ct ClusterType) IsOpenShift() bool {
	return ct == ClusterTypeInfraOpenShift4 || ct == ClusterTypeOpenShift4
}

func (ct ClusterType) IsAKS() bool {
	return ct == ClusterTypeInfraAKS || ct == ClusterTypeAKS
}

// String returns the string representation of a ClusterType
func (ct ClusterType) String() string {
	switch ct {
	case ClusterTypeInfraGKE:
		return "GKE (infra)"
	case ClusterTypeInfraOpenShift4:
		return "OpenShift4 (infra)"
	case ClusterTypeInfraAKS:
		return "AKS (infra)"
	default:
		return string(ct)
	}
}

func AllClusterTypes() []ClusterType {
	return []ClusterType{
		ClusterTypeUnknown,
		ClusterTypeGKE,
		ClusterTypeInfraGKE,
		ClusterTypeKind,
		ClusterTypeMinikube,
		ClusterTypeK3s,
		ClusterTypeCRC,
		ClusterTypeInfraOpenShift4,
		ClusterTypeOpenShift4,
		ClusterTypeAKS,
		ClusterTypeInfraAKS,
	}
}

func (ct *ClusterType) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}

	var sAsClusterType = ClusterType(s)

	for _, valid := range AllClusterTypes() {
		if sAsClusterType == valid {
			*ct = valid
			return nil
		}
	}
	return fmt.Errorf("unknown cluster type identifier: %q", s)
}

// NeedsDefaultRegistryPullSecrets reports whether this cluster type lacks
// auto-configured credentials for the default image registry (quay.io/rhacs-eng).
func (ct ClusterType) NeedsDefaultRegistryPullSecrets() bool {
	return ct != ClusterTypeInfraOpenShift4
}

func (ct ClusterType) IsLocal() bool {
	switch ct {
	case ClusterTypeKind, ClusterTypeMinikube, ClusterTypeK3s, ClusterTypeCRC:
		return true
	}
	return false
}

func (ct ClusterType) SupportsLoadBalancer() bool {
	return ct.IsGKE() || ct.IsOpenShift() || ct.IsAKS()
}

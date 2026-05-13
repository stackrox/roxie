package types

// ClusterType represents different types of Kubernetes clusters
type ClusterType int

const (
	// ClusterTypeUnknown represents an unidentified cluster type
	ClusterTypeUnknown ClusterType = iota
	// ClusterTypeInfraGKE represents a GKE cluster created via Infra.
	ClusterTypeInfraGKE
	// ClusterTypeInfraOpenShift4 represents an OpenShift 4 cluster
	ClusterTypeInfraOpenShift4
	// Generic OpenShift4 cluster (e.g. for prow CI)
	ClusterTypeOpenShift4
	// ClusterTypeKind represents a Kind (Kubernetes in Docker) cluster
	ClusterTypeKind
	// ClusterTypeMinikube represents a Minikube cluster
	ClusterTypeMinikube
	// ClusterTypeK3s represents a K3s cluster
	ClusterTypeK3s
	// ClusterTypeCRC represents a CRC (CodeReady Containers) cluster
	ClusterTypeCRC
)

func (ct ClusterType) IsOpenShift() bool {
	return ct == ClusterTypeInfraOpenShift4 || ct == ClusterTypeOpenShift4
}

// String returns the string representation of a ClusterType
func (ct ClusterType) String() string {
	switch ct {
	case ClusterTypeInfraGKE:
		return "GKE"
	case ClusterTypeInfraOpenShift4:
		return "OpenShift4 (infra)"
	case ClusterTypeOpenShift4:
		return "OpenShift4"
	case ClusterTypeKind:
		return "Kind"
	case ClusterTypeMinikube:
		return "minikube"
	case ClusterTypeK3s:
		return "k3s"
	case ClusterTypeCRC:
		return "crc"
	default:
		return "Unknown"
	}
}

func AllClusterTypes() []ClusterType {
	return []ClusterType{
		ClusterTypeInfraGKE,
		ClusterTypeKind,
		ClusterTypeMinikube,
		ClusterTypeK3s,
		ClusterTypeCRC,
		ClusterTypeInfraOpenShift4,
		ClusterTypeOpenShift4,
	}
}

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
	// ClusterTypeKind represents a Kind (Kubernetes in Docker) cluster
	ClusterTypeKind
	// ClusterTypeMinikube represents a Minikube cluster
	ClusterTypeMinikube
	// ClusterTypeK3s represents a K3s cluster
	ClusterTypeK3s
	// ClusterTypeCRC represents a CRC (CodeReady Containers) cluster
	ClusterTypeCRC
)

// String returns the string representation of a ClusterType
func (ct ClusterType) String() string {
	switch ct {
	case ClusterTypeInfraGKE:
		return "GKE"
	case ClusterTypeInfraOpenShift4:
		return "OpenShift4"
	case ClusterTypeKind:
		return "kind"
	case ClusterTypeMinikube:
		return "minikube"
	case ClusterTypeK3s:
		return "k3s"
	case ClusterTypeCRC:
		return "crc"
	default:
		return "unknown"
	}
}

func AllClusterTypes() []ClusterType {
	return []ClusterType{
		ClusterTypeInfraGKE,
		ClusterTypeKind,
		ClusterTypeMinikube,
		ClusterTypeK3s,
		ClusterTypeCRC,
	}
}

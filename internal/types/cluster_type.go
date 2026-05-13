package types

import "fmt"

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

var (
	clusterTypeToIdentifier = map[ClusterType]string{
		ClusterTypeUnknown:         "Unknown",
		ClusterTypeInfraGKE:        "InfraGKE",
		ClusterTypeInfraOpenShift4: "InfraOpenShift4",
		ClusterTypeOpenShift4:      "OpenShift4",
		ClusterTypeKind:            "Kind",
		ClusterTypeMinikube:        "Minikube",
		ClusterTypeK3s:             "K3s",
		ClusterTypeCRC:             "CRC",
	}

	identifierToClusterType = func() map[string]ClusterType {
		m := make(map[string]ClusterType, len(clusterTypeToIdentifier))
		for k, v := range clusterTypeToIdentifier {
			m[v] = k
		}
		return m
	}()
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

func (ct ClusterType) MarshalYAML() (any, error) {
	if id, ok := clusterTypeToIdentifier[ct]; ok {
		return id, nil
	}
	return nil, fmt.Errorf("unknown cluster type: %d", ct)
}

func (ct *ClusterType) UnmarshalYAML(unmarshal func(any) error) error {
	var s string
	if err := unmarshal(&s); err != nil {
		return err
	}
	parsed, ok := identifierToClusterType[s]
	if !ok {
		return fmt.Errorf("unknown cluster type identifier: %q", s)
	}
	*ct = parsed
	return nil
}

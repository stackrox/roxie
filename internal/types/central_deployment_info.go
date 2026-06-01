package types

// CentralDeploymentInfo holds the state of a Central deployment.
type CentralDeploymentInfo struct {
	Endpoint       string
	Password       string
	KubeContext    string
	Exposure       Exposure
	CACertFile     string
	HAProxyStarted bool
}

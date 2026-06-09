package roxieenv

import (
	"github.com/stackrox/roxie/internal/types"
)

// AssembleRoxieEnvironment returns a roxie environment for interacting with an ACS Central deployment.
// This is used for
// * writing envrc files
// * spawning sub-shells as part of the deployer
// * spawning sub-shells/executing commands for the 'shell' command
func AssembleRoxieEnvironment(info types.CentralDeploymentInfo) types.RoxieEnvironment {
	var env types.RoxieEnvironment

	if info.Endpoint != "" {
		env.APIEndpoint = info.Endpoint
		env.RoxEndpoint = info.Endpoint
		env.RoxBaseURL = "https://" + info.Endpoint
	}
	if info.Username != "" {
		env.RoxUsername = info.Username
	}
	if info.Password != "" {
		env.RoxAdminPassword = info.Password
	}
	if info.CACertFile != "" {
		env.RoxCaCertFile = info.CACertFile
	}

	return env
}

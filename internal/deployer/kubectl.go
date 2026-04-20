package deployer

import (
	"context"

	"github.com/stackrox/roxie/internal/k8s"
)

// runKubectl is a thin wrapper around k8s.RunKubectl that injects the deployer's logger.
//
// TODO(#91): perhaps this should not return a pointer so that the lack of nil checks elsewhere
// is robust?
func (d *Deployer) runKubectl(ctx context.Context, opts k8s.KubectlOptions) (*k8s.KubectlResult, error) {
	return k8s.RunKubectl(ctx, d.logger, opts)
}

package deployer

import (
	"context"

	"github.com/stackrox/roxie/internal/k8s"
)

// runKubectl is a thin wrapper around k8s.RunKubectl.
func (d *Deployer) runKubectl(ctx context.Context, opts k8s.KubectlOptions) (k8s.KubectlResult, error) {
	return k8s.RunKubectl(ctx, opts)
}

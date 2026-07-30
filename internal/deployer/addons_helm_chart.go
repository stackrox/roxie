package deployer

import (
	"context"
	"fmt"

	"github.com/stackrox/roxie/internal/helm"
	"github.com/stackrox/roxie/internal/logger"
)

const (
	addOnReleasePrefix    = "roxie-addon-"
	maxHelmReleaseNameLen = 53
)

type helmAddOn struct {
	log         *logger.Logger
	verbose     bool
	name        string
	releaseName string
	installOpts helm.InstallOptions
	optional    bool
	priority    uint
}

func (a *helmAddOn) Name() string {
	return a.name
}

func (a *helmAddOn) Priority() uint {
	return a.priority
}

func (a *helmAddOn) IsOptional() bool {
	return a.optional
}

func (a *helmAddOn) Deploy(ctx context.Context) error {
	a.log.Infof("Installing add-on %q as Helm release %q", a.name, a.releaseName)
	helmCtx := helm.HelmCtx{
		Ctx:     ctx,
		Log:     a.log,
		Verbose: a.verbose,
	}
	return helm.Install(helmCtx, a.installOpts)
}

func (a *helmAddOn) Teardown(ctx context.Context) error {
	a.log.Infof("Uninstalling add-on Helm release %q", a.releaseName)
	helmCtx := helm.HelmCtx{
		Ctx:     ctx,
		Log:     a.log,
		Verbose: a.verbose,
	}
	return helm.Uninstall(helmCtx, a.releaseName, a.installOpts.Namespace)
}

func newHelmAddOn(
	addOnCfg AddOnConfig,
	commonProperties CommonAddOnProperties,
	name, namespace string,
	opts helm.InstallOptions,
) (AddOn, error) {
	releaseName := addOnReleasePrefix + name
	if len(releaseName) > maxHelmReleaseNameLen {
		return nil, fmt.Errorf("add-on %q: release name %q exceeds %d characters", name, releaseName, maxHelmReleaseNameLen)
	}
	opts.ReleaseName = releaseName
	opts.Namespace = namespace
	return &helmAddOn{
		log:         addOnCfg.log,
		verbose:     addOnCfg.verbose,
		name:        name,
		releaseName: releaseName,
		installOpts: opts,
		optional:    commonProperties.Optional,
		priority:    commonProperties.Priority,
	}, nil
}

// New creates a helmAddOn that installs a chart from a public Helm repository.
func (h *HelmChartRepoAddOn) New(
	addOnCfg AddOnConfig,
	commonProperties CommonAddOnProperties,
	name, namespace string) (AddOn, error) {
	addOnCfg.log.Infof("Add-on %s: using Helm chart %s from %s", name, h.Chart, h.Repo)

	values, err := h.GetValues()
	if err != nil {
		return nil, fmt.Errorf("retrieving values for Helm chart add-on %q: %w", name, err)
	}
	opts := helm.InstallOptions{
		RepoURL:      h.Repo,
		ChartName:    h.Chart,
		ChartVersion: h.Version,
		Values:       values,
	}

	return newHelmAddOn(addOnCfg, commonProperties, name, namespace, opts)
}

package helm

import (
	"context"
	"fmt"
	"io"
	"regexp"
	"strings"
	"time"

	"github.com/stackrox/roxie/internal/helpers"
	"github.com/stackrox/roxie/internal/logger"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart/loader"
	"helm.sh/helm/v3/pkg/cli"
	"helm.sh/helm/v3/pkg/downloader"
	"helm.sh/helm/v3/pkg/getter"
	"helm.sh/helm/v3/pkg/release"
)

// InstallOptions contains options for installing or upgrading a Helm chart.
type InstallOptions struct {
	ReleaseName  string
	ChartPath    string
	RepoURL      string
	ChartName    string
	ChartVersion string
	Namespace    string
	Values       map[string]any
}

const (
	retryDelay  = 2 * time.Second
	maxAttempts = 3
	helmDriver  = "secrets"
)

var retryableErrors = []string{
	"connection refused",
	"connection reset",
	"timed out",
	"timeout",
	"temporary failure",
	"network is unreachable",
	"no route to host",
	"tls handshake timeout",
	"eof",
	"broken pipe",
	"context deadline exceeded",
}

type HelmCtx struct {
	Ctx     context.Context
	Log     *logger.Logger
	Verbose bool
	DryRun  bool
}

// Install installs or upgrades a Helm chart idempotently.
func Install(helmCtx HelmCtx, opts InstallOptions) error {
	if opts.ChartPath == "" && (opts.RepoURL == "" || opts.ChartName == "") {
		return fmt.Errorf("either ChartPath or RepoURL+ChartName must be set")
	}

	return executeHelmActionWithRetries(helmCtx, "install",
		func(helmCtx HelmCtx) error {
			return doInstall(helmCtx, opts)
		})
}

// Uninstall removes a Helm release, ignoring "not found" errors.
func Uninstall(helmCtx HelmCtx, releaseName, namespace string) error {
	return executeHelmActionWithRetries(helmCtx, "uninstall",
		func(helmCtx HelmCtx) error {
			err := doUninstall(helmCtx, releaseName, namespace)
			if err == nil || strings.Contains(strings.ToLower(err.Error()), "not found") {
				return nil
			}
			return err
		})
}

// ListByPrefix returns the names of Helm releases whose name starts with the given prefix.
func ListByPrefix(helmCtx HelmCtx, prefix, namespace string) ([]string, error) {
	var releases []string

	err := executeHelmActionWithRetries(helmCtx, "list",
		func(helmCtx HelmCtx) error {
			result, err := doListByPrefix(helmCtx, prefix, namespace)
			if err != nil {
				return err
			}
			releases = result
			return nil
		})
	if err != nil {
		return nil, err
	}

	return releases, nil
}

func executeHelmActionWithRetries(helmCtx HelmCtx, actionName string, helmAction func(helmCtx HelmCtx) error) error {
	var lastErr error
	for attempt := 1; attempt <= maxAttempts; attempt++ {
		if attempt > 1 {
			waitTime := time.Duration(attempt) * retryDelay
			helmCtx.Log.Infof("Retrying helm %s (attempt %d/%d) after %v...", actionName, attempt, maxAttempts, waitTime)
			select {
			case <-helmCtx.Ctx.Done():
				return fmt.Errorf("helm %s aborted while waiting to retry: %w", actionName, helmCtx.Ctx.Err())
			case <-time.After(waitTime):
			}
		}

		err := helmAction(helmCtx)
		if err == nil {
			return nil
		}
		lastErr = err

		if !isRetryable(err) || attempt == maxAttempts {
			return fmt.Errorf("helm %s failed: %w", actionName, err)
		}

		helmCtx.Log.Warningf("Transient error during helm %s: %v", actionName, err)
	}
	return fmt.Errorf("helm %s failed after %d attempts: %w", actionName, maxAttempts, lastErr)
}

func doInstall(helmCtx HelmCtx, opts InstallOptions) error {
	cfg, err := newActionConfig(helmCtx, opts.Namespace)
	if err != nil {
		return err
	}

	chartPath, err := resolveChart(opts)
	if err != nil {
		return err
	}
	if helmCtx.Verbose {
		helmCtx.Log.Dimf("resolved Helm chart for release %s as %q", opts.ReleaseName, chartPath)
		helmCtx.Log.Dimf("values for Helm chart for release %s:", opts.ReleaseName)
		helpers.LogMultilineYaml(helmCtx.Log, opts.Values)
	}

	if helmCtx.DryRun {
		helmCtx.Log.Infof("skipping action for add-on Helm chart %s", opts.ReleaseName)
		return nil
	}

	if helmCtx.Verbose {
		if opts.ChartPath != "" {
			helmCtx.Log.Dimf("installing Helm chart from directory %q as release %s into namespace %s",
				opts.ChartPath, opts.ReleaseName, opts.Namespace)
		} else {
			helmCtx.Log.Dimf("installing Helm chart %s/%s:%s as release %s into namespace %s",
				opts.RepoURL, opts.ChartName, opts.ChartVersion, opts.ReleaseName, opts.Namespace)
		}
	}

	status := releaseStatus(cfg, opts.ReleaseName)
	if status.IsPending() || status == release.StatusFailed {
		helmCtx.Log.Warningf("Helm release %s is in state %q, forcing uninstall before reinstall", opts.ReleaseName, status)
		uninstall := action.NewUninstall(cfg)
		if _, err := uninstall.Run(opts.ReleaseName); err != nil && !strings.Contains(strings.ToLower(err.Error()), "not found") {
			return fmt.Errorf("cleaning up stuck release %s: %w", opts.ReleaseName, err)
		}
	} else if status == release.StatusDeployed {
		return doUpgrade(helmCtx, cfg, opts, chartPath)
	}

	install := action.NewInstall(cfg)
	install.ReleaseName = opts.ReleaseName
	install.Namespace = opts.Namespace
	install.Wait = false

	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("loading chart from %q: %w", chartPath, err)
	}

	_, err = install.RunWithContext(helmCtx.Ctx, chart, opts.Values)
	return err
}

func doUpgrade(helmCtx HelmCtx, cfg *action.Configuration, opts InstallOptions, chartPath string) error {
	if helmCtx.DryRun {
		helmCtx.Log.Infof("skipping upgrade action for add-on Helm chart %s", opts.ReleaseName)
		return nil
	}

	if helmCtx.Verbose {
		helmCtx.Log.Dimf("a Helm release named %s already exists in namespace %s, conducting upgrade",
			opts.ReleaseName, opts.Namespace)
	}
	upgrade := action.NewUpgrade(cfg)
	upgrade.Namespace = opts.Namespace
	upgrade.Wait = false

	chart, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("loading chart from %q: %w", chartPath, err)
	}

	if helmCtx.Verbose {
		helmCtx.Log.Dimf("values for Helm chart for release %s:", opts.ReleaseName)
		helpers.LogMultilineYaml(helmCtx.Log, opts.Values)
	}

	_, err = upgrade.RunWithContext(helmCtx.Ctx, opts.ReleaseName, chart, opts.Values)
	return err
}

func doUninstall(helmCtx HelmCtx, releaseName, namespace string) error {
	if helmCtx.DryRun {
		helmCtx.Log.Infof("skipping uninstall action for add-on Helm chart %s", releaseName)
		return nil
	}

	cfg, err := newActionConfig(helmCtx, namespace)
	if err != nil {
		return err
	}

	if helmCtx.Verbose {
		helmCtx.Log.Dimf("uninstalling Helm release %s from namespace %s", releaseName, namespace)
	}

	uninstall := action.NewUninstall(cfg)
	_, err = uninstall.Run(releaseName)
	return err
}

func doListByPrefix(helmCtx HelmCtx, prefix, namespace string) ([]string, error) {
	cfg, err := newActionConfig(helmCtx, namespace)
	if err != nil {
		return nil, err
	}

	list := action.NewList(cfg)
	list.Filter = "^" + regexp.QuoteMeta(prefix)

	results, err := list.Run()
	if err != nil {
		return nil, err
	}

	var releases []string
	for _, r := range results {
		releases = append(releases, r.Name)
	}
	return releases, nil
}

func isRetryable(err error) bool {
	if err == nil {
		return false
	}
	msg := strings.ToLower(err.Error())
	for _, pattern := range retryableErrors {
		if strings.Contains(msg, pattern) {
			return true
		}
	}
	return false
}

func newActionConfig(helmCtx HelmCtx, namespace string) (*action.Configuration, error) {
	settings := cli.New()
	settings.SetNamespace(namespace)

	cfg := new(action.Configuration)
	logFunc := func(format string, v ...any) {
		if helmCtx.Log != nil && helmCtx.Verbose {
			helmCtx.Log.Dimf("[helm] "+format, v...)
		}
	}
	if err := cfg.Init(settings.RESTClientGetter(), namespace, helmDriver, logFunc); err != nil {
		return nil, fmt.Errorf("initializing helm configuration: %w", err)
	}
	return cfg, nil
}

func resolveChart(opts InstallOptions) (string, error) {
	if opts.ChartPath != "" {
		return opts.ChartPath, nil
	}
	cpo := action.ChartPathOptions{
		RepoURL: opts.RepoURL,
		Version: opts.ChartVersion,
	}
	resolved, err := cpo.LocateChart(opts.ChartName, cli.New())
	if err != nil {
		return "", fmt.Errorf("locating chart %q from repo %q: %w", opts.ChartName, opts.RepoURL, err)
	}
	return resolved, nil
}

// BuildDependencies fetches missing sub-chart dependencies for a local chart directory.
// It is a no-op if the chart has no dependencies or all are already present.
func BuildDependencies(log *logger.Logger, chartPath string) error {
	ch, err := loader.Load(chartPath)
	if err != nil {
		return fmt.Errorf("loading chart from %q: %w", chartPath, err)
	}

	if err := action.CheckDependencies(ch, ch.Metadata.Dependencies); err == nil {
		return nil
	}

	log.Infof("Building Helm chart dependencies for %s...", chartPath)

	settings := cli.New()
	man := &downloader.Manager{
		Out:              io.Discard,
		ChartPath:        chartPath,
		Getters:          getter.All(settings),
		RepositoryConfig: settings.RepositoryConfig,
		RepositoryCache:  settings.RepositoryCache,
	}
	if err := man.Build(); err != nil {
		return fmt.Errorf("building chart dependencies for %q: %w", chartPath, err)
	}
	return nil
}

func releaseStatus(cfg *action.Configuration, name string) release.Status {
	hist := action.NewHistory(cfg)
	hist.Max = 1
	releases, err := hist.Run(name)
	if err != nil || len(releases) == 0 {
		return release.StatusUnknown
	}
	return releases[0].Info.Status
}

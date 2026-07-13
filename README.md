**PLEASE NOTE: This repository contains a deployment tool for ACS, which is used by
ACS engineers. It is **not** a general-purpose installation frontend for ACS or StackRox users.**

# roxie – ACS deployments made easy

[![Code Quality](https://github.com/stackrox/roxie/actions/workflows/code-quality.yml/badge.svg)](https://github.com/stackrox/roxie/actions/workflows/code-quality.yml)
[![Tests](https://github.com/stackrox/roxie/actions/workflows/test.yml/badge.svg)](https://github.com/stackrox/roxie/actions/workflows/test.yml)

roxie is a fast, developer-friendly CLI to deploy and manage Red Hat Advanced Cluster Security (ACS) on any Kubernetes/OpenShift cluster.

roxie has been authored with significant AI contributions.

## Highlights

- Quick & easy ACS deployment: one command to get Central and a Secured Cluster up and running.
- Automated waiting for readiness and loadbalancer availability.
- No fiddling with API endpoints: detects and wires endpoints automatically.
- No fiddling with init bundles or CRS: roxie generates and handles these bits for you.
- Deploys the ACS Operator without requiring OpenShift/OLM.
- Ability to replace operator versions (up- and downgrading).
- Automated fast ACS teardowns.
- Handles Quay image pull secrets automatically.
- Verifies image existence before attempting deployment.

## Installation

Look up the latest release from https://github.com/stackrox/roxie/releases.

### Install from GitHub releases into local dev environment

For example, installing into `$HOME/bin`:
```bash
curl -fsSL -o "${HOME}/bin/roxie" \
    https://github.com/stackrox/roxie/releases/download/v0.4.0/roxie-linux-amd64
chmod +x "${HOME}/bin/roxie"
```

On macOS you likely also need
```bash
xattr -d com.apple.quarantine "${HOME}/bin/roxie"
```

### Installing from source into local dev environment

Built using:
```bash
git clone git@github.com:stackrox/roxie.git
cd roxie
make install
```

This will install `roxie` into `${GOPATH}/bin`. If that is not desired you can also
build and copy manually:
```bash
make build
cp roxie /your/custom/bin
```

### Install from GitHub releases as part of CI workflow

```bash
curl -fsSL --retry 5 --retry-all-errors -o /usr/local/bin/roxie \
    https://github.com/stackrox/roxie/releases/download/v0.4.0/roxie-linux-amd64
chmod +x /usr/local/bin/roxie
```

### Install in container image

roxie can also be installed by extracting from a published roxie container image, for example
during container building:

```dockerfile
ARG ROXIE_VERSION=0.4.0
ARG ROXIE_CHECKSUM=sha256:5fe1d6d4d9c0e33385d8ca9de4baa14b4893cc5f27ddb6a3bddfe5021017fbf5
FROM quay.io/rhacs-eng/roxie:v${ROXIE_VERSION}@${ROXIE_CHECKSUM} AS roxie-installer

FROM <your-base-image>
COPY --from=roxie-installer /usr/local/bin/roxie /usr/bin/roxie
```

## Quick start

### Option 1: Deploying using image (Recommended for non-developers)

**Requirements:**
* Podman (or Docker) is set up
* kubeconfig configuration file

And, depending on the cluster:
* credentials for the `quay.io` registry in the environment variables `REGISTRY_USERNAME` and `REGISTRY_PASSWORD`.

Infra OpenShift4 clusters come already equipped with image pull secrets for `quay.io`, so in this case
passing of `REGISTRY_USERNAME` and `REGISTRY_PASSWORD` to the container is not required:

Example for deploying Central and SecuredCluster to an Infra OpenShift 4 cluster:
```bash
podman run --rm -it --privileged \
    -v $KUBECONFIG:/kubeconfig:U \
    quay.io/rhacs-eng/roxie:latest deploy -t 4.11.0 --resources=auto
```
Specify the main image tag (`--tag` or `-t`) as desired.

Deploying to a GKE cluster requires passing of some more arguments:
```
podman run --rm -it --privileged \
    -v ~/.config/gcloud:/.config/gcloud:U \
    -v $KUBECONFIG:/kubeconfig:U \
    -e REGISTRY_USERNAME=$REGISTRY_USERNAME \
    -e REGISTRY_PASSWORD=$REGISTRY_PASSWORD \
    quay.io/rhacs-eng/roxie:latest deploy -t 4.11.0 --resources=auto
```
Note that in this case we also need to pass the gcloud configuration for the authentication towards
the cluster to succeed.

### Option 2: Deploying using native executable

Prerequisites:
- `kubectl` configured to point at your target cluster
- `roxie` CLI is installed

Deploy using:
```bash
./roxie deploy -t 4.11.0 [ <component> ]
```
where `component` can be `central` or `sensor`. If not specified, both components will be deployed.
Specify the tag to deploy as desired.

Similarly, the deployment(s) can be torn down using:
```bash
./bin/roxie teardown [ <component> ]
```

### Multi-cluster deployments

roxie supports hub + spoke architectures where Central and SecuredCluster run on separate clusters.

1. Deploy Central on the hub cluster:
```bash
./roxie deploy central -t 4.11.0
```

2. Create a config file for the spoke cluster, pointing at the Central endpoint (printed during step 1):
```yaml
# spoke-config.yaml
securedCluster:
  spec:
    centralEndpoint: "<central-loadbalancer-ip>:443"
```

3. Switch kubectl context to the spoke cluster and deploy SecuredCluster:
```bash
ROX_ADMIN_PASSWORD=<admin-password> \
ROX_CA_CERT_FILE=<path-to-ca-cert> \
./roxie deploy secured-cluster -t 4.11.0 -c spoke-config.yaml
```

> **Tip:** If deploying from the roxie subshell, `ROX_ADMIN_PASSWORD` and `ROX_CA_CERT_FILE` are
> already set. For automation, consider using `--envrc <file>` on the Central deploy to write the
> environment to a file instead of spawning a subshell.

### Helm chart add-ons

roxie can deploy additional Helm charts alongside ACS. Add-ons are installed after Central
becomes ready and torn down before Central is removed.

Add-ons are configured entirely via the config file (`--config`). Each add-on needs a
definition in `availableAddOns`:

```yaml
central:
  addOns:
    monitoring: true
  availableAddOns:
    monitoring:
      helmChart:
        repo: https://prometheus-community.github.io/helm-charts
        chart: kube-state-metrics
        version: "5.27.0"
```

Charts from a local stackrox repo checkout are also supported:

```yaml
central:
  addOns:
    my-chart: true
  availableAddOns:
    my-chart:
      stackroxRepoHelmChart:
        path: deploy/charts/my-chart
        valuesFile: deploy/charts/my-chart/dev-values.yaml
        envSubst: true   # expand environment variables in the values file
```

Add-ons are installed as Helm releases prefixed with `roxie-addon-` in the Central namespace.
During teardown, roxie discovers which add-ons have been installed as part of the most
recent deployment and uninstalls those add-ons as part of a Central teardown.

## Development

Enter the dev shell:
```bash
nix develop
```

Common tasks:
```bash
make fmt          # Format code (ruff)
make lint         # Lint (ruff)
make test         # Unit tests
make test-e2e     # E2E tests (requires a real cluster context)
```

A new roxie image for the current platform can be built using:

```bash
make docker-build
```

This creates two tags:
- `localhost/roxie:latest`
- `localhost/roxie:<version-tag>`

Docker images can be built for the platforms `linux/amd64` and `linux/arm64`. See the `Makefile` for more
docker related targets.


## Testing (E2E)

The E2E suite expects a valid `kubectl` context.

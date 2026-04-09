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
- Operator by default: deploys the ACS Operator without requiring OpenShift/OLM. Helm is also supported.
- Ability to replace operator versions (up- and downgrading).
- Automated fast ACS teardowns.
- Handles Quay image pull secrets automatically.
- Verifies image existence before attempting deployment.

Note: Helm charts are currently also supported: use `--helm` to deploy Central and Secured Cluster via Helm. Only
use this in case you have very specific requirements which force you to use the Helm charts directly.
One example would be working on the Helm charts and needing to test them independently of the ACS operator.
Support for Helm charts might be dropped in the future.

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
    -e MAIN_IMAGE_TAG=4.9.2 \
    quay.io/rhacs-eng/roxie:latest deploy --resources=auto
```
Specify the `MAIN_IMAGE_TAG` as desired.

Deploying to a GKE cluster requires passing of some more arguments:
```
podman run --rm -it --privileged \
    -v ~/.config/gcloud:/.config/gcloud:U \
    -v $KUBECONFIG:/kubeconfig:U \
    -e MAIN_IMAGE_TAG=4.9.2 \
    -e REGISTRY_USERNAME=$REGISTRY_USERNAME \
    -e REGISTRY_PASSWORD=$REGISTRY_PASSWORD \
    quay.io/rhacs-eng/roxie:latest deploy --resources=auto
```
Note that in this case we also need to pass the gcloud configuration for the authentication towards
the cluster to succeed.

### Option 2: Deploying using local build

Prerequisites:
- `kubectl` configured to point at your target cluster
- The `roxctl` CLI
- The `roxie` branch forked and cloned to your local machine

Built using:
```bash
make build
```

Get help:
```bash
./roxie --help
```

Deploy using:
```bash
MAIN_IMAGE_TAG=4.9.2 ./roxie deploy [ <component> ]
```
where `component` can be `central` or `sensor`. If not specified, both components will be deployed.
Specify the `MAIN_IMAGE_TAG` as desired.

Similarly, the deployment(s) can be torn down using:
```bash
./bin/roxie teardown [ <component> ]
```

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
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
- Helm charts supported: use `--helm` to deploy Central and Secured Cluster via Helm.

## Quick start

### Option 1: Deploying using Docker image (Recommended for non-developers)

**Requirements:**
* Working Docker setup
* kubeconfig configuration file
* quay.io registry credentials in the environment variables REGISTRY_USERNAME and REGISTRY_PASSWORD.

Note that **Podman is currently not supported** for running
containerized roxie due to incomplete mapping of user IDs on macOS. This prevents the passing-in of the gcloud
configuration directory to be functional within the container, which is required for interacting with GKE clusters.

Example for deploying Central and SecuredCluster to the current Kubernetes cluster context:
```bash
docker run --rm -it --privileged \
    -v ~/.config/gcloud:/.config/gcloud \
    -v $KUBECONFIG:/kubeconfig \
    -e REGISTRY_USERNAME=$REGISTRY_USERNAME \
    -e REGISTRY_PASSWORD=$REGISTRY_PASSWORD \
    ghcr.io/stackrox/roxie:latest deploy
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

### Option 2: Deploying using local build

Prerequisites:
- `kubectl` configured to point at your target cluster
- `podman` is set up and available
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
./roxie deploy [ <component> ]
```
where `component` can be `central` or `sensor`. If not specified, both components will be deployed.

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

## Testing (E2E)

The E2E suite expects a valid `kubectl` context.
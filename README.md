# roxie – ACS deployments made easy

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

### Option 1: Container Image (Recommended for non-developers)

**Requirements:** Only Docker/Podman and a kubeconfig!

```bash
# Build the image (current platform)
make docker-build

# This creates two tags:
#   - roxie:latest
#   - roxie:0.1-<git-commit> (e.g., roxie:0.1-4469692)

# Build for multiple architectures (amd64 + arm64)
make docker-build-multiarch

# Deploy ACS
make docker-deploy COMPONENT=both

# Deploy with specific version tag (recommended for production)
make docker-deploy DOCKER_TAG=0.1-4469692 COMPONENT=both
```

**Supported architectures:** linux/amd64, linux/arm64

**Version Tags:** Every build automatically creates a version-tagged image (VERSION-COMMIT format) alongside `latest`. This enables:
- **Reproducible deployments** - Pin exact versions in CI/CD
- **Easy rollbacks** - Revert to any previous build
- **Version tracking** - Know exactly what code is running

### Option 2: Local Build (For development)

Prerequisites:
- `kubectl` configured to point at your target cluster
- The `roxctl` CLI installed
- The `roxie` branch forked and cloned to your local machine


Get help:
```bash
./bin/roxie --help
```

Deploy Central (via operator):
```bash
./bin/roxie deploy central
```

Deploy Secured Cluster (via operator):
```bash
# Ensure Central is reachable; roxie discovers and wires the endpoint
./bin/roxie deploy secured-cluster
```

Deploy both in one go:
```bash
./bin/roxie deploy both
```

Use Helm instead of Operator:
```bash
./bin/roxie deploy central --helm
./bin/roxie deploy secured-cluster --helm
# or
./bin/roxie deploy both --helm
```

Teardown:
```bash
./bin/roxie teardown central
./bin/roxie teardown secured-cluster
./bin/roxie teardown both
```

## Development

Enter the dev shell (pins Python, kubectl, helm, pytest, etc.):
```bash
./shell.sh
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
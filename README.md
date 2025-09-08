# roxie – ACS deployments made easy

Roxie is a fast, developer-friendly CLI to deploy and manage Red Hat Advanced Cluster Security (ACS) on any Kubernetes/OpenShift cluster.

## Highlights

- Quick & easy ACS deployment: one command to get Central and a Secured Cluster up and running.
- Automated waiting for readiness and loadbalancer availability.
- No fiddling with API endpoints: detects and wires endpoints automatically.
- No fiddling with init bundles or CRS: roxie generates and handles these bits for you.
- Operator by default: deploys the ACS Operator without requiring OpenShift/OLM. Helm is also supported.
- Ability to replace operator versions (up- and downgrading).
- Automated ACS teardowns.
- Helm charts supported: use `--helm` to deploy Central and Secured Cluster via Helm.
- Reproducible environment with Nix: portable, pinned dependencies; no host pollution.

## Quick start

Prerequisites:
- Nix with flakes enabled (recommended). New to Nix? See the quick start in the Determinate Systems installer: https://install.determinate.systems/nix
- `kubectl` configured to point at your target cluster

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

### Environment integration (direnv)

Roxie manages an envrc file at `~/.envrc.roxie` with handy variables (e.g., `API_ENDPOINT`, `ROX_ADMIN_PASSWORD`) from the current deployment. You can source it from any project’s `.envrc` to make these available in your shell:

```bash
if [[ -f ~/.envrc.roxie ]]; then
  source_env ~/.envrc.roxie
fi
```

This makes it convenient to access the admin password and API endpoint from the latest deploy.

## Development

Enter the dev shell (pins Python, kubectl, helm, pytest, etc.):
```bash
./shell.sh
```

Common tasks:
```bash
make fmt          # Format code (ruff)
make lint         # Lint (ruff)
make typecheck    # Type check (mypy)
make test         # Unit tests
make test-e2e     # E2E tests (requires a real cluster context)
```

## Testing (E2E)

The E2E suite expects a working `kubectl` context and may need environment variables for secured-cluster scenarios (e.g. loaded from `~/.envrc.roxie`).

Run a single scenario:
```bash
pytest -m e2e -s tests/test_e2e_basic.py::test_deploy_both_components_together
```

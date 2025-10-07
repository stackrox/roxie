import os
import shutil
import subprocess
import tempfile
import time

import pytest
from dotenv import dotenv_values

# Light import to reuse tag conversion logic for preflight
from deployer import ACSDeployer

pytestmark = pytest.mark.e2e

main_image_tag = "4.8.2"

common_deploy_args = ["--port-forwarding", "--exposure=none", "--resources=small"]


def get_repo_root() -> str:
    file_dir = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(file_dir, os.pardir))
    return repo_root


repo_root = get_repo_root()
roxie_path = os.path.join(repo_root, "bin", "roxie")


def _require_binary(name: str) -> None:
    if shutil.which(name) is None:
        msg = f"'{name}' not found in PATH; skipping e2e test"
        print(msg, flush=True)
        pytest.skip(msg)


def _require_kube_context() -> str:
    try:
        ctx = subprocess.check_output(["kubectl", "config", "current-context"], text=True).strip()
    except Exception as e:  # noqa: BLE001
        msg = f"No kubectl context available: {e}"
        print(msg, flush=True)
        pytest.skip(msg)
    if not ctx:
        pytest.skip("No current kubectl context; skipping e2e")
    return ctx


@pytest.fixture(scope="module", autouse=True)
def e2e_startup():
    _require_binary("kubectl")
    _require_binary("nix")
    _require_binary("skopeo")
    # Provide a default MAIN_IMAGE_TAG for all scenarios if not already set
    if not os.environ.get("MAIN_IMAGE_TAG"):
        os.environ["MAIN_IMAGE_TAG"] = main_image_tag
    ctx = _require_kube_context()
    print(f"Using kubectl context: {ctx}")
    if not os.path.exists(roxie_path):
        pytest.skip("bin/roxie not found; skipping e2e")


@pytest.fixture(scope="module")
def e2e_envrc_path():
    # Create a temporary, permission-protected envrc file path for tests
    tmp = tempfile.NamedTemporaryFile(prefix=".envrc.roxie-test-", delete=False)
    path = tmp.name
    tmp.close()
    os.chmod(path, 0o600)
    try:
        yield path
    finally:
        os.unlink(path)


def _run(cmd: list[str], env: dict[str, str] | None = None, timeout: int = 900) -> subprocess.CompletedProcess[str]:
    proc = subprocess.Popen(cmd, env=env)
    try:
        proc.wait(timeout=timeout)
    except subprocess.TimeoutExpired:
        proc.kill()
        raise
    if proc.returncode != 0:
        raise subprocess.CalledProcessError(proc.returncode, cmd)
    return subprocess.CompletedProcess(cmd, proc.returncode)


def _preflight_operator_bundle_pull(env: dict[str, str]) -> None:
    # Use skopeo to verify image availability (silent on success)
    # skopeo is guaranteed to be available in the Nix environment
    skopeo = "skopeo"

    # Compute operator tag using deployer logic
    d = ACSDeployer(cache_enabled=False)
    operator_tag = getattr(d, "operator_tag", None)

    if not operator_tag:
        # Fallback: expect override
        override = env.get("ROXIE_E2E_OPERATOR_BUNDLE_TAG")
        if not override:
            msg = "Cannot determine operator bundle tag. Set MAIN_IMAGE_TAG to a valid value or set ROXIE_E2E_OPERATOR_BUNDLE_TAG."
            print(msg, flush=True)
            pytest.skip(msg)
        operator_tag = override

    image = f"quay.io/rhacs-eng/stackrox-operator-bundle:{operator_tag}"
    try:
        # skopeo inspect is fast and does not pull; silence output unless error
        subprocess.run(
            [skopeo, "inspect", "--raw", f"docker://{image}"],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.PIPE,
            text=True,
            check=True,
        )
    except subprocess.CalledProcessError as e:
        detail = (e.stderr or "").strip()
        msg = f"Preflight failed to access {image}. Ensure you are logged in or provide an accessible tag via MAIN_IMAGE_TAG/ROXIE_E2E_OPERATOR_BUNDLE_TAG.\n{detail}"
        print(msg, flush=True)
        pytest.skip(msg)


def _load_envrc_env(path: str) -> dict[str, str]:
    expanded = os.path.expanduser(path)
    if not os.path.exists(expanded):
        return {}
    values = dotenv_values(expanded)
    return {k: v for k, v in values.items() if v is not None}


def maybe_skip_operator_test():
    if os.environ.get("SKIP_OPERATOR_TESTS"):
        pytest.skip("SKIP_OPERATOR_TESTS")


def test_deploy_central_and_secured_cluster(e2e_envrc_path):
    maybe_skip_operator_test()

    env = os.environ.copy()
    # Force roxie to use nix develop for e2e to ensure dependencies are present
    env.pop("IN_NIX_SHELL", None)
    # Reduce Python stdio buffering in child processes for more immediate output
    env["PYTHONUNBUFFERED"] = "1"

    # Deploy central
    print("=== Deploying central ===", flush=True)
    _preflight_operator_bundle_pull(env)
    _run([roxie_path, "deploy", "central", "--envrc", e2e_envrc_path] + common_deploy_args, env=env, timeout=1800)

    merged_env = env.copy()
    envrc_env = _load_envrc_env(e2e_envrc_path)
    print("Loaded environment from ~/.envrc.roxie for secured-cluster", flush=True)
    merged_env.update(envrc_env)

    print("=== Deploying secured-cluster ===", flush=True)
    # Deploy secured-cluster with env from ~/.envrc.roxie
    _run([roxie_path, "deploy", "secured-cluster"] + common_deploy_args, env=merged_env, timeout=1800)

    # Basic smoke checks: namespaces should exist (operator defaults)
    # Central
    print("Verifying namespace: acs-central", flush=True)
    subprocess.run(["kubectl", "get", "namespace", "acs-central"], check=True)
    # Environment for secured-cluster was prepared from ~/.envrc.roxie above
    # Secured-cluster
    print("Verifying namespace: acs-sensor", flush=True)
    subprocess.run(["kubectl", "get", "namespace", "acs-sensor"], check=True)

    # Give the cluster a moment to settle to reduce flakiness before teardown in follow-up scenarios
    print("Pausing briefly before exit...", flush=True)
    time.sleep(5)


def test_teardown_central_and_secured_cluster():
    maybe_skip_operator_test()

    env = os.environ.copy()
    env.pop("IN_NIX_SHELL", None)
    env["PYTHONUNBUFFERED"] = "1"

    # Merge env from ~/.envrc.roxie if present
    merged_env = env.copy()
    envrc_env = _load_envrc_env("~/.envrc.roxie")
    if envrc_env:
        print("Loaded environment from ~/.envrc.roxie for teardown", flush=True)
        merged_env.update(envrc_env)
        merged_env.pop("IN_NIX_SHELL", None)
    else:
        print("~/.envrc.roxie not found or empty; proceeding with current environment", flush=True)

    print("=== Tearing down central and secured-cluster ===", flush=True)
    _run([roxie_path, "teardown", "both"] + common_deploy_args, env=merged_env, timeout=1800)

    # Verify namespaces are deleted
    def _ns_absent(ns: str) -> None:
        res = subprocess.run(["kubectl", "get", "namespace", ns], capture_output=True, text=True)
        assert res.returncode != 0, f"Namespace {ns} still exists: {res.stdout or res.stderr}"

    print("Verifying namespaces are removed", flush=True)
    _ns_absent("acs-central")
    _ns_absent("acs-sensor")


def test_deploy_both_components_together(e2e_envrc_path):
    maybe_skip_operator_test()

    env = os.environ.copy()
    env.pop("IN_NIX_SHELL", None)
    env["PYTHONUNBUFFERED"] = "1"

    print("=== Deploying both components ===", flush=True)
    _preflight_operator_bundle_pull(env)
    _run([roxie_path, "deploy", "both", "--envrc", e2e_envrc_path] + common_deploy_args, env=env, timeout=2400)

    print("Verifying namespace: acs-central", flush=True)
    subprocess.run(["kubectl", "get", "namespace", "acs-central"], check=True)
    print("Verifying namespace: acs-sensor", flush=True)
    subprocess.run(["kubectl", "get", "namespace", "acs-sensor"], check=True)


def test_deploy_central_and_secured_cluster_via_helm(e2e_envrc_path):
    env = os.environ.copy()
    env.pop("IN_NIX_SHELL", None)
    env["PYTHONUNBUFFERED"] = "1"

    print("=== Deploying central via Helm ===", flush=True)
    _run(
        [roxie_path, "deploy", "central", "--helm", "--envrc", e2e_envrc_path] + common_deploy_args,
        env=env,
        timeout=2400,
    )

    merged_env = env.copy()
    envrc_env = _load_envrc_env(e2e_envrc_path)
    print("Loaded environment from ~/.envrc.roxie for secured-cluster", flush=True)
    merged_env.update(envrc_env)

    print("=== Deploying secured-cluster via Helm ===", flush=True)
    _run([roxie_path, "deploy", "secured-cluster", "--helm"] + common_deploy_args, env=merged_env, timeout=2400)

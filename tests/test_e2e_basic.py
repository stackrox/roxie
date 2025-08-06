import os
import shutil
import subprocess
import sys
import time
from dotenv import dotenv_values

import pytest

# Light import to reuse tag conversion logic for preflight
try:
    from deployer import ACSDeployer  # type: ignore
except Exception:  # noqa: BLE001
    ACSDeployer = None  # type: ignore


pytestmark = pytest.mark.e2e


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


def _run(cmd: list[str], env: dict[str, str] | None = None, timeout: int = 900) -> subprocess.CompletedProcess[str]:
    print(f"$ {' '.join(cmd)}", flush=True)
    # Stream output in real time
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
    # Determine container tool
    tool = shutil.which("podman") or shutil.which("docker")
    if not tool:
        msg = "Neither podman nor docker found; skipping e2e"
        print(msg, flush=True)
        pytest.skip(msg)

    # Compute operator tag using deployer logic if available
    operator_tag = None
    if ACSDeployer is not None:
        try:
            d = ACSDeployer()  # type: ignore[call-arg]
            operator_tag = getattr(d, "operator_tag", None)
        except Exception:
            operator_tag = None

    if not operator_tag:
        # Fallback: expect override
        override = env.get("ROXIE_E2E_OPERATOR_BUNDLE_TAG")
        if not override:
            msg = (
                "Cannot determine operator bundle tag. Set MAIN_IMAGE_TAG to a valid value or set ROXIE_E2E_OPERATOR_BUNDLE_TAG."
            )
            print(msg, flush=True)
            pytest.skip(msg)
        operator_tag = override

    image = f"quay.io/rhacs-eng/stackrox-operator-bundle:{operator_tag}"
    print(f"Preflight: pulling {image} with {tool}")
    try:
        subprocess.run([tool, "pull", image], check=True)
    except subprocess.CalledProcessError:
        msg = (
            f"Preflight failed to pull {image}. Ensure you are logged in or provide an accessible tag via MAIN_IMAGE_TAG/ROXIE_E2E_OPERATOR_BUNDLE_TAG."
        )
        print(msg, flush=True)
        pytest.skip(msg)


def _load_envrc_env(path: str) -> dict[str, str]:
    expanded = os.path.expanduser(path)
    if not os.path.exists(expanded):
        return {}
    values = dotenv_values(expanded)
    return {k: v for k, v in values.items() if v is not None}


def test_deploy_central_and_secured_cluster():
    _require_binary("kubectl")
    _require_binary("nix")
    ctx = _require_kube_context()
    print(f"Using kubectl context: {ctx}")

    repo_root = os.path.dirname(os.path.abspath(__file__))
    # tests/ -> project root
    repo_root = os.path.abspath(os.path.join(repo_root, os.pardir))

    roxie_path = os.path.join(repo_root, "bin", "roxie")
    if not os.path.exists(roxie_path):
        pytest.skip("bin/roxie not found; skipping e2e")

    # Ensure MAIN_IMAGE_TAG is provided for operator path if required by code
    env = os.environ.copy()
    env.setdefault("MAIN_IMAGE_TAG", env.get("MAIN_IMAGE_TAG", "4.12.x-1-gdeadbee"))
    # Force roxie to use nix develop for e2e to ensure dependencies are present
    env.pop("IN_NIX_SHELL", None)
    # Reduce Python stdio buffering in child processes for more immediate output
    env["PYTHONUNBUFFERED"] = "1"

    # Prefer operator by default (no --helm flag). Deploy central
    print("=== Deploying central ===", flush=True)
    _preflight_operator_bundle_pull(env)
    _run([roxie_path, "deploy", "central"], env=env, timeout=1800)

    merged_env = env.copy()
    envrc_env = _load_envrc_env("~/.envrc.roxie")
    print("Loaded environment from ~/.envrc.roxie for secured-cluster", flush=True)
    merged_env.update(envrc_env)

    print("=== Deploying secured-cluster ===", flush=True)
    # Deploy secured-cluster with env from ~/.envrc.roxie
    _run([roxie_path, "deploy", "secured-cluster"], env=merged_env, timeout=1800)

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
    _require_binary("kubectl")
    _require_binary("nix")
    ctx = _require_kube_context()
    print(f"Using kubectl context: {ctx}")

    repo_root = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(repo_root, os.pardir))
    roxie_path = os.path.join(repo_root, "bin", "roxie")
    if not os.path.exists(roxie_path):
        pytest.skip("bin/roxie not found; skipping e2e")

    # Base env
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
    _run([roxie_path, "teardown", "both"], env=merged_env, timeout=1800)

    # Verify namespaces are deleted
    def _ns_absent(ns: str) -> None:
        res = subprocess.run(["kubectl", "get", "namespace", ns], capture_output=True, text=True)
        assert res.returncode != 0, f"Namespace {ns} still exists: {res.stdout or res.stderr}"

    print("Verifying namespaces are removed", flush=True)
    _ns_absent("acs-central")
    _ns_absent("acs-sensor")

def test_deploy_both_components_together():
    _require_binary("kubectl")
    _require_binary("nix")
    ctx = _require_kube_context()
    print(f"Using kubectl context: {ctx}")

    repo_root = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(repo_root, os.pardir))
    roxie_path = os.path.join(repo_root, "bin", "roxie")
    if not os.path.exists(roxie_path):
        pytest.skip("bin/roxie not found; skipping e2e")

    env = os.environ.copy()
    env.setdefault("MAIN_IMAGE_TAG", env.get("MAIN_IMAGE_TAG", "4.12.x-1-gdeadbee"))
    env.pop("IN_NIX_SHELL", None)
    env["PYTHONUNBUFFERED"] = "1"

    # Merge env from ~/.envrc.roxie for secured-cluster bits
    merged_env = env.copy()
    envrc_env = _load_envrc_env("~/.envrc.roxie")
    if envrc_env:
        print("Loaded environment from ~/.envrc.roxie for both-components deploy", flush=True)
        merged_env.update(envrc_env)

    print("=== Deploying both components ===", flush=True)
    _preflight_operator_bundle_pull(merged_env)
    _run([roxie_path, "deploy", "both"], env=merged_env, timeout=2400)

    print("Verifying namespace: acs-central", flush=True)
    subprocess.run(["kubectl", "get", "namespace", "acs-central"], check=True)
    print("Verifying namespace: acs-sensor", flush=True)
    subprocess.run(["kubectl", "get", "namespace", "acs-sensor"], check=True)

def test_deploy_central_and_secured_cluster_via_helm():
    _require_binary("kubectl")
    _require_binary("nix")
    ctx = _require_kube_context()
    print(f"Using kubectl context: {ctx}")

    repo_root = os.path.dirname(os.path.abspath(__file__))
    repo_root = os.path.abspath(os.path.join(repo_root, os.pardir))
    roxie_path = os.path.join(repo_root, "bin", "roxie")
    if not os.path.exists(roxie_path):
        pytest.skip("bin/roxie not found; skipping e2e")

    env = os.environ.copy()
    env.pop("IN_NIX_SHELL", None)
    env["PYTHONUNBUFFERED"] = "1"

    print("=== Deploying central via Helm ===", flush=True)
    _run([roxie_path, "deploy", "central", "--helm"], env=env, timeout=2400)

    merged_env = env.copy()
    envrc_env = _load_envrc_env("~/.envrc.roxie")
    print("Loaded environment from ~/.envrc.roxie for secured-cluster", flush=True)
    merged_env.update(envrc_env)

    print("=== Deploying secured-cluster via Helm ===", flush=True)
    _run([roxie_path, "deploy", "secured-cluster", "--helm"], env=env, timeout=2400)

    print("Verifying namespace: acs-central-helm", flush=True)
    subprocess.run(["kubectl", "get", "namespace", "acs-central-helm"], check=True)
    print("Verifying namespace: acs-sensor-helm", flush=True)
    subprocess.run(["kubectl", "get", "namespace", "acs-sensor-helm"], check=True)


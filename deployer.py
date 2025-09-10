"""
Core deployment logic for the roxie deployment tool.

This module contains the ACSDeployer class and all deployment functionality
for both Helm-based and operator-based deployments.
"""

import base64
import math
import os
import secrets
import string
import subprocess
import tempfile
import time
from typing import Any

import yaml
from rich.console import Console
from rich.panel import Panel
from rich.progress import (
    BarColumn,
    Progress,
    SpinnerColumn,
    TextColumn,
)

from docker_auth import DockerAuth
from errors import RoxieError
from helpers import TimestampColumn
from image_cache import ImageCache
from logger import Logger

default_main_image_tag = "4.8.2"

class ACSDeployer:
    """Deploys Advanced Cluster Security (ACS) using Kubernetes and Helm"""

    def __init__(self, console: Any | None = None, cache_enabled: bool = True):
        """Initialize ACS Deployer with configuration"""
        self.logger = Logger()
        self.start_time = time.time()
        self.console = console or Console()
        self.docker_auth = DockerAuth(console=self.console, cache_enabled=cache_enabled)
        self.image_cache = ImageCache(self.logger)
        self.central_env_file = os.environ.get("ROXIE_ENVRC", os.path.expanduser("~/.envrc.roxie"))
        self.central_password = os.environ.get("ROX_ADMIN_PASSWORD") or self.generate_password()
        self.rox_ca_cert_file = os.environ.get("ROX_CA_CERT_FILE", None)
        self.kubectl = os.environ.get("ORCH_CMD", "kubectl")
        self.central_namespace = "acs-central"
        self.secured_cluster_namespace = "acs-sensor"
        self.central_namespace = "acs-central"
        self.secured_cluster_namespace = "acs-sensor"
        self.main_image_tag = self.lookup_main_image_tag()
        self.operator_tag = self.convert_main_tag_to_operator_tag(self.main_image_tag)
        self.central_endpoint = os.environ.get("API_ENDPOINT", "")
        self.log_file = os.environ.get("LOG_FILE", self.create_temp_log())
        self.roxctl_version = self.get_roxctl_version()
        self.cluster_name = ""  # Will be set during secured cluster deployment
        self.logger.print_with_timestamp("🚀 ACS Deployer initialized", style="bold green")
        self.kube_context = self.get_current_context()

        self.logger.print_with_timestamp(f"Kubernetes context: {self.kube_context}", style="bold cyan")

    def deploy(self, component: str = "both") -> None:
        """Deploy ACS components - should be implemented by subclasses"""
        raise NotImplementedError("deploy method must be implemented by subclasses")

    def lookup_latest_tag_from_stackrox_git_root(self) -> str | None:
        """Lookup latest tag from stackrox git root"""
        stackrox_git_root = os.environ.get("STACKROX_GIT_ROOT", "").strip()
        if stackrox_git_root:
            return self.get_latest_commit_tag_from_dir()
        return None

    def lookup_main_image_tag(self) -> str:
        """Lookup main image tag from the environment"""
        main_image_tag = os.environ.get("MAIN_IMAGE_TAG", "").strip()
        if main_image_tag:
            return main_image_tag
        latest_main_image_tag = self.lookup_latest_tag_from_stackrox_git_root()
        if latest_main_image_tag:
            return latest_main_image_tag
        return default_main_image_tag

    def get_timestamp(self) -> str:
        """Get relative timestamp since start"""
        elapsed = time.time() - self.start_time
        minutes = int(elapsed // 60)
        seconds = int(elapsed % 60)
        return f"{minutes:02d}:{seconds:02d}"

    def get_roxctl_version(self) -> str:
        """Get roxctl version with error handling"""
        try:
            result = subprocess.run(["roxctl", "version"], capture_output=True, text=True, check=True, timeout=10)
            return result.stdout.strip()
        except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError) as e:
            # Surface a clear error upstream; include stderr/stdout when available
            detail = ""
            try:
                if hasattr(e, "stderr") and e.stderr:
                    stderr_str = e.stderr.decode() if isinstance(e.stderr, bytes) else e.stderr
                    detail = f": {stderr_str.strip()}"
                elif hasattr(e, "stdout") and e.stdout:
                    stdout_str = e.stdout.decode() if isinstance(e.stdout, bytes) else e.stdout
                    detail = f": {stdout_str.strip()}"
            except Exception:
                detail = ""
            raise RuntimeError(f"roxctl invocation failed: {detail}") from e

    def create_temp_log(self) -> str:
        """Create a temporary log file"""
        fd, path = tempfile.mkstemp(suffix=".log", text=True)
        os.close(fd)  # close immediately, we only care about the path
        return path

    def generate_password(self) -> str:
        """Generate a random 20-character alphanumeric password"""
        chars = string.ascii_letters + string.digits
        return "".join(secrets.choice(chars) for _ in range(20))

    def convert_main_tag_to_operator_tag(self, main_tag: str) -> str:
        """Convert main image tag format to operator tag format

        Conversion pattern:
        Main:     4.9.x-441-g7754d5a916
        Operator: v4.9.0-441-g7754d5a916

        Changes:
        1. Add 'v' prefix
        2. Replace 'x' with '0'
        """

        if main_tag.strip() == "":
            raise RoxieError("Main image tag is empty")

        operator_tag = main_tag

        # Remove -dirty suffix if present
        if "dirty" in operator_tag:
            operator_tag = operator_tag.replace("-dirty", "")

        # Apply transformation: add 'v' prefix and replace 'x' with '0'
        # Example: "4.9.x-441-g7754d5a916" -> "v4.9.0-441-g7754d5a916"
        operator_tag = f"v{operator_tag.replace('.x', '.0')}"

        return operator_tag

    def get_latest_commit_tag_from_dir(self) -> str:
        """Get latest commit tag from git repo"""
        try:
            stackrox_git_root = os.environ.get("STACKROX_GIT_ROOT", "").strip()
            if not stackrox_git_root:
                raise RoxieError("STACKROX_GIT_ROOT environment variable is not set")

            if not os.path.isdir(stackrox_git_root):
                raise RoxieError(f"STACKROX_GIT_ROOT directory does not exist: {stackrox_git_root}")

            if not os.path.isdir(os.path.join(stackrox_git_root, ".git")):
                raise RoxieError(f"STACKROX_GIT_ROOT is not a git repository: {stackrox_git_root}")

            # Execute make tag command
            result = subprocess.run(
                ["make", "--quiet", "--no-print-directory", "tag"],
                cwd=stackrox_git_root,
                capture_output=True,
                text=True,
                check=True,
                timeout=30,
            )

            tag = result.stdout.strip()
            if not tag:
                raise RoxieError(f"make tag command in STACKROX_GIT_ROOT ({stackrox_git_root}) returned empty output")

            return tag

        except (subprocess.CalledProcessError, subprocess.TimeoutExpired, FileNotFoundError, ValueError) as e:
            # Log the specific error for debugging
            error_msg = str(e)
            if hasattr(e, "stderr") and e.stderr:
                stderr_val = e.stderr.decode() if isinstance(e.stderr, bytes | bytearray) else str(e.stderr)
                error_msg += f" (stderr: {stderr_val.strip()})"

            # Fallback to a default value if make tag fails
            return f"latest (error: {error_msg})"

    def print_with_timestamp(self, message: str, style: str | None = None) -> None:
        """Print message with timestamp prefix"""
        timestamp = self.get_timestamp()
        if style:
            self.console.print(f"[green]{timestamp}[/green] {message}", style=style)
        else:
            self.console.print(f"[green]{timestamp}[/green] {message}")

    def create_progress_with_timestamp(self, include_bar: bool = False, **kwargs) -> Progress:
        """Create a Progress instance with live timestamp column"""
        columns = [
            TimestampColumn(self.start_time),
            SpinnerColumn(),
            TextColumn("[progress.description]{task.description}"),
        ]
        if include_bar:
            columns.extend([BarColumn(complete_style="green", finished_style="green")])
        return Progress(*columns, console=self.console, **kwargs)

    def log(self, message: str) -> None:
        """Print log message with green styling"""
        timestamp = self.get_timestamp()
        self.console.print(f"[green]{timestamp}[/green] {message}", style="bold green")

    def prepare_namespace(self, namespace: str):
        """Prepare Kubernetes namespace with required resources"""
        # Create pull secret using Python function
        try:
            pull_secret_yaml = self.docker_auth.create_pull_secret_yaml(namespace)
            # f"Applying pull secret to {namespace}",
            subprocess.run(
                [self.kubectl, "-n", namespace, "apply", "-f", "-"],
                input=pull_secret_yaml.encode("utf-8"),
                capture_output=True,
                check=True,
            )
        except Exception as e:
            self.logger.error(f"Failed to create pull secret: {str(e)}")
            return False

    def namespace_exist(self, namespace: str) -> bool:
        """Check if Helm release doesn't exist"""
        result = subprocess.run(
            [self.kubectl, "get", "namespace", namespace], stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL
        )
        return result.returncode == 0

    def wait_for_central_endpoint(self, namespace: str):
        """Wait for LoadBalancer IP and store endpoint"""

        with self.create_progress_with_timestamp(include_bar=True, transient=True) as progress:
            task = progress.add_task("Waiting for LoadBalancer IP", total=120)

            for _i in range(30):
                try:
                    result = subprocess.run(
                        [
                            self.kubectl,
                            "-n",
                            namespace,
                            "get",
                            "service",
                            "central-loadbalancer",
                            "-o",
                            "jsonpath={.status.loadBalancer.ingress[0].ip}",
                        ],
                        capture_output=True,
                        text=True,
                        check=True,
                    )

                    lb_ip = result.stdout.strip()
                    if lb_ip and lb_ip != "<none>":
                        # Store the endpoint
                        self.central_endpoint = f"{lb_ip}:443"
                        # Immediately print success message to overwrite progress bar
                        progress.stop()
                        self.logger.print_with_timestamp(f"✓ Got LoadBalancer IP: {lb_ip}", style="bold green")
                        return

                    progress.update(task, advance=4)
                    time.sleep(1)

                except (subprocess.CalledProcessError, subprocess.TimeoutExpired):
                    progress.update(task, advance=1)
                    time.sleep(1)

            progress.stop()
            raise RoxieError("Timeout waiting for LoadBalancer IP")

    def get_current_context(self) -> str:
        """Get current kubectl context"""
        try:
            result = subprocess.run(
                [self.kubectl, "config", "current-context"], capture_output=True, text=True, check=True
            )
            return result.stdout.strip()
        except Exception as e:
            self.logger.error(f"Failed to get current context: {str(e)}")
            raise

    def initiate_namespace_deletion(self, namespaces: list[str], wait: bool = False):
        """Initiate deletion of one or more namespaces"""
        for namespace in namespaces:
            cmd = [self.kubectl, "delete", "namespace", namespace, "--force", "--grace-period=0"]
            if not wait:
                cmd.insert(-3, "--wait=false")  # Insert before --force

            try:
                if wait:
                    # Synchronous deletion - wait for completion
                    subprocess.run(cmd, stderr=subprocess.DEVNULL, check=True)
                else:
                    # Asynchronous deletion - fire and forget
                    subprocess.Popen(cmd, stdout=subprocess.DEVNULL, stderr=subprocess.DEVNULL)
            except subprocess.CalledProcessError:
                # For synchronous deletion, we want to know about failures
                if wait:
                    raise
                # For async deletion, ignore failures (namespace might not exist)
            except Exception:  # noqa: S110
                # Always ignore other exceptions (process creation failures, etc.)
                pass

    def wait_for_namespaces_deletion(self, namespaces: list[str], timeout_seconds: int = 300):
        """Wait for one or more namespaces to be completely deleted"""
        if not namespaces:
            return

        progress_msg = "Waiting for namespaces to be deleted"

        with self.create_progress_with_timestamp() as progress:
            task = progress.add_task(progress_msg, total=None)

            for namespace in namespaces:
                self._attempt_clear_namespace_finalizers(namespace)

            start_time = time.time()
            while time.time() - start_time < timeout_seconds:
                try:
                    # Check each namespace individually
                    existing_namespaces = []
                    terminating_count = 0

                    for namespace in namespaces:
                        result = subprocess.run(
                            [
                                self.kubectl,
                                "get",
                                "namespace",
                                namespace,
                                "-o",
                                "custom-columns=NAME:.metadata.name,STATUS:.status.phase",
                                "--no-headers",
                            ],
                            capture_output=True,
                            text=True,
                            check=False,
                        )

                        if result.returncode == 0 and result.stdout.strip():
                            # Namespace exists, check its status
                            line = result.stdout.strip()
                            existing_namespaces.append(line)
                            if "Terminating" in line:
                                terminating_count += 1

                    # If no namespaces exist, we're done
                    if not existing_namespaces:
                        break

                    # Update progress with current status
                    if len(namespaces) == 1:
                        # Single namespace - show terminating status
                        if terminating_count > 0:
                            progress.update(
                                task, description=f"Waiting for {namespaces} namespace to be deleted (terminating)"
                            )
                    else:
                        # Multiple namespaces - show counts
                        if terminating_count > 0:
                            progress.update(
                                task,
                                description=f"Waiting for namespaces to be deleted ({terminating_count} terminating)",
                            )
                        else:
                            progress.update(
                                task,
                                description=f"Waiting for namespaces to be deleted ({len(existing_namespaces)} remaining)",
                            )

                    time.sleep(1)

                except Exception:
                    # If we can't check status, assume they're being deleted
                    time.sleep(1)

            # Final check to ensure namespaces are really gone
            try:
                remaining_namespaces = []
                for namespace in namespaces:
                    result = subprocess.run(
                        [self.kubectl, "get", "namespace", namespace, "-o", "name"],
                        capture_output=True,
                        text=True,
                        check=False,
                    )

                    if result.returncode == 0 and result.stdout.strip():
                        remaining_namespaces.append(namespace)

                if remaining_namespaces:
                    # Some namespaces still exist after timeout
                    self.logger.print_with_timestamp(
                        f"⚠ Timeout: {len(remaining_namespaces)} namespace(s) still exist after {timeout_seconds}s",
                        style="bold yellow",
                    )
                    for ns in remaining_namespaces:
                        self.logger.print_with_timestamp(f"  - {ns}", style="yellow")
                    raise RuntimeError(
                        f"Timeout: {len(remaining_namespaces)} namespace(s) still exist after {timeout_seconds}s: {', '.join(remaining_namespaces)}"
                    )

            except RuntimeError:
                # Re-raise our timeout exception
                raise
            except Exception:  # noqa: S110
                # If we can't check, assume success
                pass

        # Success message
        if len(namespaces) == 1:
            self.logger.print_with_timestamp(f"✓ namespace {namespaces[0]} deleted", style="bold green")
        else:
            self.logger.print_with_timestamp("✓ All namespaces deleted", style="bold green")

    def _attempt_clear_namespace_finalizers(self, namespace: str) -> None:
        """Best-effort removal of namespace finalizers to accelerate deletion."""
        try:
            subprocess.run(
                [
                    self.kubectl,
                    "patch",
                    "namespace",
                    namespace,
                    "-p",
                    '{"metadata":{"finalizers":[]}}',
                    "--type=merge",
                ],
                check=False,
                capture_output=True,
                text=True,
            )
        except Exception as e:
            # Best-effort only; log and continue
            self.logger.print_with_timestamp(
                f"Finalizer clear attempt failed for {namespace}: {e}", style="dim yellow"
            )

    def teardown_all_async(self):
        """Teardown all ACS namespaces asynchronously"""
        try:
            # Start namespace deletion in background
            self.initiate_namespace_deletion(
                [
                    self.central_namespace,
                    self.secured_cluster_namespace,
                    self.central_namespace,
                    self.secured_cluster_namespace,
                ],
                wait=False,
            )

            # Wait for namespaces to be completely deleted
            self.wait_for_namespaces_deletion(
                [
                    self.central_namespace,
                    self.secured_cluster_namespace,
                    self.central_namespace,
                    self.secured_cluster_namespace,
                ],
                timeout_seconds=600,
            )

        except Exception as e:
            self.logger.error(f"Failed to teardown namespaces: {str(e)}")
            raise

    def teardown(self, component: str = "both"):
        if component == "central":
            self.teardown_central()
        elif component == "secured-cluster":
            self.teardown_secured_cluster()
        elif component == "both":
            self.teardown_secured_cluster()
            self.teardown_central()
        else:
            raise RoxieError(f"Unknown component for operator teardown: {component}")

    def teardown_central(self):
        namespace = self.central_namespace
        self.logger.print_with_timestamp(
            f"🗑️  Tearing down resources in: {namespace}", style="bold cyan"
        )

        result = subprocess.run([self.kubectl, "get", "namespace", namespace], capture_output=True, text=True)
        if result.returncode != 0:
            self.logger.print_with_timestamp(
                f"Namespace '{namespace}' does not exist - nothing to teardown", style="dim yellow"
            )
            return

        # Try to delete Central CR.
        subprocess.run(
            [
                self.kubectl,
                "-n",
                namespace,
                "delete",
                "central",
                "--all",
                "--ignore-not-found=true",
                "--timeout=120s",
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        self.teardown_namespace(namespace)

    def teardown_secured_cluster(self):
        namespace = self.secured_cluster_namespace
        self.logger.print_with_timestamp(
            f"🗑️  Tearing down resources in: {namespace}", style="bold cyan"
        )

        result = subprocess.run([self.kubectl, "get", "namespace", namespace], capture_output=True, text=True)
        if result.returncode != 0:
            self.logger.print_with_timestamp(
                f"Namespace '{namespace}' does not exist - nothing to teardown", style="dim yellow"
            )
            return

        # Try to delete SecuredCluster CR.
        subprocess.run(
            [
                self.kubectl,
                "delete",
                "securedcluster",
                "--all",
                "-n",
                namespace,
                "--ignore-not-found=true",
                "--timeout=120s",
            ],
            check=False,
            capture_output=True,
            text=True,
        )
        self.teardown_namespace(namespace)

    def teardown_namespace(self, namespace: str):
        # Force delete workloads
        try:
            subprocess.run(
                [
                    self.kubectl,
                    "-n",
                    namespace,
                    "delete",
                    "pods,deployments,daemonsets",
                    "--all",
                    "--force",
                    "--grace-period=0",
                    "--ignore-not-found=true",
                    "--timeout=60s",
                ],
                check=False,
                capture_output=True,
                text=True,
            )
        except Exception as e:
            self.logger.print_with_timestamp(f"Warning: failed force-deleting workloads: {e}", style="dim yellow")

        # Delete other namespaced resources
        for args, label in [
            (["all", "--all"], "all resources"),
            (["secrets,configmaps", "--all"], "secrets/configmaps"),
            (["pvc", "--all"], "PVCs"),
        ]:
            try:
                subprocess.run(
                    [self.kubectl, "-n", namespace, "delete", *args, "--ignore-not-found=true", "--timeout=120s"],
                    check=False,
                    capture_output=True,
                    text=True,
                )
            except Exception as e:
                self.logger.print_with_timestamp(
                    f"Warning: failed deleting {label} in {namespace}: {e}", style="dim yellow"
                )

        # Delete the namespace asynchronously and wait until gone.
        self.initiate_namespace_deletion([namespace], wait=False)
        self.wait_for_namespaces_deletion([namespace], timeout_seconds=600)

    def apply_admin_password_secret(self, name: str) -> None:
        secret = {
            "apiVersion": "v1",
            "kind": "Secret",
            "metadata": {"namespace": self.central_namespace, "name": name},
            "stringData": {"password": self.central_password},
        }
        subprocess.run(
            [self.kubectl, "apply", "-f", "-"], input=yaml.dump(secret), capture_output=True, text=True, check=True
        )

    def generate_crs(self, cluster_name: str) -> str:
        """Generate CRS (Central Resource Secret) for secured cluster deployment"""
        # Retry up to 3 times on network-related transient errors
        retryable_error_substrings = [
            "connection refused",
            "connection reset",
            "connection timed out",
            "timed out",
            "timeout",
            "network is unreachable",
            "temporary failure in name resolution",
            "no route to host",
            "tls handshake timeout",
            "eof",
            "bad gateway",
            "service unavailable",
            "context deadline exceeded"
        ]

        max_attempts = 3
        for attempt_number in range(1, max_attempts + 1):
            try:
                env = {**os.environ, "ROX_ADMIN_PASSWORD": self.central_password}
                if self.rox_ca_cert_file:
                    env["ROX_CA_CERT_FILE"] = self.rox_ca_cert_file
                result = subprocess.run(
                    [
                        "roxctl",
                        "-e",
                        self.central_endpoint,
                        "central",
                        "crs",
                        "generate",
                        cluster_name,
                        "--output=-",
                    ],
                    capture_output=True,
                    text=True,
                    check=True,
                    env=env,
                )
                return result.stdout.strip()
            except (subprocess.CalledProcessError, subprocess.TimeoutExpired) as e:
                # Decide whether to retry based on error text
                error_output = ""
                try:
                    if hasattr(e, "stderr") and e.stderr:
                        error_output = e.stderr.decode() if isinstance(e.stderr, bytes) else e.stderr
                    elif hasattr(e, "stdout") and e.stdout:
                        error_output = e.stdout.decode() if isinstance(e.stdout, bytes) else e.stdout
                except Exception:
                    error_output = ""

                error_output_lc = (error_output or "").lower()
                should_retry = isinstance(e, subprocess.TimeoutExpired) or any(
                    substr in error_output_lc for substr in retryable_error_substrings
                )

                if should_retry and attempt_number < max_attempts:
                    backoff_seconds = 2 ** (attempt_number - 1)
                    # Use secrets for jitter to avoid S311 and ensure non-predictable jitter
                    # Uniform [0, 0.25): derive from integer range to avoid floating RNG
                    jitter_quarters = secrets.randbelow(250)  # 0..249
                    jitter_seconds = jitter_quarters / 1000.0
                    total_sleep = backoff_seconds + jitter_seconds
                    self.logger.print_with_timestamp(
                        f"Transient network error from roxctl (attempt {attempt_number}/{max_attempts}). Retrying in {total_sleep:.2f}s...",
                        style="yellow",
                    )
                    time.sleep(total_sleep)
                    continue

                # Final attempt or non-retryable error
                detail = ""
                try:
                    if error_output:
                        detail = f": {error_output.strip()}"
                except Exception:
                    detail = ""
                self.logger.error(f"CRS issuing failed{detail}")
                raise

        # This should never be reached due to the raise above, but mypy requires it
        raise RuntimeError("Unexpected code path in generate_crs")

    def apply_yaml_to_namespace(self, namespace: str, crs_content: str):
        """Apply CRS content as a Kubernetes Secret for operator consumption"""
        subprocess.run(
            [self.kubectl, "apply", "-n", namespace, "-f", "-"],
            input=crs_content,
            capture_output=True,
            text=True,
            check=True,
        )

    def show_secured_cluster_success_panel(self):
        """Show success panel and write environment file for operator SecuredCluster deployment"""
        success_panel = Panel.fit(
            f"[bold green]✓ Secured Cluster Deployment Complete[/bold green]\n\n"
            f"[bold]Namespace:        [/bold] {self.secured_cluster_namespace}\n"
            f"[bold]Cluster Name:     [/bold] {self.cluster_name}\n"
            f"[bold]Deployment Mode:  [/bold] Operator\n"
            f"[bold]Central Endpoint: [/bold] {self.central_endpoint}\n"
            f"[bold]Log File:         [/bold] {self.log_file}",
            title="[bold green]SecuredCluster Deployment Success[/bold green]",
            border_style="green",
        )
        self.console.print(success_panel)

    def get_central_endpoint(self, namespace: str) -> str:
        """Get Central endpoint from LoadBalancer or service"""
        try:
            # Try to get LoadBalancer IP from Central service

            # Check for Central service with LoadBalancer
            result = subprocess.run(
                [
                    self.kubectl,
                    "-n",
                    namespace,
                    "get",
                    "service",
                    "central-loadbalancer",
                    "-o",
                    "jsonpath={.status.loadBalancer.ingress[0].ip}",
                ],
                capture_output=True,
                text=True,
            )

            if result.returncode == 0 and result.stdout.strip():
                return result.stdout.strip()

            raise ValueError("No Central endpoint found")

        except Exception as e:
            self.logger.error(f"Failed to get central endpoint: {str(e)}")
            raise

    def ensure_namespace_exists(self, namespace: str):
        """Ensure the specified namespace exists, create if it doesn't"""
        try:
            # Check if namespace exists
            result = subprocess.run([self.kubectl, "get", "namespace", namespace], capture_output=True, text=True)

            if result.returncode == 0:
                self.logger.print_with_timestamp(f"Namespace '{namespace}' already exists", style="dim cyan")
                return True

            # Create namespace
            namespace_obj = {
                "apiVersion": "v1",
                "kind": "Namespace",
                "metadata": {"name": namespace, "labels": {"name": namespace}},
            }

            # f"Creating namespace: {namespace}"
            result = subprocess.run(
                [self.kubectl, "apply", "-f", "-"],
                input=yaml.dump(namespace_obj),
                check=True,  # , encoding="utf-8"), check=True,
                capture_output=True,
                text=True,
            )

        except Exception as e:
            self.logger.error(f"Failed to ensure namespace exists: {str(e)}")
            raise

    def wait_for_ready_deployment(self, namespace: str, deployment: str, timeout: int = 800):
        """Wait for deployment to become ready"""
        with self.create_progress_with_timestamp(include_bar=True) as progress:
            task = progress.add_task("Waiting for Central deployment readiness", total=timeout)

            start_time = time.time()
            steps_progressed = 0
            while time.time() - start_time < timeout:
                try:
                    # Check if central deployment exists and is ready
                    result = subprocess.run(
                        [
                            self.kubectl,
                            "get",
                            "deployment",
                            deployment,
                            "-n",
                            namespace,
                            "-o",
                            "jsonpath={.status.readyReplicas}",
                        ],
                        capture_output=True,
                        text=True,
                    )

                    if result.returncode == 0 and result.stdout.strip():
                        ready_replicas = result.stdout.strip()
                        if ready_replicas and ready_replicas != "0":
                            progress.stop()
                            self.logger.print_with_timestamp(f"✓ Deployment {deployment} ready", style="bold green")
                            return

                except Exception as e:
                    self.logger.print_with_timestamp(
                        f"Ignoring transient error while checking {deployment} readiness: {e}",
                        style="dim yellow",
                    )

                more_steps = math.floor(time.time() - start_time) - steps_progressed
                progress.update(task, advance=more_steps)
                steps_progressed += more_steps
                time.sleep(1)

            # Loop finished without success
            progress.stop()

        raise RoxieError(f"Timeout waiting for {deployment} deployment to become ready after {timeout}s")

    def fetch_central_ca_cert(self, namespace: str) -> str:
        """Fetch central CA certificate from Kubernetes secret and persist it to a temp file.

        Returns the path to the written PEM file.
        """
        try:
            # Get base64-encoded CA from the secret
            result = subprocess.run(
                [
                    self.kubectl,
                    "-n",
                    namespace,
                    "get",
                    "secret",
                    "central-tls",
                    "-o",
                    "jsonpath={.data.ca\\.pem}",
                ],
                capture_output=True,
                text=True,
                check=True,
            )

            encoded = (result.stdout or "").strip()
            if not encoded:
                raise RoxieError("central CA not found in secret central-tls")

            decoded_bytes = base64.b64decode(encoded)

            fd, path = tempfile.mkstemp(prefix="roxie-ca-", suffix=".pem", text=False)
            try:
                os.write(fd, decoded_bytes)
            finally:
                os.close(fd)

            self.rox_ca_cert_file = path
            return path

        except subprocess.CalledProcessError as e:
            detail = (e.stderr or e.stdout or "").strip()
            raise RoxieError(f"Failed to fetch central CA: {detail}") from e

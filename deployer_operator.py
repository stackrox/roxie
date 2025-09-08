import os
import subprocess
import time
from typing import Any, Dict, List, Optional
import random

import yaml
from rich.panel import Panel

import helpers
from deployer import ACSDeployer
from errors import RoxieError


class ACSDeployerOperator(ACSDeployer):
    """Operator-specific deployer that implements Operator deployment/teardown."""

    def teardown(self, component: str = "both"):
        self.teardown_operator_custom_resources(component)

    def apply_crds_to_cluster(self, crd_files: List[str]):
        """Apply CRD files to the cluster using kubectl"""
        self.logger.print_with_timestamp(f"Applying {len(crd_files)} CRD(s) to cluster", style="bold cyan")

        applied_count = 0
        for crd_file in crd_files:
            crd_filename = os.path.basename(crd_file)

            # Apply the CRD using kubectl
            try:
                subprocess.run([self.kubectl, "apply", "-f", crd_file], capture_output=True, text=True, check=True)
                applied_count += 1
            except subprocess.CalledProcessError as e:
                raise RoxieError(f"Failed to apply CRD: {crd_filename}") from e

            self.logger.print_with_timestamp(f"✓ Successfully applied {applied_count} CRD(s)", style="bold green")

    def deploy_rhacs_operator(self):
        operator_tag_for_image = self.operator_tag
        self.logger.print_with_timestamp(f"Operator tag: {self.operator_tag}", style="bold blue")
        bundle_image = f"quay.io/rhacs-eng/stackrox-operator-bundle:{operator_tag_for_image}"
        bundle_dir = self.download_and_extract_operator_bundle(bundle_image)

        self.logger.print_with_timestamp(f"Bundle image: {bundle_image}", style="bold blue")
        self.list_operator_bundle_contents(bundle_dir)
        crd_files = self.identify_crd_files(bundle_dir)
        self.apply_crds_to_cluster(crd_files)
        self.deploy_operator_from_csv(bundle_dir)

    def deploy(self, component: str = "both"):
        """Deploy specified component(s) using operator."""
        self.logger.print_with_timestamp("🚀 Initiating operator-based deployment of ACS", style="bold cyan")

        operator_exists_and_ready = self.is_operator_deployed_and_ready()
        if operator_exists_and_ready:
            if self.has_operator_version_mismatch(self.operator_tag):
                self.teardown_rhacs_operator()
                self.logger.print_with_timestamp(
                    "Cleaned up existing operator deployments", style="bold yellow"
                )
                self.deploy_rhacs_operator()
        else:
            # No operator present; just deploy it
            self.deploy_rhacs_operator()

        self.deploy_component(component)
        self.logger.print_with_timestamp("✓ Operator-based deployment completed successfully!", style="bold green")

    def deploy_central(self):
        self.teardown_all_async()
        self.ensure_namespace_exists(self.central_namespace_operator)
        self.prepare_namespace(self.central_namespace_operator)
        # Ensure CRDs are present before creating Central CR
        self.ensure_crds_installed()
        self.create_central_cr()
        self.show_central_success_panel()

    def deploy_secured_cluster(self):
        self.teardown_single_namespace(self.secured_cluster_namespace_operator, "secured cluster")
        self.teardown_single_namespace(self.secured_cluster_namespace, "secured cluster")
        self.ensure_namespace_exists(self.secured_cluster_namespace_operator)
        self.prepare_namespace(self.secured_cluster_namespace_operator)
        self.create_secured_cluster_cr("")

    def deploy_component(self, component: str):
        """Deploy Custom Resource for the specified component"""
        self.logger.print_with_timestamp(f"Deploying Custom Resources for: {component}", style="bold cyan")

        if component == "central":
            self.deploy_central()
        elif component == "secured-cluster":
            self.deploy_secured_cluster()
        elif component == "both":
            self.deploy_central()
            self.deploy_secured_cluster()
        self.logger.print_with_timestamp("✓ Custom Resources deployed successfully for", style="bold green")

    def create_central_cr(self):
        """Create Central Custom Resource for operator deployment"""
        self.apply_admin_password_secret("admin-password")
        # Create Central CR specification
        central_cr = {
            "apiVersion": "platform.stackrox.io/v1alpha1",
            "kind": "Central",
            "metadata": {
                "name": "stackrox-central-services",
                "namespace": self.central_namespace_operator,
                "labels": {
                    "app": "stackrox-central"
                },
            },
            "spec": {
                "central": {
                    "adminPasswordSecret": {
                        "name": "admin-password"
                    },
                    "exposure": {
                        "loadBalancer": {
                            "enabled": True
                        }
                    },
                    "telemetry": {
                        "enabled": False
                    }
                },
                "scanner": {
                    "analyzer": {
                        "scaling": {
                            "autoScaling": "Enabled"
                        }
                    }
                },
                "scannerV4": {
                    "indexer": {
                        "scaling": {
                            "autoScaling": "Disabled",
                            "minReplicas": 1,
                            "replicas": 1
                        }
                    },
                    "matcher": {
                        "scaling": {
                            "autoScaling": "Disabled",
                            "minReplicas": 1,
                            "replicas": 1
                        }
                    }
                }
            },
        }

        # f"Creating Central Custom Resource in namespace: {self.central_namespace_operator}",
        subprocess.run(
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(central_cr),
            check=True,
            capture_output=True,
            text=True,
        )
        self.wait_for_ready_deployment(self.central_namespace_operator, "central")
        self.wait_for_central_endpoint(self.central_namespace_operator)
        # Fetch Central CA certificate and persist to temp file
        try:
            self.fetch_central_ca_cert(self.central_namespace_operator)
        except Exception as e:
            self.logger.print_with_timestamp(f"Warning: failed to fetch central CA: {e}", style="bold yellow")

    def create_secured_cluster_cr(self, cluster_name="sensor"):
        """Create SecuredCluster Custom Resource for operator deployment"""
        if not cluster_name:
            cluster_name = f"sensor-{random.randint(1000, 9999)}"  # noqa: S311
        self.cluster_name = cluster_name
        crs_content = self.generate_crs(cluster_name)
        self.apply_yaml_to_namespace(self.secured_cluster_namespace_operator, crs_content)

        # Determine central endpoint
        if not self.central_endpoint:
            # Try to get Central endpoint from service/LoadBalancer
            self.central_endpoint = self.get_central_endpoint(self.central_namespace_operator)

        # Create SecuredCluster CR specification
        secured_cluster_cr = {
            "apiVersion": "platform.stackrox.io/v1alpha1",
            "kind": "SecuredCluster",
            "metadata": {
                "name": "stackrox-secured-cluster-services",
                "namespace": self.secured_cluster_namespace_operator,
                "labels": {"app": "stackrox-secured-cluster"},
            },
            "spec": {
                "clusterName": self.cluster_name,
                "centralEndpoint": self.central_endpoint,
                "imagePullSecrets": [{"name": "stackrox"}],
            },
        }

        subprocess.run(
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(secured_cluster_cr),
            check=True,
            capture_output=True,
            text=True,
        )

        self.wait_for_ready_deployment(self.central_namespace_operator, "central")
        self.logger.print_with_timestamp("📋 SecuredCluster CR details:", style="bold cyan")
        self.logger.print_with_timestamp(f"  • Cluster Name: {cluster_name}", style="dim cyan")
        self.logger.print_with_timestamp(f"  • Central Endpoint: {self.central_endpoint}", style="dim cyan")

        self.show_secured_cluster_success_panel()

    def show_central_success_panel(self):
        """Show success panel and write environment file for operator central deployment"""
        success_panel = Panel.fit(
            f"[bold green]✓ Central Deployment Complete[/bold green]\n\n"
            f"[bold]API Endpoint:     [/bold] {self.central_endpoint}\n"
            f"[bold]Admin Password:   [/bold] {self.central_password}\n"
            f"[bold]Namespace:        [/bold] {self.central_namespace_operator}\n"
            f"[bold]Deployment Mode:  [/bold] Operator\n"
            f"[bold]Log File:         [/bold] {self.log_file}",
            title="[bold green]Central Deployment Success[/bold green]",
            border_style="green",
        )
        self.console.print(success_panel)

        env_content = f"""
export API_ENDPOINT="{self.central_endpoint}"
export ROX_ADMIN_PASSWORD="{self.central_password}"
"""

        with open(self.central_env_file, "w") as f:
            f.write(env_content)

    # Inline operator-specific teardown methods
    def teardown_operator_custom_resources(self, component: str):
        self.logger.print_with_timestamp("🗑️  Tearing down operator-managed resources", style="bold cyan")
        if component == "central":
            self.teardown_central_operator_resources()
        elif component == "secured-cluster":
            self.teardown_secured_cluster_operator_resources()
        elif component == "both":
            self.teardown_secured_cluster_operator_resources()
            self.teardown_central_operator_resources()
        else:
            raise RoxieError(f"Unknown component for operator teardown: {component}")

    def teardown_central_operator_resources(self):
        namespace = self.central_namespace_operator
        self.logger.print_with_timestamp(
            f"🗑️  Tearing down Central operator resources in: {namespace}", style="bold cyan"
        )

        result = subprocess.run([self.kubectl, "get", "namespace", namespace], capture_output=True, text=True)
        if result.returncode != 0:
            self.logger.print_with_timestamp(
                f"Namespace '{namespace}' does not exist - nothing to teardown", style="dim yellow"
            )
            return

        teardown_steps: List[Dict[str, Any]] = [
            {
                "description": f"Deleting Central CR in {namespace}",
                "command": [
                    self.kubectl,
                    "delete",
                    "central",
                    "--all",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                    "--timeout=300s",
                ],
            },
            {"description": f"Waiting for operator cleanup in {namespace}", "wait_for_empty_namespace": True},
            {
                "description": f"Force deleting remaining resources in {namespace}",
                "command": [
                    self.kubectl,
                    "delete",
                    "all",
                    "--all",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                    "--timeout=120s",
                ],
            },
            {
                "description": f"Deleting secrets and configmaps in {namespace}",
                "command": [
                    self.kubectl,
                    "delete",
                    "secrets,configmaps",
                    "--all",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                ],
            },
            {
                "description": f"Deleting namespace: {namespace}",
                "command": [self.kubectl, "delete", "namespace", namespace, "--ignore-not-found=true"],
            },
        ]
        self.execute_teardown_steps(teardown_steps, namespace)

    def teardown_secured_cluster_operator_resources(self):
        namespace = self.secured_cluster_namespace_operator
        self.logger.print_with_timestamp(
            f"🗑️  Tearing down SecuredCluster operator resources in: {namespace}", style="bold cyan"
        )

        result = subprocess.run([self.kubectl, "get", "namespace", namespace], capture_output=True, text=True)
        if result.returncode != 0:
            self.logger.print_with_timestamp(
                f"Namespace '{namespace}' does not exist - nothing to teardown", style="dim yellow"
            )

        teardown_steps: List[Dict[str, Any]] = [
            {
                "description": f"Deleting SecuredCluster CR in {namespace}",
                "command": [
                    self.kubectl,
                    "delete",
                    "securedcluster",
                    "--all",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                    "--timeout=300s",
                ],
            },
            {"description": f"Waiting for operator cleanup in {namespace}", "wait_for_empty_namespace": True},
            {
                "description": f"Force deleting remaining resources in {namespace}",
                "command": [
                    self.kubectl,
                    "delete",
                    "all",
                    "--all",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                    "--timeout=120s",
                ],
            },
            {
                "description": f"Deleting secrets and configmaps in {namespace}",
                "command": [
                    self.kubectl,
                    "delete",
                    "secrets,configmaps",
                    "--all",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                ],
            },
            {
                "description": f"Deleting namespace: {namespace}",
                "command": [self.kubectl, "delete", "namespace", namespace, "--ignore-not-found=true"],
            },
        ]
        self.execute_teardown_steps(teardown_steps, namespace)

    def has_operator_deployment(self, component: str) -> bool:
        """Check if operator deployment exists for the given component"""
        try:
            if component == "central":
                namespace = self.central_namespace_operator
                cr_type = "central"
            elif component == "secured-cluster":
                namespace = self.secured_cluster_namespace_operator
                cr_type = "securedcluster"
            elif component in ["all", "both"]:
                # Check both
                return self.has_operator_deployment("central") or self.has_operator_deployment("secured-cluster")
            else:
                return False

            # Check if namespace exists
            result = subprocess.run([self.kubectl, "get", "namespace", namespace], capture_output=True, text=True)

            if result.returncode != 0:
                return False

            # Check if CR exists
            result = subprocess.run([self.kubectl, "get", cr_type, "-n", namespace], capture_output=True, text=True)

            return result.returncode == 0

        except Exception as e:
            self.logger.error(f"Failed to check operator deployment: {str(e)}")
            return False

    def download_and_extract_operator_bundle(self, bundle_image: str) -> str:
        """Download and extract operator bundle image to temporary directory"""
        try:
            # Create temporary directory for bundle extraction
            import tempfile

            bundle_dir = tempfile.mkdtemp(prefix="stackrox-operator-bundle-")
            self.logger.print_with_timestamp(f"Created temporary directory: {bundle_dir}", style="dim blue")

            # Download and extract the bundle image using podman
            container_tool = helpers.get_container_tool()  # podman-only

            self.logger.print_with_timestamp(f"Using {container_tool} to extract bundle", style="dim blue")

            # Pull the bundle image (silent on success)
            try:
                subprocess.run(
                    [container_tool, "pull", bundle_image],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.PIPE,
                    text=True,
                    check=True,
                )
            except subprocess.CalledProcessError as e:
                detail = (e.stderr or "").strip()
                raise RuntimeError(f"Failed to pull bundle image: {bundle_image}: {detail}") from e

            # Extract bundle contents using container copy
            # Create a temporary container and copy files out
            container_id = f"stackrox-bundle-extract-{int(time.time())}"

            try:
                # Create container
                subprocess.run(
                    [container_tool, "create", "--name", container_id, bundle_image],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.PIPE,
                    text=True,
                    check=True,
                )

                # Copy manifests directory from container to host
                subprocess.run(
                    [container_tool, "cp", f"{container_id}:/manifests/.", bundle_dir],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.PIPE,
                    text=True,
                    check=True,
                )

            finally:
                # Clean up container
                try:
                    subprocess.run(
                        [container_tool, "rm", container_id],
                        stdout=subprocess.DEVNULL,
                        stderr=subprocess.DEVNULL,
                        check=False,
                    )
                except Exception:  # noqa: S110
                    pass

            self.logger.print_with_timestamp(f"✓ Bundle extracted to: {bundle_dir}", style="bold green")
            return bundle_dir

        except Exception as e:
            self.logger.error(f"Failed to download/extract operator bundle: {str(e)}")
            raise

    def list_operator_bundle_contents(self, bundle_dir: str) -> None:
        """List and display contents of the operator bundle directory"""
        try:
            self.logger.print_with_timestamp("📋 Operator bundle contents:", style="bold cyan")

            # Walk through the bundle directory and list all files

            for root, _dirs, files in os.walk(bundle_dir):
                # Get relative path from bundle_dir
                rel_path = os.path.relpath(root, bundle_dir)
                if rel_path == ".":
                    rel_path = ""

                # Print directory structure
                if rel_path:
                    self.logger.print_with_timestamp(f"📁 {rel_path}/", style="bold yellow")

                # Print files in this directory
                for file in sorted(files):
                    file_path = os.path.join(rel_path, file) if rel_path else file
                    file_size = os.path.getsize(os.path.join(root, file))
                    self.logger.print_with_timestamp(f"  📄 {file_path} ({file_size} bytes)", style="dim green")

        except Exception as e:
            self.logger.error(f"Failed to list bundle contents: {str(e)}")

    def ensure_crds_installed(self) -> None:
        """Ensure required CRDs exist and are established before creating CRs."""
        required_crds = [
            "centrals.platform.stackrox.io",
            "securedclusters.platform.stackrox.io",
            "securitypolicies.config.stackrox.io",
        ]

        missing: List[str] = []
        for crd in required_crds:
            result = subprocess.run([self.kubectl, "get", "crd", crd], capture_output=True, text=True)
            if result.returncode != 0:
                missing.append(crd)

        if missing:
            # Need to fetch bundle and apply CRDs
            operator_tag_for_image = self.operator_tag
            bundle_image = f"quay.io/rhacs-eng/stackrox-operator-bundle:{operator_tag_for_image}"
            self.logger.print_with_timestamp(
                f"Missing CRDs detected ({', '.join(missing)}); fetching bundle {bundle_image}", style="bold yellow"
            )
            bundle_dir = self.download_and_extract_operator_bundle(bundle_image)
            crd_files = self.identify_crd_files(bundle_dir)
            self.apply_crds_to_cluster(crd_files)
        else:
            # CRDs exist; proceed
            pass

    def identify_crd_files(self, bundle_dir: str) -> List[str]:
        """Identify CRD files in the operator bundle directory"""
        crd_files = []

        try:
            # Walk through the bundle directory to find CRD files
            for root, _dirs, files in os.walk(bundle_dir):
                for file in files:
                    # CRD files typically end with .yaml and contain customresourcedefinition patterns
                    # or have specific CRD-related naming patterns
                    if file.endswith(".yaml") or file.endswith(".yml"):
                        file_path = os.path.join(root, file)

                        # Check if this is likely a CRD file based on filename patterns
                        # Common patterns: *_customresourcedefinition.yaml, *crd*.yaml, or files containing CRD-like names
                        is_crd_candidate = (
                            # Look for typical CRD filename patterns
                            "customresourcedefinition" in file.lower()
                            or "crd" in file.lower()
                            or
                            # Based on previous bundle listing, files ending with platform/config domain names
                            any(domain in file for domain in ["platform.stackrox.io", "config.stackrox.io"])
                            and "clusterserviceversion" not in file.lower()  # Exclude CSV files
                        )

                        if is_crd_candidate:
                            # Double-check by looking at file content for 'kind: CustomResourceDefinition'
                            try:
                                with open(file_path) as f:
                                    content = f.read()
                                    if "kind: CustomResourceDefinition" in content:
                                        crd_files.append(file_path)
                            except Exception:  # noqa: S110
                                # If we can't read the file, skip it
                                pass

        except Exception as e:
            self.logger.error(f"Failed to identify CRD files: {str(e)}")
            raise

        return crd_files

    def parse_csv_deployment_spec(self, csv_file: str) -> Dict[str, Any]:
        """Parse ClusterServiceVersion to extract deployment specifications"""
        try:
            with open(csv_file) as f:
                csv_content = yaml.safe_load(f)

            # Extract key information from CSV
            spec = csv_content.get("spec", {})
            install_spec = spec.get("install", {}).get("spec", {})

            # Extract deployment information
            deployments = install_spec.get("deployments", [])
            cluster_permissions = install_spec.get("clusterPermissions", [])

            # Extract metadata
            metadata = csv_content.get("metadata", {})

            deployment_spec = {
                "name": metadata.get("name", "rhacs-operator"),
                "version": metadata.get("annotations", {}).get("createdAt", "unknown"),
                "container_image": metadata.get("annotations", {}).get("containerImage", ""),
                "deployments": deployments,
                "cluster_permissions": cluster_permissions,
                "service_account": None,
            }

            # Extract service account name from cluster permissions
            if cluster_permissions:
                deployment_spec["service_account"] = cluster_permissions[0].get(
                    "serviceAccountName", "rhacs-operator-controller-manager"
                )

            return deployment_spec

        except Exception as e:
            self.logger.error(f"Failed to parse CSV: {str(e)}")
            raise

    def create_operator_namespace(self, namespace: str = "rhacs-operator-system"):
        """Create namespace for operator deployment"""
        try:
            namespace_yaml = f"""apiVersion: v1
kind: Namespace
metadata:
  name: {namespace}
  labels:
    name: {namespace}
"""

            # Write namespace manifest to temp file
            import tempfile

            with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
                f.write(namespace_yaml)
                temp_file = f.name

            try:
                subprocess.run([self.kubectl, "apply", "-f", temp_file], capture_output=True, text=True, check=True)
            finally:
                os.unlink(temp_file)

        except Exception as e:
            self.logger.error(f"Failed to create operator namespace: {str(e)}")
            raise

    def create_service_account(self, namespace: str, service_account_name: str):
        """Create ServiceAccount for operator"""
        try:
            sa_yaml = {
                "apiVersion": "v1",
                "kind": "ServiceAccount",
                "metadata": {"name": service_account_name, "namespace": namespace, "labels": {"app": "rhacs-operator"}},
            }

            subprocess.run(
                [self.kubectl, "apply", "-f", "-"], input=yaml.dump(sa_yaml), capture_output=True, text=True, check=True
            )

        except Exception as e:
            raise RoxieError("Failed to create ServiceAccount") from e

    def create_cluster_role_from_csv(self, deployment_spec: dict):
        """Create ClusterRole from CSV cluster permissions"""
        cluster_permissions = deployment_spec.get("cluster_permissions", [])
        if not cluster_permissions:
            self.logger.print_with_timestamp("No cluster permissions found in CSV", style="bold yellow")
            return

        # Extract rules from first cluster permission (typically the main one)
        rules = cluster_permissions[0].get("rules", [])

        cluster_role_yaml = {
            "apiVersion": "rbac.authorization.k8s.io/v1",
            "kind": "ClusterRole",
            "metadata": {"name": "rhacs-operator-manager-role", "labels": {"app": "rhacs-operator"}},
            "rules": rules,
        }

        subprocess.run(
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(cluster_role_yaml),
            capture_output=True,
            text=True,
            check=True,
        )

    def create_cluster_role_binding(self, namespace: str, service_account_name: str):
        """Create ClusterRoleBinding to link ServiceAccount to ClusterRole"""
        crb_yaml = {
            "apiVersion": "rbac.authorization.k8s.io/v1",
            "kind": "ClusterRoleBinding",
            "metadata": {"name": "rhacs-operator-manager-rolebinding", "labels": {"app": "rhacs-operator"}},
            "roleRef": {
                "apiGroup": "rbac.authorization.k8s.io",
                "kind": "ClusterRole",
                "name": "rhacs-operator-manager-role",
            },
            "subjects": [{"kind": "ServiceAccount", "name": service_account_name, "namespace": namespace}],
        }
        subprocess.run(
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(crb_yaml),
            capture_output=True,
            text=True,
            check=True,
        )

    def create_deployment_from_csv(self, namespace: str, deployment_spec: dict):
        """Create Deployment from CSV deployment specification"""
        deployments = deployment_spec.get("deployments", [])

        # Use the first deployment (typically the main operator deployment)
        csv_deployment = deployments[0]
        deployment_name = csv_deployment.get("name", "rhacs-operator-controller-manager")

        # Extract deployment spec and modify for our namespace
        deployment_template = csv_deployment.get("spec", {})

        # Create Kubernetes Deployment manifest
        deployment_yaml = {
            "apiVersion": "apps/v1",
            "kind": "Deployment",
            "metadata": {
                "name": deployment_name,
                "namespace": namespace,
                "labels": csv_deployment.get("label", {}),
            },
            "spec": deployment_template,
        }

        # Ensure the service account is set in the pod template
        if "template" in deployment_yaml["spec"]:
            if "spec" not in deployment_yaml["spec"]["template"]:
                deployment_yaml["spec"]["template"]["spec"] = {}
            deployment_yaml["spec"]["template"]["spec"]["serviceAccountName"] = deployment_spec.get(
                "service_account", "rhacs-operator-controller-manager"
            )

        subprocess.run(
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(deployment_yaml),
            capture_output=True,
            text=True,
            check=True,
        )

    def apply_bundle_service_resources(self, bundle_dir: str, namespace: str):
        """Apply Service and ClusterRole resources from bundle to the operator namespace"""
        success_count = 0
        total_count = 0

        # Apply the Service resource
        service_file = os.path.join(bundle_dir, "rhacs-operator-controller-manager-metrics-service_v1_service.yaml")
        if os.path.exists(service_file):
            total_count += 1
            # Need to patch the service to add namespace
            import tempfile

            with open(service_file) as f:
                service_content = yaml.safe_load(f)

            # Add namespace to metadata
            if "metadata" not in service_content:
                service_content["metadata"] = {}
            service_content["metadata"]["namespace"] = namespace

            with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml", delete=False) as f:
                yaml.dump(service_content, f, default_flow_style=False)
                temp_file = f.name

            try:
                subprocess.run([self.kubectl, "apply", "-f", temp_file], capture_output=True, text=True, check=True)
                success_count += 1
            finally:
                os.unlink(temp_file)

        # Apply the ClusterRole resource (metrics reader)
        clusterrole_file = os.path.join(
            bundle_dir, "rhacs-operator-metrics-reader_rbac.authorization.k8s.io_v1_clusterrole.yaml"
        )
        if os.path.exists(clusterrole_file):
            total_count += 1
            subprocess.run([self.kubectl, "apply", "-f", clusterrole_file], capture_output=True, text=True, check=True)
            success_count += 1

        self.logger.print_with_timestamp(
            f"✓ Applied {success_count}/{total_count} bundle service resources", style="bold green"
        )

    def wait_for_operator_ready(
        self, namespace: str, deployment_name: str = "rhacs-operator-controller-manager", timeout: int = 300
    ):
        """Wait for operator deployment to become ready"""
        try:
            self.logger.print_with_timestamp("⏳ Waiting for operator deployment to become ready...", style="bold cyan")

            start_time = time.time()
            while time.time() - start_time < timeout:
                # Check deployment status
                result = subprocess.run(
                    [
                        self.kubectl,
                        "get",
                        "deployment",
                        deployment_name,
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
                        self.logger.print_with_timestamp(
                            f"✓ Operator deployment is ready ({ready_replicas} replicas)", style="bold green"
                        )
                        return

                # Wait a bit before checking again
                time.sleep(1)

            self.logger.error(f"Timeout waiting for operator deployment to become ready after {timeout}s")
            raise RoxieError("Operator did not become ready")

        except Exception as e:
            self.logger.error(f"Failed to wait for operator readiness: {str(e)}")
            raise

    def is_operator_deployed_and_ready(
        self, namespace: str = "rhacs-operator-system", deployment_name: str = "rhacs-operator-controller-manager"
    ) -> bool:
        """Check if the operator is already deployed and ready"""
        try:
            # Check if namespace exists
            result = subprocess.run([self.kubectl, "get", "namespace", namespace], capture_output=True, text=True)

            if result.returncode != 0:
                self.logger.print_with_timestamp(f"Operator namespace '{namespace}' does not exist", style="dim cyan")
                return False

            # Check if deployment exists and is ready
            result = subprocess.run(
                [
                    self.kubectl,
                    "get",
                    "deployment",
                    deployment_name,
                    "-n",
                    namespace,
                    "-o",
                    "jsonpath={.status.readyReplicas}",
                ],
                capture_output=True,
                text=True,
            )

            if result.returncode != 0:
                self.logger.print_with_timestamp(
                    f"Operator deployment '{deployment_name}' does not exist in namespace '{namespace}'",
                    style="dim cyan",
                )
                return False

            ready_replicas = result.stdout.strip()
            if ready_replicas and ready_replicas != "0":
                self.logger.print_with_timestamp(
                    f"✓ Operator is already deployed and ready ({ready_replicas} replicas)", style="bold green"
                )
                return True
            else:
                self.logger.print_with_timestamp(
                    f"Operator deployment exists but is not ready (ready replicas: {ready_replicas})",
                    style="dim yellow",
                )
                return False

        except Exception as e:
            self.logger.error(f"Failed to check operator deployment status: {str(e)}")
            return False

    def get_current_operator_version(
        self, namespace: str = "rhacs-operator-system", deployment_name: str = "rhacs-operator-controller-manager"
    ) -> str:
        """Get the version of the currently deployed operator"""
        try:
            # Get the container image from the deployment
            result = subprocess.run(
                [
                    self.kubectl,
                    "get",
                    "deployment",
                    deployment_name,
                    "-n",
                    namespace,
                    "-o",
                    "jsonpath={.spec.template.spec.containers[0].image}",
                ],
                capture_output=True,
                text=True,
            )

            if result.returncode != 0:
                return ""

            image = result.stdout.strip()
            if ":" in image:
                # Extract tag from image (e.g., "quay.io/rhacs-eng/stackrox-operator:4.9.0-441-g7754d5a916")
                current_tag = image.split(":")[-1]
                return current_tag

            return ""

        except Exception as e:
            self.logger.error(f"Failed to get current operator version: {str(e)}")
            raise

    def has_operator_version_mismatch(
        self, desired_operator_tag: str, namespace: str = "rhacs-operator-system"
    ) -> bool:
        """Check if there's a version mismatch between desired and current operator"""
        try:
            current_version = self.get_current_operator_version(namespace)

            if not current_version:
                self.logger.print_with_timestamp("Could not determine current operator version", style="dim yellow")
                return True  # Assume mismatch if we can't determine current version

            # Clean the operator tags for comparison (remove 'v' prefix if present)
            desired_clean = desired_operator_tag.lstrip("v")
            current_clean = current_version.lstrip("v")

            if desired_clean != current_clean:
                self.logger.print_with_timestamp("🔄 Version mismatch detected:", style="bold yellow")
                self.logger.print_with_timestamp(f"  • Current: {current_version}", style="dim yellow")
                self.logger.print_with_timestamp(f"  • Desired: {desired_clean}", style="dim yellow")
                return True
            else:
                self.logger.print_with_timestamp(f"✓ Version match: {current_version}", style="bold green")
                return False

        except Exception as e:
            self.logger.error(f"Failed to check operator version mismatch: {str(e)}")
            raise

    def teardown_rhacs_operator(self, namespace: str = "rhacs-operator-system"):
        """Teardown the operator deployment including CRDs"""
        self.logger.print_with_timestamp("🗑️  Tearing down operator deployment for version update...", style="bold cyan")

        # List of resources to clean up (in order)
        cleanup_steps = [
            {
                "description": "Removing operator deployment",
                "command": [
                    self.kubectl,
                    "delete",
                    "deployment",
                    "rhacs-operator-controller-manager",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                    "--wait=false",
                ],
            },
            {
                "description": "Removing operator service",
                "command": [
                    self.kubectl,
                    "delete",
                    "service",
                    "rhacs-operator-controller-manager-metrics-service",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                    "--wait=false",
                ],
            },
            {
                "description": "Removing ClusterRoleBinding",
                "command": [
                    self.kubectl,
                    "delete",
                    "clusterrolebinding",
                    "rhacs-operator-manager-rolebinding",
                    "--ignore-not-found=true",
                ],
            },
            {
                "description": "Removing ClusterRole",
                "command": [
                    self.kubectl,
                    "delete",
                    "clusterrole",
                    "rhacs-operator-manager-role",
                    "--ignore-not-found=true",
                ],
            },
            {
                "description": "Removing metrics ClusterRole",
                "command": [
                    self.kubectl,
                    "delete",
                    "clusterrole",
                    "rhacs-operator-metrics-reader",
                    "--ignore-not-found=true",
                ],
            },
            {
                "description": "Removing ServiceAccount",
                "command": [
                    self.kubectl,
                    "delete",
                    "serviceaccount",
                    "rhacs-operator-controller-manager",
                    "-n",
                    namespace,
                    "--ignore-not-found=true",
                    "--wait=false",
                ],
            },
            {
                "description": "Removing operator namespace",
                "command": [
                    self.kubectl,
                    "delete",
                    "namespace",
                    namespace,
                    "--ignore-not-found=true",
                    "--wait=false",
                ],
            },
            {
                "description": "Removing CRDs",
                "command": [
                    self.kubectl,
                    "delete",
                    "crd",
                    "centrals.platform.stackrox.io",
                    "securedclusters.platform.stackrox.io",
                    "securitypolicies.config.stackrox.io",
                    "--ignore-not-found=true",
                    "--timeout=60s",
                ],
            },
        ]

        # Execute cleanup steps
        success_count = 0
        for step in cleanup_steps:
            try:
                self.logger.print_with_timestamp(f"➡️  {step['description']}", style="dim cyan")
                subprocess.run(
                    step["command"],
                    text=True,
                    check=True,
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.PIPE,
                )
                success_count += 1
            except subprocess.CalledProcessError as e:
                # Log but continue with other cleanup steps
                self.logger.print_with_timestamp(
                    f"⚠️  Failed: {step['description']} (continuing...) - {(e.stderr or str(e)).strip()}", style="dim yellow"
                )
                # continue to next step
                continue

        # Wait for namespace deletion
        self.logger.print_with_timestamp("⏳ Waiting for operator namespace to be fully deleted...", style="bold cyan")

        timeout = 120  # 2 minutes timeout
        start_time = time.time()

        while time.time() - start_time < timeout:
            result = subprocess.run([self.kubectl, "get", "namespace", namespace], capture_output=True, text=True)
            if result.returncode != 0:
                self.logger.print_with_timestamp("✓ Operator teardown completed", style="bold green")
                return
            time.sleep(2)

        self.logger.print_with_timestamp(
            "⚠️  Timeout waiting for namespace deletion, proceeding anyway...", style="bold yellow"
        )

    def deploy_operator_from_csv(self, bundle_dir: Optional[str] = None):
        """Deploy the operator using CSV extraction and conversion"""

        namespace = "rhacs-operator-system"
        deployment_name = "rhacs-operator-controller-manager"

        # If bundle_dir is None, it means version check was already done and operator is up-to-date
        if bundle_dir is None:
            self.logger.print_with_timestamp(
                "Skipping operator deployment - correct version already running", style="bold yellow"
            )
            return

        # Check if operator is already deployed and ready (this handles version mismatch cases)
        if self.is_operator_deployed_and_ready(namespace, deployment_name):
            # Operator exists - this should only happen for version mismatches since we checked earlier
            self.logger.print_with_timestamp("Redeploying operator due to version mismatch...", style="bold yellow")

            # Get the operator tag for version comparison (sanitized for image reference)
            operator_tag_for_image = self.operator_tag

            if not self.has_operator_version_mismatch(operator_tag_for_image, namespace):
                # This shouldn't happen since we checked earlier, but handle gracefully
                self.logger.print_with_timestamp(
                    "️Operator deployment not needed - correct version running", style="bold yellow"
                )
                return

        csv_file = os.path.join(bundle_dir, "rhacs-operator.clusterserviceversion.yaml")
        if not os.path.exists(csv_file):
            self.logger.error("ClusterServiceVersion file not found in bundle")
            return False

        self.logger.print_with_timestamp("🔍 Parsing ClusterServiceVersion deployment specification", style="bold cyan")
        deployment_spec = self.parse_csv_deployment_spec(csv_file)
        if not deployment_spec:
            return False

        service_account_name = deployment_spec.get("service_account", "rhacs-operator-controller-manager")

        self.logger.print_with_timestamp("📋 Operator deployment plan:", style="bold cyan")
        self.logger.print_with_timestamp(f"  • Namespace: {namespace}", style="dim cyan")
        self.logger.print_with_timestamp(f"  • ServiceAccount: {service_account_name}", style="dim cyan")
        self.logger.print_with_timestamp(
            f"  • Container Image: {deployment_spec.get('container_image', 'unknown')}", style="dim cyan"
        )
        self.logger.print_with_timestamp(
            f"  • Deployments: {len(deployment_spec.get('deployments', []))}", style="dim cyan"
        )
        self.logger.print_with_timestamp(
            f"  • Cluster Permissions: {len(deployment_spec.get('cluster_permissions', []))}", style="dim cyan"
        )

        self.create_operator_namespace(namespace)
        self.create_service_account(namespace, service_account_name)
        self.create_cluster_role_from_csv(deployment_spec)
        self.create_cluster_role_binding(namespace, service_account_name)
        self.create_deployment_from_csv(namespace, deployment_spec)
        self.apply_bundle_service_resources(bundle_dir, namespace)
        self.wait_for_operator_ready(namespace)
        self.logger.print_with_timestamp("🎉 Operator deployment completed successfully!", style="bold green")


    def wait_for_operator_cleanup(self, namespace: str, timeout: int = 180):
        """Wait for operator to clean up managed resources in namespace"""

        with self.create_progress_with_timestamp(include_bar=False, transient=True) as progress:
            task_id = progress.add_task(f"Waiting for operator cleanup in [bold]{namespace}[/bold]", total=None)

            start_time = time.time()
            while time.time() - start_time < timeout:
                # Check for remaining pods (excluding completed jobs)
                result = subprocess.run(
                    [
                        self.kubectl,
                        "get",
                        "pods",
                        "-n",
                        namespace,
                        "--field-selector=status.phase!=Succeeded",
                        "-o",
                        "name",
                    ],
                    capture_output=True,
                    text=True,
                )

                if result.returncode == 0:
                    pods = [p for p in result.stdout.strip().split("\n") if p.strip()]
                    if not pods or (len(pods) == 1 and not pods[0].strip()):
                        progress.stop()
                        self.logger.print_with_timestamp(
                            f"✓ Operator cleanup completed in {namespace}", style="bold green"
                        )
                        return True

                progress.update(task_id, advance=1)
                time.sleep(1)

        self.logger.print_with_timestamp(f"⚠️  Timeout waiting for operator cleanup in {namespace}", style="bold yellow")

    def upgrade_operator(self):
        """Upgrade the operator to the latest version"""
        self.logger.print_with_timestamp("🔄 Upgrading operator to the latest version...", style="bold cyan")
        self.deploy_rhacs_operator()

    def deploy_operator(self):
        """Deploy the ACS operator"""
        self.logger.print_with_timestamp("🔄 Deploying ACS operator...", style="bold cyan")
        self.deploy_rhacs_operator()

    # def check_and_cleanup_existing_operator_deployments(self, component: str) -> bool:
    #     """Check for existing operator deployments and clean them up if found"""
    #     try:
    #         # Define operator namespaces to check
    #         central_op_namespace = self.central_namespace_operator
    #         secured_cluster_op_namespace = self.secured_cluster_namespace_operator

    #         # Check which namespaces exist
    #         existing_namespaces = []

    #         # Check central operator namespace
    #         central_exists = False
    #         result = subprocess.run(
    #             [self.kubectl, "get", "namespace", central_op_namespace], capture_output=True, text=True
    #         )
    #         if result.returncode == 0:
    #             existing_namespaces.append(central_op_namespace)
    #             central_exists = True

    #         # Check secured cluster operator namespace
    #         result = subprocess.run(
    #             [self.kubectl, "get", "namespace", secured_cluster_op_namespace], capture_output=True, text=True
    #         )
    #         if result.returncode == 0:
    #             existing_namespaces.append(secured_cluster_op_namespace)

    #         # If stackrox-central-op exists, always clean up both namespaces
    #         # This matches the user requirement and ensures clean state
    #         if central_exists or existing_namespaces:
    #             self.logger.print_with_timestamp(
    #                 f"Found existing operator deployments in: {', '.join(existing_namespaces)}", style="bold yellow"
    #             )
    #             self.logger.print_with_timestamp("Initiating cleanup of existing operator deployments...", style="bold yellow")

    #             # Always clean up both central and secured cluster operator namespaces
    #             # This ensures a clean slate for the new deployment, especially since
    #             # secured-cluster depends on central and we want consistent state
    #             try:
    #                 self.teardown_operator_custom_resources("both")
    #                 return True
    #             except Exception as e:
    #                 self.logger.error(f"Failed to cleanup existing operator deployments: {str(e)}")
    #                 # Continue with deployment anyway, but warn user
    #                 self.logger.print_with_timestamp(
    #                     "Continuing with deployment despite cleanup failure...", style="bold yellow"
    #                 )
    #                 return False

    #         return False

    #     except Exception as e:
    #         self.logger.error(f"Error checking for existing operator deployments: {str(e)}")
    #         return False

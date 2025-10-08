import os
import random
import subprocess
import time
from typing import Any, cast

import yaml

import helpers
from deployer import ACSDeployer
from errors import RoxieError
from helpers import run_command


class ACSDeployerOperator(ACSDeployer):
    """Operator-specific deployer that implements Operator deployment/teardown."""

    def apply_crds_to_cluster(self, crd_files: list[str]):
        """Apply CRD files to the cluster using kubectl"""
        self.logger.print_with_timestamp(f"Applying {len(crd_files)} CRD(s) to cluster", style="bold cyan")

        for crd_file in crd_files:
            run_command(
                "Applying CRD to cluster",
                [self.kubectl, "apply", "-f", crd_file],
                capture_output=True,
                text=True,
                check=True,
            )
            crd_basename = os.path.basename(crd_file)
            self.logger.print_with_timestamp(f"✓ Successfully applied CRD {crd_basename}", style="bold green")

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

    def deploy(self, component: str, resources: str, exposure: str):
        """Deploy specified component(s) using operator."""
        self.logger.print_with_timestamp("Initiating operator-based deployment of ACS", style="bold cyan")

        operator_exists_and_ready = self.is_operator_deployed_and_ready()
        if operator_exists_and_ready:
            if self.has_operator_version_mismatch(self.operator_tag):
                # First remove any deployed components so CRDs can be safely removed afterward
                self.logger.print_with_timestamp(
                    "Version mismatch detected: tearing down Central and SecuredCluster before operator upgrade",
                    style="bold yellow",
                )
                try:
                    self.teardown("both")
                except Exception as e:
                    self.logger.print_with_timestamp(
                        f"Warning: teardown of existing components encountered issues: {e}", style="bold yellow"
                    )

                # Then remove the operator (including CRDs)
                self.teardown_rhacs_operator()
                self.logger.print_with_timestamp("Cleaned up existing operator deployment", style="bold yellow")
                self.deploy_rhacs_operator()
        else:
            # No operator present; just deploy it
            self.deploy_rhacs_operator()

        # Persist exposure, apply convenience defaults (e.g., kind)
        resources, exposure = self.apply_convenience_defaults(resources, exposure)
        self.exposure = exposure
        self.deploy_component(component, resources, exposure)
        self.logger.print_with_timestamp("✓ Operator-based deployment completed successfully!", style="bold green")

    def deploy_central(self, resources: str, exposure: str):
        self.logger.print_with_timestamp(f"Deploying Central with resources: {resources}", style="bold cyan")
        self.teardown()
        self.ensure_namespace_exists(self.central_namespace)
        self.prepare_namespace(self.central_namespace)
        self.ensure_crds_installed()
        cr = self.create_central_cr(resources, exposure)
        self.apply_central_cr(cr)
        self.show_central_success_panel()

    def deploy_secured_cluster(self, resources: str):
        self.logger.print_with_timestamp(f"Deploying Secured Cluster with resources: {resources}", style="bold cyan")
        self.teardown("secured-cluster")
        self.ensure_namespace_exists(self.secured_cluster_namespace)
        self.prepare_namespace(self.secured_cluster_namespace)
        cr = self.create_secured_cluster_cr(resources)
        self.apply_secured_cluster_cr(cr)
        self.show_secured_cluster_success_panel()

    def deploy_component(self, component: str, resources: str, exposure: str):
        """Deploy Custom Resource for the specified component"""
        self.logger.print_with_timestamp(f"Deploying Custom Resources for: {component}", style="bold cyan")

        if component == "central":
            self.deploy_central(resources, exposure)
        elif component == "secured-cluster":
            self.deploy_secured_cluster(resources=resources)
        elif component == "both":
            self.deploy_central(resources, exposure)
            self.deploy_secured_cluster(resources=resources)
        self.logger.print_with_timestamp("✓ Custom Resources deployed successfully for", style="bold green")

    def get_central_resources(self, resource_name: str = "default"):
        if resource_name == "small":
            return {
                "spec": {
                    "central": {
                        "resources": {
                            "requests": {"cpu": "500m", "memory": "700Mi"},
                            "limits": {"cpu": "2000m", "memory": "3500Mi"},
                        },
                        "db": {
                            "resources": {
                                "requests": {"cpu": "500m", "memory": "700Mi"},
                                "limits": {"cpu": "2000m", "memory": "3500Mi"},
                            }
                        },
                    },
                    "scanner": {"scannerComponent": "Disabled"},
                    "scannerV4": {
                        "db": {
                            "resources": {
                                "requests": {"cpu": "400m", "memory": "1000Mi"},
                                "limits": {"cpu": "1000m", "memory": "2500Mi"},
                            }
                        },
                        "indexer": {
                            "resources": {
                                "requests": {"cpu": "400m", "memory": "512Mi"},
                                "limits": {"cpu": "2000m", "memory": "4Gi"},
                            }
                        },
                    },
                }
            }
        return {}

    def get_secured_cluster_resources(self, resource_name: str = "default"):
        if resource_name == "small":
            return {
                "spec": {
                    "sensor": {
                        "resources": {
                            "requests": {"cpu": "500m", "memory": "500Mi"},
                            "limits": {"cpu": "1000m", "memory": "2Gi"},
                        }
                    },
                    "scanner": {"scannerComponent": "Disabled"},
                    "scannerV4": {"scannerComponent": "Disabled"},
                }
            }
        return {}

    def get_central_cr_exposure(self, exposure: str) -> dict[str, Any]:
        exposure_cr = {}  # Disabled exposure.
        if exposure == "loadbalancer":
            exposure_cr = {"loadBalancer": {"enabled": True}}
        return {"spec": {"central": {"exposure": exposure_cr}}}

    def create_central_cr(self, resources_name: str, exposure: str) -> dict[str, Any]:
        """Create Central Custom Resource for operator deployment"""
        self.apply_admin_password_secret("admin-password")
        base_central_cr: dict[str, Any] = {
            "apiVersion": "platform.stackrox.io/v1alpha1",
            "kind": "Central",
            "metadata": {
                "name": "stackrox-central-services",
                "namespace": self.central_namespace,
                "labels": {"app": "stackrox-central"},
            },
            "spec": {
                "central": {
                    "adminPasswordSecret": {"name": "admin-password"},
                    "telemetry": {"enabled": False},
                },
                "scanner": {"analyzer": {"scaling": {"autoScaling": "Disabled", "replicas": 1}}},
                "scannerV4": {
                    "indexer": {"scaling": {"autoScaling": "Disabled", "replicas": 1}},
                    "matcher": {"scaling": {"autoScaling": "Disabled", "replicas": 1}},
                },
            },
        }
        resources_cr = self.get_central_resources(resources_name)
        exposure_cr = self.get_central_cr_exposure(exposure)
        central_cr = helpers.merge_dicts(base_central_cr, resources_cr, exposure_cr)
        return central_cr

    def apply_central_cr(self, central_cr: dict[str, Any]):
        run_command(
            "Applying Central CR",
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(central_cr),
            check=True,
            capture_output=True,
            text=True,
        )
        self.wait_for_ready_deployment(self.central_namespace, "central")
        # Establish endpoint depending on exposure
        if self.exposure == "none" and self.port_forwarding_enabled:
            self.start_central_port_forward(self.central_namespace)
        self.wait_for_central_endpoint(self.central_namespace)
        # Fetch Central CA certificate and persist to temp file
        try:
            self.fetch_central_ca_cert(self.central_namespace)
        except Exception as e:
            self.logger.print_with_timestamp(f"Warning: failed to fetch central CA: {e}", style="bold yellow")

    def create_secured_cluster_cr(self, resources_name: str, cluster_name="sensor") -> dict[str, Any]:
        """Create SecuredCluster Custom Resource for operator deployment"""
        if not cluster_name:
            cluster_name = f"sensor-{random.randint(1000, 9999)}"  # noqa: S311
        self.cluster_name = cluster_name
        crs_content = self.generate_crs(cluster_name)
        self.apply_yaml_to_namespace(self.secured_cluster_namespace, crs_content)

        # Determine central endpoint
        if not self.central_endpoint:
            # Try to get Central endpoint from service/LoadBalancer
            self.central_endpoint = self.get_central_endpoint(self.central_namespace)

        base_secured_cluster_cr = {
            "apiVersion": "platform.stackrox.io/v1alpha1",
            "kind": "SecuredCluster",
            "metadata": {
                "name": "stackrox-secured-cluster-services",
                "namespace": self.secured_cluster_namespace,
                "labels": {"app": "stackrox-secured-cluster"},
            },
            "spec": {
                "clusterName": self.cluster_name,
                "centralEndpoint": "https://central.acs-central.svc:443",
                "imagePullSecrets": [{"name": "stackrox"}],
                "admissionControl": {"replicas": 1},
                "scanner": {"analyzer": {"scaling": {"autoScaling": "Enabled", "replicas": 1}}},
                "scannerV4": {
                    "indexer": {"scaling": {"autoScaling": "Disabled", "replicas": 1}},
                },
            },
        }
        resources_cr = self.get_secured_cluster_resources(resources_name)
        secured_cluster_cr = helpers.merge_dicts(base_secured_cluster_cr, resources_cr)
        return secured_cluster_cr

    def apply_secured_cluster_cr(self, secured_cluster_cr: dict[str, Any]):
        run_command(
            "Applying SecuredCluster CR",
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(secured_cluster_cr),
            check=True,
            capture_output=True,
            text=True,
        )
        self.wait_for_ready_deployment(self.secured_cluster_namespace, "sensor")

    def has_operator_deployment(self, component: str) -> bool:
        """Check if operator deployment exists for the given component"""
        try:
            if component == "central":
                namespace = self.central_namespace
                cr_type = "central"
            elif component == "secured-cluster":
                namespace = self.secured_cluster_namespace
                cr_type = "securedcluster"
            elif component in ["all", "both"]:
                # Check both
                return self.has_operator_deployment("central") or self.has_operator_deployment("secured-cluster")
            else:
                return False

            # Check if namespace exists
            result = run_command(
                title=f"Retrieving namespace {namespace}",
                cmd=[self.kubectl, "get", "namespace", namespace],
                capture_output=True,
                text=True,
            )
            if result.returncode != 0:
                return False

            # Check if CR exists
            result = run_command(
                title=f"Retrieving {cr_type} in namespace {namespace}",
                cmd=[self.kubectl, "get", cr_type, "-n", namespace],
                capture_output=True,
                text=True,
            )
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
            run_command(
                "Pulling operator bundle image",
                [container_tool, "pull", bundle_image],
                stdout=subprocess.DEVNULL,
                stderr=subprocess.PIPE,
                text=True,
                check=True,
            )

            # Extract bundle contents using container copy
            # Create a temporary container and copy files out
            container_id = f"stackrox-bundle-extract-{int(time.time())}"

            try:
                run_command(
                    "Creating temporary container for bundle",
                    [container_tool, "create", "--name", container_id, bundle_image],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.PIPE,
                    text=True,
                    check=True,
                )

                run_command(
                    "Retrieving bundle contents",
                    [container_tool, "cp", f"{container_id}:/manifests/.", bundle_dir],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.PIPE,
                    text=True,
                    check=True,
                )

            finally:
                # Clean up container
                run_command(
                    "Removing temporary container",
                    [container_tool, "rm", container_id],
                    stdout=subprocess.DEVNULL,
                    stderr=subprocess.DEVNULL,
                    check=False,
                )

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

        missing: list[str] = []
        for crd in required_crds:
            result = run_command("Retrieving CRD", [self.kubectl, "get", "crd", crd], capture_output=True, text=True)
            if result.returncode != 0:
                missing.append(crd)

        if missing:
            # Need to fetch bundle and apply CRDs
            operator_tag_for_image = self.operator_tag
            bundle_image = f"quay.io/rhacs-eng/stackrox-operator-bundle:{operator_tag_for_image}"
            self.logger.print_with_timestamp(f"Missing CRDs detected ({', '.join(missing)})", style="bold yellow")
            self.logger.print_with_timestamp(f"Fetching bundle {bundle_image}", style="bold yellow")
            bundle_dir = self.download_and_extract_operator_bundle(bundle_image)
            crd_files = self.identify_crd_files(bundle_dir)
            self.apply_crds_to_cluster(crd_files)
        else:
            # CRDs exist; proceed
            pass

    def identify_crd_files(self, bundle_dir: str) -> list[str]:
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

    def parse_csv_deployment_spec(self, csv_file: str) -> dict[str, Any]:
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
                run_command(
                    "Applying namespace",
                    [self.kubectl, "apply", "-f", temp_file],
                    capture_output=True,
                    text=True,
                    check=True,
                )
            finally:
                os.unlink(temp_file)

        except Exception as e:
            self.logger.error(f"Failed to create operator namespace: {str(e)}")
            raise

    def create_service_account(self, namespace: str, service_account_name: str):
        """Create ServiceAccount for operator"""
        sa_yaml = {
            "apiVersion": "v1",
            "kind": "ServiceAccount",
            "metadata": {"name": service_account_name, "namespace": namespace, "labels": {"app": "rhacs-operator"}},
        }

        run_command(
            "Creating ServiceAccount for operator",
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(sa_yaml),
            capture_output=True,
            text=True,
            check=True,
        )

    def create_cluster_role_from_csv(self, deployment_spec: dict[str, Any]):
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

        run_command(
            "Applying ClusterRole",
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
        run_command(
            "Applying ClusterRoleBinding",
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(crb_yaml),
            capture_output=True,
            text=True,
            check=True,
        )

    def create_deployment_from_csv(self, namespace: str, deployment_spec: dict[str, Any]):
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

        run_command(
            "Applying operator deployment",
            [self.kubectl, "apply", "-f", "-"],
            input=yaml.dump(deployment_yaml),
            capture_output=True,
            text=True,
            check=True,
        )

    def apply_bundle_service_resources(self, bundle_dir: str, namespace: str):
        """Apply Service and ClusterRole resources from bundle to the operator namespace"""

        service_file = os.path.join(bundle_dir, "rhacs-operator-controller-manager-metrics-service_v1_service.yaml")
        run_command(
            "Applying Service",
            [self.kubectl, "apply", "-n", namespace, "-f", service_file],
            capture_output=True,
            text=True,
            check=True,
        )

        clusterrole_file = os.path.join(
            bundle_dir, "rhacs-operator-metrics-reader_rbac.authorization.k8s.io_v1_clusterrole.yaml"
        )
        run_command(
            "Applying ClusterRole",
            [self.kubectl, "apply", "-f", clusterrole_file],
            capture_output=True,
            text=True,
            check=True,
        )

        self.logger.print_with_timestamp(
            f"✓ Applied bundle service resources to namespace {namespace}", style="bold green"
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
                result = run_command(
                    "Checking operator deployment status",
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
            result = run_command(
                "Checking namespace", [self.kubectl, "get", "namespace", namespace], capture_output=True, text=True
            )

            if result.returncode != 0:
                self.logger.print_with_timestamp(f"Operator namespace '{namespace}' does not exist", style="dim cyan")
                return False

            # Check if deployment exists and is ready
            result = run_command(
                "Checking operator deployment status",
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
            result = run_command(
                "Getting current operator version",
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

            image = cast(str, result.stdout).strip()
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
            self.logger.print_with_timestamp(f"➡️  {step['description']}", style="dim cyan")
            desc = cast(str, step["description"])
            cmd = cast(list[str], step["command"])
            run_command(
                title=desc,
                cmd=cmd,
                text=True,
                check=True,
                stdout=subprocess.DEVNULL,
                stderr=subprocess.PIPE,
            )
            success_count += 1

        # Wait for namespace deletion
        self.logger.print_with_timestamp("⏳ Waiting for operator namespace to be fully deleted...", style="bold cyan")

        timeout = 120  # 2 minutes timeout
        start_time = time.time()

        while time.time() - start_time < timeout:
            result = run_command(
                "Checking namespace", [self.kubectl, "get", "namespace", namespace], capture_output=True, text=True
            )
            if result.returncode != 0:
                self.logger.print_with_timestamp("✓ Operator teardown completed", style="bold green")
                return
            time.sleep(2)

        self.logger.print_with_timestamp(
            "⚠️  Timeout waiting for namespace deletion, proceeding anyway...", style="bold yellow"
        )

    def deploy_operator_from_csv(self, bundle_dir: str | None = None):
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

    def deploy_operator(self):
        """Deploy the ACS operator"""
        self.logger.print_with_timestamp("🔄 Deploying ACS operator...", style="bold cyan")
        self.deploy_rhacs_operator()

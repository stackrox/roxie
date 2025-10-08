import random
import tempfile
from typing import Any

import yaml
from rich.panel import Panel

import helpers
from deployer import ACSDeployer
from helpers import run_command
from resources_presets import (
    resources_central_db_small,
    resources_central_small,
    resources_scanner_analyzer_small,
    resources_scanner_db_small,
    resources_scanner_v4_db_small,
    resources_scanner_v4_indexer_small,
    resources_scanner_v4_matcher_small,
    resources_sensor_small,
)


class ACSDeployerHelm(ACSDeployer):
    """Helm-specific deployer that implements Helm deployment/teardown."""

    def get_central_resources(self, resources_name: str) -> dict[str, Any]:
        """Return a dict overlay for central resources preset."""
        resources_small = {
            "central": {
                "resources": resources_central_small,
                "telemetry": {"enabled": False},
                "exposure": {"loadBalancer": {"enabled": True}},
                "db": {"resources": resources_central_db_small},
            },
            "scanner": {
                "resources": resources_scanner_analyzer_small,
                "dbResources": resources_scanner_db_small,
                "replicas": 1,
                "autoscaling": {"disable": True},
            },
            "scannerV4": {
                "indexer": {
                    "replicas": 1,
                    "autoscaling": {"disable": True},
                    "resources": resources_scanner_v4_indexer_small,
                },
                "matcher": {
                    "replicas": 1,
                    "autoscaling": {"disable": True},
                    "resources": resources_scanner_v4_matcher_small,
                },
                "db": {"resources": resources_scanner_v4_db_small},
            },
        }
        resources: dict[str, Any] = {}

        if resources_name == "small":
            resources = resources_small
        return resources

    def create_central_yaml(self, resources_name: str, exposure: str) -> str:
        """Create a YAML configuration for central-services Helm chart as a string."""
        base = {
            "central": {
                "adminPassword": {"value": self.central_password},
                "exposure": {"loadBalancer": {"enabled": True}},
                "telemetry": {"enabled": False},
            },
            "scanner": {"replicas": 1, "autoscaling": {"disable": True, "minReplicas": 1}},
            "scannerV4": {
                "indexer": {"replicas": 1, "autoscaling": {"disable": True, "minReplicas": 1}},
                "matcher": {"replicas": 1, "autoscaling": {"disable": True, "minReplicas": 1}},
            },
            "allowNonstandardNamespace": True,
        }

        image_settings = {}
        if self.main_image_tag:
            image_settings = {
                "central": {"db": {"image": {"tag": self.main_image_tag}}, "image": {"tag": self.main_image_tag}},
                "scannerV4": {"image": {"tag": self.main_image_tag}, "db": {"image": {"tag": self.main_image_tag}}},
            }

        resources_overlay = self.get_central_resources(resources_name)
        override_values = self.load_override_dict()
        central_dict = helpers.merge_dicts(base, image_settings, resources_overlay, override_values)
        central_yaml: str = yaml.dump(central_dict)
        return central_yaml

    def get_secured_cluster_resources(self, resources_name: str) -> dict[str, Any]:
        """Return a dict overlay for secured-cluster resources preset."""
        resources_small = {
            "sensor": {
                "resources": resources_sensor_small,
            },
            "scanner": {"disable": True},
            "scannerV4": {"disable": True},
        }
        resources: dict[str, Any] = {}
        if resources_name == "small":
            resources = resources_small
        return resources

    def create_secured_cluster_yaml(self, cluster_name: str, resources_name: str) -> str:
        """Create values YAML for secured-cluster Helm chart as a string."""
        base: dict[str, Any] = {
            "clusterName": cluster_name,
            "centralEndpoint": "https://central.acs-central.svc:443",
            "allowNonstandardNamespace": True,
        }

        image_settings: dict[str, Any] = {}
        if self.main_image_tag:
            image_settings = {
                "image": {"main": {"tag": self.main_image_tag}},
                "scannerV4": {"tag": self.main_image_tag},
                "scannerV4DB": {"tag": self.main_image_tag},
            }

        resources = self.get_secured_cluster_resources(resources_name)
        override_values = self.load_override_dict()
        secured_cluster_dict = helpers.merge_dicts(base, image_settings, resources, override_values)
        secured_cluster_yaml: str = yaml.dump(secured_cluster_dict)
        return secured_cluster_yaml

    def deploy(
        self,
        component: str,
        resources: str,
        exposure: str,
        helm_args: list[str] | None = None,
        input_yaml: str = "",
    ):
        self.logger.print_with_timestamp("Initiating Helm-based deployment of ACS", style="bold cyan")
        # Persist exposure, apply convenience defaults (e.g., kind)
        resources, exposure = self.apply_convenience_defaults(resources, exposure)
        self.exposure = exposure
        self.deploy_component(component, resources, exposure, helm_args, input_yaml)

    def deploy_component(
        self,
        component: str,
        resources: str,
        exposure: str,
        helm_args: list[str] | None = None,
        input_yaml: str = "",
    ):
        if helm_args is None:
            helm_args = []

        if component == "central":
            self.deploy_central(resources, exposure, helm_args, input_yaml)
        elif component == "secured-cluster":
            self.deploy_secured_cluster(resources, helm_args, input_yaml)
        elif component in ["both"]:
            self.deploy_central(resources, exposure, helm_args, input_yaml)
            self.deploy_secured_cluster(resources, helm_args, input_yaml)
        else:
            self.logger.error("Error: central or secured-cluster?")
            raise ValueError("FIXME")

    def deploy_central(self, resources: str, exposure: str, helm_args: list[str], input_yaml: str = ""):
        self.logger.print_with_timestamp(f"Deploying Central with Helm with resources: {resources}", style="bold cyan")
        self.teardown()
        central_yaml = self.create_central_yaml(resources, exposure)

        with tempfile.TemporaryDirectory() as central_chart_dir:
            info_panel = Panel.fit(
                f"[bold]Namespace:        [/bold] {self.central_namespace}\n"
                f"[bold]Context:          [/bold] {self.kube_context}\n"
                f"[bold]Image Tag:        [/bold] {self.main_image_tag or 'default'}\n"
                f"[bold]Log File:         [/bold] {self.log_file}",
                title="[bold magenta]ACS Central Deployment Plan[/bold magenta]",
                border_style="magenta",
            )
            self.console.print(info_panel)

            if self.namespace_exist(self.central_namespace):
                self.teardown_all_async()

            run_command(
                "Instantiating central-services chart",
                [
                    "roxctl",
                    "helm",
                    "output",
                    "central-services",
                    "--remove",
                    "--debug",
                    "--output-dir",
                    central_chart_dir,
                ],
                check=True,
                capture_output=True,
                text=True,
            )

            with (
                tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as central_f,
            ):
                central_f.write(central_yaml)
                central_f.flush()

                helm_template_cmd = [
                    "helm",
                    "template",
                    "-n",
                    self.central_namespace,
                    "stackrox-central-services",
                    central_chart_dir,
                    "-f",
                    central_f.name,
                ] + helm_args

                template_result = run_command(
                    "Rendering central-services chart", helm_template_cmd, capture_output=True, text=True, check=True
                )
                image_refs = []
                for line in template_result.stdout.split("\n"):
                    if 'image: "' in line:
                        image_ref = line.split('image: "')[1].split('"')[0]
                        if "/main:" in image_ref and image_ref not in image_refs:
                            image_refs.append(image_ref)
                if image_refs and not self.image_cache.verify_images_pullable(*image_refs):
                    raise ValueError("Error: One or more images not found.")
                crds = [
                    "centrals.platform.stackrox.io",
                    "securedclusters.platform.stackrox.io",
                    "securitypolicies.config.stackrox.io",
                ]
                run_command(
                    "Deleting CRDs",
                    [self.kubectl, "delete", "crd", "--ignore-not-found=true"] + crds,
                    check=True,
                    capture_output=True,
                    text=True,
                )
                self.ensure_namespace_exists(self.central_namespace)
                self.prepare_namespace(self.central_namespace)

                install_cmd = [
                    "helm",
                    "install",
                    "-n",
                    self.central_namespace,
                    "stackrox-central-services",
                    central_chart_dir,
                    "-f",
                    central_f.name,
                ] + helm_args

                run_command("Installing helm-services chart", install_cmd, check=True, capture_output=True, text=True)

            self.wait_for_ready_deployment(self.central_namespace, "central")
            if self.exposure == "none" and self.port_forwarding_enabled:
                self.start_central_port_forward(self.central_namespace)
            self.wait_for_central_endpoint(self.central_namespace)
            self.fetch_central_ca_cert(self.central_namespace)
            self.show_central_success_panel()

    def deploy_secured_cluster(self, resources: str, helm_args: list[str], input_yaml: str = ""):
        self.logger.print_with_timestamp(
            f"Deploying Secured Cluster with Helm with resources: {resources}", style="bold cyan"
        )
        cluster_name = f"sensor-{random.randint(1000, 9999)}"  # noqa: S311
        values_yaml = self.create_secured_cluster_yaml(cluster_name, resources)

        with tempfile.TemporaryDirectory() as chart_dir:
            info_panel = Panel.fit(
                f"[bold]Cluster Name:     [/bold] {cluster_name}\n"
                f"[bold]Namespace:        [/bold] {self.secured_cluster_namespace}\n"
                f"[bold]Image Tag:        [/bold] {self.main_image_tag or 'default'}",
                title="[bold cyan]ACS Secured Cluster Deployment Plan[/bold cyan]",
                border_style="cyan",
            )
            self.console.print(info_panel)

            if self.namespace_exist(self.secured_cluster_namespace):
                self.teardown("secured-cluster")

            crs_content = self.generate_crs(cluster_name)

            run_command(
                "Instantiating secured-cluster-services chart",
                [
                    "roxctl",
                    "helm",
                    "output",
                    "secured-cluster-services",
                    "--remove",
                    "--debug",
                    "--output-dir",
                    chart_dir,
                ],
                check=True,
                capture_output=True,
            )

            self.ensure_namespace_exists(self.secured_cluster_namespace)
            self.prepare_namespace(self.secured_cluster_namespace)

            with (
                tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as crs_f,
                tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as values_f,
            ):
                crs_f.write(crs_content)
                crs_f.flush()

                values_f.write(values_yaml)
                values_f.flush()

                install_cmd = [
                    "helm",
                    "install",
                    "-n",
                    self.secured_cluster_namespace,
                    "stackrox-secured-cluster-services",
                    chart_dir,
                    "--set-file",
                    f"crs.file={crs_f.name}",
                    "-f",
                    values_f.name,
                ] + helm_args

                run_command(
                    "Installing secured-cluster-services chart", install_cmd, check=True, capture_output=True, text=True
                )

            # success_panel = Panel.fit(
            #     f"[bold green]✓ Secured Cluster Deployment Complete[/bold green]\n\n[bold]Cluster Name:     [/bold] {cluster_name}\n",
            #     title="[bold green]Secured Cluster[/bold green]",
            #     border_style="green",
            # )
            # self.console.print(success_panel)
            self.show_secured_cluster_success_panel()

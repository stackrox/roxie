import subprocess
import tempfile
from typing import List, Optional

from rich.panel import Panel

from deployer import ACSDeployer


class ACSDeployerHelm(ACSDeployer):
    """Helm-specific deployer that implements Helm deployment/teardown."""

    def deploy(self, component: str = "both", helm_args: Optional[List[str]] = None, input_yaml: str = ""):
        self.logger.print_with_timestamp("Initiating Helm-based deployment of ACS", style="bold cyan")
        self.deploy_component(component, helm_args, input_yaml)

    def deploy_component(
        self, component: str = "both", helm_args: Optional[List[str]] = None, input_yaml: str = ""
    ):
        if helm_args is None:
            helm_args = []

        if component == "central":
            self.deploy_central(helm_args, input_yaml)
        elif component == "secured-cluster":
            self.deploy_secured_clusterhelm(helm_args, input_yaml)
        elif component in ["both"]:
            self.deploy_central(helm_args, input_yaml)
            self.deploy_secured_cluster(helm_args, input_yaml)
        else:
            self.logger.error("Error: central or secured-cluster?")
            raise ValueError("FIXME")

    def deploy_central(self, helm_args: List[str], input_yaml: str = ""):
        with tempfile.TemporaryDirectory() as central_chart_dir:
            default_settings = f"""
central:
    adminPassword:
        value: "{self.central_password}"
    exposure:
        loadBalancer:
            enabled: true
    telemetry:
        enabled: false
scanner:
    replicas: 1
    autoscaling:
        disable: true
        minReplicas: 1
scannerV4:
    indexer:
        replicas: 1
        autoscaling:
            disable: true
            minReplicas: 1
    matcher:
        replicas: 1
        autoscaling:
            disable: true
            minReplicas: 1
allowNonstandardNamespace: true
"""

            image_settings = ""
            main_image_tag = self.main_image_tag
            if main_image_tag:
                image_settings = f"""
central:
    db:
        image:
            tag: "{main_image_tag}"
    image:
        tag: "{main_image_tag}"

scannerV4:
    image:
        tag: "{main_image_tag}"
    db:
        image:
            tag: "{main_image_tag}"
"""

            info_panel = Panel.fit(
                f"[bold]Namespace:        [/bold] {self.central_namespace}\n"
                f"[bold]Context:          [/bold] {self.kube_context}\n"
                f"[bold]Image Tag:        [/bold] {main_image_tag or 'default'}\n"
                f"[bold]Log File:         [/bold] {self.log_file}",
                title="[bold magenta]ACS Central Deployment Plan[/bold magenta]",
                border_style="magenta",
            )
            self.console.print(info_panel)

            if self.namespace_exist(self.central_namespace):
                self.teardown_all_async()

            try:
                subprocess.run(
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
            except subprocess.CalledProcessError as e:
                self.logger.error(f"Failed to generate central chart: {e}")
                raise

            with (
                tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as default_f,
                tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as image_f,
                tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as input_f,
            ):
                default_f.write(default_settings)
                default_f.flush()

                image_f.write(image_settings)
                image_f.flush()

                input_f.write(input_yaml)
                input_f.flush()

                helm_template_cmd = [
                    "helm", "template", "-n", self.central_namespace, "stackrox-central-services", central_chart_dir,
                    "-f", default_f.name,
                    "-f", image_f.name,
                    "-f", input_f.name,
                ] + helm_args

                template_result = subprocess.run(helm_template_cmd, capture_output=True, text=True, check=True)
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
            subprocess.run([self.kubectl, "delete", "crd", "--ignore-not-found=true"] + crds, check=True, capture_output=True, text=True)
            self.ensure_namespace_exists(self.central_namespace)
            self.prepare_namespace(self.central_namespace)

            with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as default_f, tempfile.NamedTemporaryFile(
                mode="w", suffix=".yaml"
            ) as image_f, tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as input_f:
                default_f.write(default_settings)
                default_f.flush()

                image_f.write(image_settings)
                image_f.flush()

                input_f.write(input_yaml)
                input_f.flush()

                install_cmd = [
                    "helm", "install", "-n", self.central_namespace, "stackrox-central-services", central_chart_dir,
                    "-f", default_f.name,
                    "-f", image_f.name,
                    "-f", input_f.name,
                ] + helm_args

                subprocess.run(install_cmd, check=True, capture_output=True, text=True)

            self.wait_for_ready_deployment(self.central_namespace, "central")
            self.wait_for_central_endpoint(self.central_namespace)
            # Fetch Central CA certificate and persist to temp file
            try:
                self.fetch_central_ca_cert(self.central_namespace)
            except Exception as e:
                self.console.print(f"Warning: failed to fetch central CA: {e}", style="bold yellow")

            success_panel = Panel.fit(
                f"[bold green]✓ Central Deployment Complete[/bold green]\n\n"
                f"[bold]API Endpoint:     [/bold] {self.central_endpoint}\n"
                f"[bold]Admin Password:   [/bold] {self.central_password}\n"
                f"[bold]Log File:         [/bold] {self.log_file}",
                title="[bold green]Success[/bold green]",
                border_style="green",
            )
            self.console.print(success_panel)

            env_content = f"""
export API_ENDPOINT="{self.central_endpoint}"
export ROX_ADMIN_PASSWORD="{self.central_password}"
"""
            with open(self.central_env_file, "w") as f:
                f.write(env_content)

    def deploy_secured_cluster(self, helm_args: List[str], input_yaml: str = ""):
        main_image_tag = self.main_image_tag
        with tempfile.TemporaryDirectory() as chart_dir:
            import random

            cluster_name = f"sensor-{random.randint(1000, 9999)}"  # noqa: S311

            info_panel = Panel.fit(
                f"[bold]Cluster Name:     [/bold] {cluster_name}\n"
                f"[bold]Namespace:        [/bold] {self.secured_cluster_namespace}\n"
                f"[bold]Central Endpoint: [/bold] central.{self.central_namespace}.svc:443\n"
                f"[bold]Image Tag:        [/bold] {main_image_tag or 'default'}\n",
                title="[bold cyan]ACS Secured Cluster Deployment Plan[/bold cyan]",
                border_style="cyan",
            )
            self.console.print(info_panel)

            if self.namespace_exist(self.secured_cluster_namespace):
                self.log("existing secured cluster")
                self.teardown("secured-cluster")

            default_settings = f"""
clusterName: "{cluster_name}"
centralEndpoint: "{self.central_endpoint}"
allowNonstandardNamespace: true
"""

            image_settings = ""
            if main_image_tag:
                image_settings = f"""
image:
    main:
        tag: "{main_image_tag}"
scannerV4:
    tag: "{main_image_tag}"
scannerV4DB:
    tag: "{main_image_tag}"
"""

            crs_content = self.generate_crs(cluster_name)

            subprocess.run(
                [ "roxctl", "helm", "output", "secured-cluster-services", "--remove", "--debug", "--output-dir", chart_dir ],
                check=True,
                capture_output=True,
            )

            self.ensure_namespace_exists(self.secured_cluster_namespace)
            self.prepare_namespace(self.secured_cluster_namespace)

            with tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as crs_f, tempfile.NamedTemporaryFile(
                mode="w", suffix=".yaml"
            ) as default_f, tempfile.NamedTemporaryFile(
                mode="w", suffix=".yaml"
            ) as image_f, tempfile.NamedTemporaryFile(mode="w", suffix=".yaml") as input_f:
                crs_f.write(crs_content)
                crs_f.flush()

                default_f.write(default_settings)
                default_f.flush()

                image_f.write(image_settings)
                image_f.flush()

                input_f.write(input_yaml)
                input_f.flush()

                install_cmd = [
                    "helm", "install", "-n", self.secured_cluster_namespace, "stackrox-secured-cluster-services", chart_dir,
                    "--set-file", f"crs.file={crs_f.name}",
                    "-f", default_f.name,
                    "-f", image_f.name,
                    "-f", input_f.name,
                ] + helm_args

                try:
                    subprocess.run(install_cmd, check=True, capture_output=True, text=True)
                except subprocess.CalledProcessError as e:
                    raise RuntimeError("Failed to install secured-cluster chart") from e

            success_panel = Panel.fit(
                f"[bold green]✓ Secured Cluster Deployment Complete[/bold green]\n\n[bold]Cluster Name:     [/bold] {cluster_name}\n",
                title="[bold green]Secured Cluster[/bold green]",
                border_style="green",
            )
            self.console.print(success_panel)

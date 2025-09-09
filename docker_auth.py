"""Docker authentication and pull secret management for the roxie deployment tool."""

import base64
import json
import os
import subprocess

from rich.console import Console

from errors import RoxieError


class DockerAuth:
    """Handles Docker authentication and pull secret management"""

    def __init__(self, console: Console | None = None, cache_enabled: bool = True):
        self.console = console or Console()
        self.cache_enabled = cache_enabled
        self._auth_cache: dict[str, str] = {}

    def get_docker_auth_string(self, username: str | None = None, password: str | None = None) -> str:
        """Generate Docker authentication string for image pull secrets"""
        try:
            # Try environment variables first
            env_username = os.environ.get("REGISTRY_USERNAME")
            env_password = os.environ.get("REGISTRY_PASSWORD")

            if env_username and env_password:
                username = env_username
                password = env_password
            else:
                # Try to get from Docker config file
                docker_config_path = os.path.expanduser("~/.docker/config.json")
                if os.path.exists(docker_config_path):
                    return self.get_docker_config_auth(docker_config_path)

                raise RoxieError("No Docker credentials found")

            # Create auth string
            auth_string = f"{username}:{password}"
            encoded_auth = base64.b64encode(auth_string.encode()).decode()

            docker_config = {"auths": {"quay.io": {"auth": encoded_auth}}}

            return json.dumps(docker_config)

        except Exception as e:
            self.console.print(f"Failed to generate Docker auth: {str(e)}", style="bold red")
            return ""

    def get_docker_config_auth(self, config_path: str) -> str:
        """Extract auth from existing Docker config"""
        try:
            with open(config_path) as f:
                config = json.load(f)

            # Check for existing auths
            auths = config.get("auths", {})
            if auths:
                return json.dumps({"auths": auths})

            # Check for credential helpers
            cred_helpers = config.get("credHelpers", {})
            if cred_helpers:
                # Try to use credential helpers
                for registry, helper in cred_helpers.items():
                    try:
                        result = subprocess.run(
                            [f"docker-credential-{helper}", "get"], input=registry, text=True, capture_output=True
                        )
                        if result.returncode == 0:
                            cred_data = json.loads(result.stdout)
                            username = cred_data.get("Username", "")
                            password = cred_data.get("Secret", "")
                            if username and password:
                                auth_string = f"{username}:{password}"
                                encoded_auth = base64.b64encode(auth_string.encode()).decode()
                                return json.dumps({"auths": {registry: {"auth": encoded_auth}}})
                    except Exception as e:  # noqa: S110
                        self.console.print(
                            f"Credential helper '{helper}' for '{registry}' failed: {e}",
                            style="dim yellow",
                        )
                        continue

            return ""

        except (json.JSONDecodeError, OSError):
            return ""

    def create_pull_secret_yaml(self, namespace: str) -> str:
        """Create Kubernetes pull secret YAML"""
        docker_config_json = self.get_docker_auth_string()
        encoded_config = base64.b64encode(docker_config_json.encode()).decode()

        secret_yaml = f"""apiVersion: v1
kind: Secret
metadata:
  name: stackrox
  namespace: {namespace}
type: kubernetes.io/dockerconfigjson
data:
  .dockerconfigjson: {encoded_config}
"""

        return secret_yaml

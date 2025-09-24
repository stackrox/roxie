"""Docker authentication and pull secret management for the roxie deployment tool."""

import base64
import json
import os
import subprocess
from typing import Any

import yaml
from rich.console import Console

from errors import RoxieError

registry = "quay.io"

class DockerAuth:
    """Handles Docker authentication and pull secret management"""

    def __init__(self, console: Console | None = None, cache_enabled: bool = True):
        self.console = console or Console()
        self.cache_enabled = cache_enabled
        self._auth_cache: dict[str, str] = {}

    def get_encoded_docker_auth(self, username: str | None = None, password: str | None = None) -> str:
        """Generate Docker authentication string for image pull secrets"""
        # Try environment variables first
        env_username = os.environ.get("REGISTRY_USERNAME")
        env_password = os.environ.get("REGISTRY_PASSWORD")

        if env_username and env_password:
            username = env_username
            password = env_password
            auth_string = f"{username}:{password}"
            return base64.b64encode(auth_string.encode()).decode()

        docker_config_path = os.path.expanduser("~/.docker/config.json")
        if os.path.exists(docker_config_path):
            encoded_auth = self.get_registry_auth_from_config(docker_config_path, registry)
            if encoded_auth:
                return encoded_auth

        raise RoxieError("No Docker credentials found")

    def get_cred_helper_from_config(self, config_path: str, registry: str) -> str:
        """Get cred helper from existing Docker config"""
        with open(config_path) as f:
            docker_config = json.load(f)

        cred_helper = ""
        cred_store = docker_config.get("credStore", "")
        if cred_store and isinstance(cred_store, str):
            cred_helper = cred_store
        cred_helpers = docker_config.get("credHelpers", {})
        if registry in cred_helpers and isinstance(cred_helpers[registry], str):
            cred_helper = cred_helpers[registry]

        return cred_helper

    def format_cred_helper_result(self, result: dict[str,str]) -> str:
        """Format Docker cred helper result"""
        return f"{result['Username']}:{result['Secret']}"

    def invoke_cred_helper(self, cred_helper: str, registry: str) -> dict[str,Any]:
        """Invoke Docker cred helper"""
        result = subprocess.run(
            [f"docker-credential-{cred_helper}", "get"], input=registry, text=True, capture_output=True)
        stdout = result.stdout.strip()
        json_result = json.loads(stdout)
        if not isinstance(json_result, dict):
            raise RoxieError(f"Invalid cred helper result: {stdout}")
        return json_result

    def get_registry_auth_from_config(self, config_path: str, registry: str) -> str:
        """Extract encoded auth from existing Docker config"""
        with open(config_path) as f:
            docker_config = json.load(f)

        auths = docker_config.get("auths", {})

        if registry not in auths:
            return ""

        if auths[registry] == {}:
            cred_helper = self.get_cred_helper_from_config(docker_config, registry)
            if cred_helper:
                helper_result = self.invoke_cred_helper(cred_helper, registry)
                return self.format_cred_helper_result(helper_result)

        if isinstance(auths[registry], str):
            return auths[registry]

        return ""

    def create_pull_secret_yaml(self, namespace: str) -> str:
        """Create Kubernetes pull secret YAML"""
        encoded_auth = self.get_encoded_docker_auth()
        docker_config = {"auths": {registry: {"auth": encoded_auth}}}
        encoded_config = base64.b64encode(json.dumps(docker_config)).encode()
        secret = {
            "apiVersion": "v1",
            "kind": "Secret",
            "metadata": {
                "name": "stackrox",
                "namespace": namespace,
            },
            "type": "kubernetes.io/dockerconfigjson",
            "data": {
                ".dockerconfigjson": encoded_config,
            },
        }
        return yaml.dump(secret)

import base64
import json
import yaml

from docker_auth import DockerAuth

def test_create_pull_secret_yaml_from_env(monkeypatch):
    monkeypatch.setenv("REGISTRY_USERNAME", "user")
    monkeypatch.setenv("REGISTRY_PASSWORD", "pass")

    da = DockerAuth()
    yaml_text = da.create_pull_secret_yaml("ns")
    secret = yaml.safe_load(yaml_text)

    assert secret["kind"] == "Secret"
    assert "data" in secret
    data = secret["data"]
    assert ".dockerconfigjson" in data
    dockerconfigjson = data[".dockerconfigjson"]
    decoded_config = base64.b64decode(dockerconfigjson).decode()
    config = json.loads(decoded_config)
    assert "auths" in config
    assert "quay.io" in config["auths"]
    registry_auth = config["auths"]["quay.io"]["auth"]
    decoded_auth = base64.b64decode(registry_auth).decode()
    assert decoded_auth == "user:pass"

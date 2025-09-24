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

    # Extract the encoded dockerconfigjson and verify it's valid base64 JSON
    assert "apiVersion: v1" in secret
    assert "kind: Secret" in secret
    assert "ns" in secret

    # Find the base64 encoded field
    encoded = yaml_text.split(":")[-1].strip()
    decoded = base64.b64decode(encoded).decode()
    data = json.loads(decoded)
    assert "auths" in data

# def test_create_pull_secret_yaml_from_env(monkeypatch):
#     monkeypatch.setenv("REGISTRY_USERNAME", "user")
#     monkeypatch.setenv("REGISTRY_PASSWORD", "pass")

#     da = DockerAuth()
#     yaml_text = da.create_pull_secret_yaml("ns")

#     # Extract the encoded dockerconfigjson and verify it's valid base64 JSON
#     assert "apiVersion: v1" in yaml_text
#     assert "kind: Secret" in yaml_text
#     assert "ns" in yaml_text

#     # Find the base64 encoded field
#     encoded = yaml_text.split(":")[-1].strip()
#     decoded = base64.b64decode(encoded).decode()
#     data = json.loads(decoded)
#     assert "auths" in data

import yaml

from deployer_helm import ACSDeployerHelm
from deployer_operator import ACSDeployerOperator


def test_create_central_cr_respects_override(tmp_path):
    # Prepare an override YAML that flips telemetry and adds an annotation
    override = {
        "metadata": {
            "annotations": {
                "example.com/central": "yes",
            }
        },
        "spec": {
            "central": {
                "telemetry": {"enabled": True},
            }
        },
    }
    override_path = tmp_path / "central_override.yaml"
    override_path.write_text(yaml.safe_dump(override))

    # Instantiate without running __init__ to avoid external calls (kubectl, roxctl)
    op = object.__new__(ACSDeployerOperator)
    # Minimal attributes/methods needed by create_central_cr
    op.central_namespace = "acs-central"
    op.override_file = str(override_path)

    cr = op.create_central_cr(resources_name="default", exposure="none")

    # Override should be present and take precedence
    assert cr["metadata"]["annotations"]["example.com/central"] == "yes"
    assert cr["spec"]["central"]["telemetry"]["enabled"] is True


def test_create_secured_cluster_cr_respects_override(tmp_path):
    # Prepare an override YAML for operator SecuredCluster CR
    override = {
        "metadata": {
            "annotations": {"example.com/sc": "ok"},
        },
        "spec": {
            "admissionControl": {"replicas": 3},
            "scanner": {"analyzer": {"scaling": {"autoScaling": "Disabled"}}},
        },
    }
    override_path = tmp_path / "sc_override.yaml"
    override_path.write_text(yaml.safe_dump(override))

    op = object.__new__(ACSDeployerOperator)
    # Minimal attributes/methods used by create_secured_cluster_cr
    op.secured_cluster_namespace = "acs-sensor"
    op.cluster_name = "sensor-x"
    op.central_namespace = "acs-central"
    op.central_endpoint = "https://central:443"
    op.override_file = str(override_path)

    def _fake_generate_crs(cluster_name: str) -> str:  # noqa: ARG001
        return "kind: Secret\napiVersion: v1"

    op.generate_crs = _fake_generate_crs  # type: ignore[method-assign]

    sc = op.create_secured_cluster_cr(resources_name="default", cluster_name="sensor-x")

    assert sc["metadata"]["annotations"]["example.com/sc"] == "ok"
    assert sc["spec"]["admissionControl"]["replicas"] == 3
    assert sc["spec"]["scanner"]["analyzer"]["scaling"]["autoScaling"] == "Disabled"


def test_create_central_yaml_respects_override(tmp_path):
    # Prepare an override values YAML for Helm central chart
    override = {
        "central": {
            "telemetry": {"enabled": True},
        },
        "scanner": {
            "autoscaling": {"disable": False},
        },
    }
    override_path = tmp_path / "central_values_override.yaml"
    override_path.write_text(yaml.safe_dump(override))

    helm = object.__new__(ACSDeployerHelm)
    helm.central_password = "pw"  # noqa: S105
    helm.main_image_tag = ""  # avoid image settings branch
    helm.override_file = str(override_path)

    values_yaml = helm.create_central_yaml(resources_name="default", exposure="none")
    values = yaml.safe_load(values_yaml)
    assert values["central"]["telemetry"]["enabled"] is True
    assert values["scanner"]["autoscaling"]["disable"] is False


def test_helm_create_secured_cluster_yaml_respects_override(tmp_path):
    # Prepare an override values YAML for Helm secured-cluster chart
    override = {
        "allowNonstandardNamespace": False,
        "scanner": {"disable": True},
    }
    override_path = tmp_path / "sc_values_override.yaml"
    override_path.write_text(yaml.safe_dump(override))

    helm = object.__new__(ACSDeployerHelm)
    helm.main_image_tag = ""  # avoid image settings branch
    helm.override_file = str(override_path)

    values_yaml = helm.create_secured_cluster_yaml(cluster_name="sensor-x", resources_name="default")
    values = yaml.safe_load(values_yaml)
    assert values["allowNonstandardNamespace"] is False
    assert values["scanner"]["disable"] is True

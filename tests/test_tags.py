from deployer import ACSDeployer


def test_convert_main_tag_to_operator_tag_basic(monkeypatch):
    monkeypatch.setenv("MAIN_IMAGE_TAG", "4.9.x-441-g7754d5a916")
    d = ACSDeployer()
    assert d.operator_tag == "v4.9.0-441-g7754d5a916"


def test_convert_main_tag_to_operator_tag_strips_dirty(monkeypatch):
    monkeypatch.setenv("MAIN_IMAGE_TAG", "4.9.x-441-g7754d5a916")
    d = ACSDeployer()
    assert d.convert_main_tag_to_operator_tag("4.9.x-441-g7754d5a916-dirty") == "v4.9.0-441-g7754d5a916"


def test_determine_operator_tag_from_env(monkeypatch):
    monkeypatch.setenv("MAIN_IMAGE_TAG", "4.12.x-100-gabc123")
    d = ACSDeployer()
    assert d.operator_tag == "v4.12.0-100-gabc123"

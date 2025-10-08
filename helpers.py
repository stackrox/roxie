"""Helper classes for the roxie deployment tool.

This module contains utility classes for timestamping and progress display.
"""

import copy
import subprocess
import time
from typing import Any

import yaml
from deepmerge import Merger
from rich.progress import ProgressColumn
from rich.text import Text

from errors import RoxieError


class TimestampColumn(ProgressColumn):
    """A column that shows a live timestamp relative to start time"""

    def __init__(self, start_time: float):
        super().__init__()
        self.start_time = start_time

    def render(self, task) -> Text:
        """Render the current timestamp"""
        elapsed = time.time() - self.start_time
        minutes = int(elapsed // 60)
        seconds = int(elapsed % 60)
        timestamp = f"{minutes:02d}:{seconds:02d}"
        return Text(timestamp, style="dim blue")


def get_current_cluster_context() -> str:
    """Get the current cluster context"""
    result = subprocess.run(["kubectl", "config", "current-context"], capture_output=True, text=True, check=True)
    return result.stdout.strip()


def get_container_tool() -> str:
    return "podman"


def run_command(title: str, cmd: list[str], **kwargs) -> subprocess.CompletedProcess[Any]:
    try:
        return subprocess.run(cmd, **kwargs)
    except subprocess.CalledProcessError as e:
        raise RoxieError(f"Step '{title}' failed", stderr=e.stderr) from e


def merge_dicts(base_dict: dict[str, Any], *dicts: dict[str, Any]) -> dict[str, Any]:
    """Merge multiple dictionaries, later dicts override earlier dicts"""
    merger = Merger([(dict, ["merge"])], ["override"], ["override"])
    result = copy.deepcopy(base_dict)
    for d in dicts:
        merger.merge(result, d)
    return result


def load_yaml_file(path: str | None) -> dict[str, Any]:
    """Load a YAML file as a dict. Returns empty dict for empty files.

    Raises RoxieError if the file cannot be read or if content is not a mapping.
    """
    if not path:
        return {}
    try:
        with open(path) as f:
            data = yaml.safe_load(f) or {}
    except Exception as e:  # noqa: S110
        raise RoxieError(f"Failed to read YAML file: {path}: {e}") from e
    if not isinstance(data, dict):
        raise RoxieError("YAML content must be a mapping (dict)")
    return data

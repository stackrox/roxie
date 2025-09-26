"""Helper classes for the roxie deployment tool.

This module contains utility classes for timestamping and progress display.
"""

import copy
import subprocess
import time
from typing import Any

from deepmerge import Merger
from rich.progress import ProgressColumn
from rich.text import Text


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


def merge_dicts(base_dict: dict[str, Any], *dicts: dict[str, Any]) -> dict[str, Any]:
    """Merge multiple dictionaries, later dicts override earlier dicts"""
    merger = Merger([(dict, ["merge"])], ["override"], ["override"])
    result = copy.deepcopy(base_dict)
    for d in dicts:
        merger.merge(result, d)
    return result

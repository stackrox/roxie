"""Package entry point for roxie.

This thin wrapper imports the repository-root `main.py` when `PYTHONPATH`
includes the repo root (as ensured by `bin/roxie`).
"""

import importlib
import sys
from typing import cast


def main() -> int:
    module = importlib.import_module("main")
    return cast(int, module.main())


if __name__ == "__main__":
    sys.exit(main())

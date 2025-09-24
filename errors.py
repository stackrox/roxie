class RoxieError(Exception):
    """Base for user-facing, expected errors (no traceback by default)."""

    def __init__(self, message: str, stderr: bytes | str | None = None):
        super().__init__(message)
        self.stderr = stderr

import time
from rich.console import Console


class Logger:
    def __init__(self):
        self.console = Console()
        self.start_time = time.time()

    def log(self, message: str):
        self.console.print(message)

    def print_with_timestamp(self, message: str, style: str = "dim"):
        self.console.print(f"[dim]{self.get_timestamp()}[/dim] {message}")

    def get_timestamp(self) -> str:
        """Get relative timestamp since start"""
        elapsed = time.time() - self.start_time
        minutes = int(elapsed // 60)
        seconds = int(elapsed % 60)
        return f"{minutes:02d}:{seconds:02d}"

    def info(self, message: str) -> None:
        """Print info message with pink styling"""
        timestamp = self.get_timestamp()
        self.console.print(f"[dim]{timestamp}[/dim] {message}", style="bold magenta")

    def error(self, message: str) -> None:
        """Print error message with red styling"""
        timestamp = self.get_timestamp()
        error_console = Console(stderr=True)
        error_console.print(f"[dim]{timestamp}[/dim] {message}", style="bold red")

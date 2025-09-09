import time

from rich.console import Console

console = Console()


def _ts() -> str:
    return time.strftime("%H:%M:%S")


def pytest_runtest_setup(item):
    console.print()
    console.rule(f"[bold cyan]START[/bold cyan] [dim]{_ts()}[/dim] {item.nodeid}", style="cyan")


def pytest_runtest_teardown(item, nextitem):
    console.rule(f"[bold magenta]END[/bold magenta] [dim]{_ts()}[/dim] {item.nodeid}", style="magenta")
    console.print()



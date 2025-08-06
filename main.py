"""Main entry point for the roxie deployment tool."""

import argparse
from subprocess import CalledProcessError

from rich.console import Console

from deployer_helm import ACSDeployerHelm
from deployer_operator import ACSDeployerOperator
from errors import RoxieError
from logger import Logger


def main() -> int:
    """Main function for roxie deployment tool"""
    parser = argparse.ArgumentParser(description="roxie - Advanced Cluster Security Deployment Tool")
    subparsers = parser.add_subparsers(dest="command", help="Available commands")

    # Deploy subcommand
    deploy_parser = subparsers.add_parser("deploy", help="Deploy ACS components")
    deploy_parser.add_argument(
        "component",
        nargs="?",
        default="both",
        choices=["central", "secured-cluster", "both", "all"],
        help="Component to deploy",
    )
    deploy_parser.add_argument(
        "--helm",
        action="store_true",
        help="Deploy using Helm charts instead of operator (default is operator). Use -- to separate helm args.",
    )

    # Teardown subcommand
    teardown_parser = subparsers.add_parser("teardown", help="Teardown ACS components")
    teardown_parser.add_argument(
        "component",
        nargs="?",
        default="both",
        choices=["central", "secured-cluster", "both"],
        help="Component to teardown",
    )
    teardown_parser.add_argument("--operator", action="store_true", help="Force teardown of operator deployment")
    teardown_parser.add_argument("--helm", action="store_true", help="Force teardown of Helm deployment")

    # upgrade-operator subcommand
    upgrade_operator_parser = subparsers.add_parser("upgrade-operator", help="Upgrade the ACS operator")

    # deploy-operator subcommand
    deploy_operator_parser = subparsers.add_parser("deploy-operator", help="Deploy the ACS operator")

    # # Roxctl subcommand
    # subparsers.add_parser('roxctl', help='Run roxctl commands')
    # Roxcurl subcommand
    # subparsers.add_parser('roxcurl', help='Run roxcurl commands')

    # Parse known args to handle roxctl/roxcurl pass-through arguments and helm args after --
    args, unknown_args = parser.parse_known_args()

    if not args.command:
        parser.print_help()
        return 0

    console = Console()

    try:
        if getattr(args, "helm", False):
            deployer = ACSDeployerHelm(console=console)
        else:
            deployer = ACSDeployerOperator(console=console)

        if args.command == "deploy":
            deployer.deploy(args.component)
        elif args.command == "teardown":
            deployer.teardown(args.component)
        elif args.command == "upgrade-operator":
            deployer.upgrade_operator()
        elif args.command == "deploy-operator":
            deployer.deploy_operator()
        else:
            raise RoxieError(f"Unknown command: {args.command}")

    except KeyboardInterrupt:
        console.print("\nOperation cancelled by user", style="bold yellow")
        return 1
    except CalledProcessError as e:
        console.print(f"[bold red]{e}[/bold red]")
        if e.stderr:
            console.print(f"[dim]stderr: {e.stderr}[/dim]")
        return 1
    except RoxieError as e:
        console.print(f"[bold red]{e}[/bold red]")
        return 1
    except Exception as e:
        console.print(f"Unexpected error: {str(e)}", style="bold red")
        console.print_exception()  # rich traceback
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())

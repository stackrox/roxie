"""Main entry point for the roxie deployment tool."""

import argparse
import os
import subprocess
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
    deploy_parser.add_argument(
        "--shell",
        help="Specify shell to spawn as sub-shell after Central deployment. If not provided, roxie will use the shell specified by the SHELL environment variable.",
    )
    deploy_parser.add_argument(
        "--envrc",
        nargs="?",
        const="",
        default=None,
        help=(
            "Preserve envrc behavior: write API_ENDPOINT and ROX_ADMIN_PASSWORD to a file (default path if omitted). "
            "If not provided, roxie will spawn a subshell with these variables set and teardown on exit."
        ),
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

        # If --envrc provided, optionally override the output path on the deployer
        envrc_provided = getattr(args, "envrc", None) is not None
        if envrc_provided:
            if args.envrc:
                deployer.central_env_file = args.envrc

        if args.command == "deploy":
            env = os.environ.copy()

            if args.component == "central" or args.component == "both":
                if "ROXIE_SHELL" in env:
                    raise RoxieError("Already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again.")

            deployer.deploy(args.component)

            # Spawn subshell only for central/both when --envrc is not used
            if args.component in ("central", "both") and not envrc_provided:
                banner = (
                    "\n[roxie] Entering a subshell with ACS environment variables set.\n"
                    "[roxie] Exit this shell to trigger automatic teardown.\n"
                )
                console.print(banner, style="bold cyan")

                if getattr(deployer, "central_endpoint", ""):
                    env["API_ENDPOINT"] = deployer.central_endpoint
                    env["ROX_ENDPOINT"] = deployer.central_endpoint # For roxctl
                if getattr(deployer, "central_password", ""):
                    env["ROX_ADMIN_PASSWORD"] = deployer.central_password
                ca_file = getattr(deployer, "ca_cert_file", "")
                if ca_file:
                    env["ROX_CA_CERT_FILE"] = ca_file
                env["ROXIE_SHELL"] = "1"

                shell = getattr(args, "shell")
                if shell is None:
                    shell = os.environ.get("ROXIE_USER_SHELL")
                deployer.logger.print_with_timestamp(f"Spawning sub-shell: {shell}", style="bold cyan")
                try:
                    subprocess.run([shell, "-i"], check=False, env=env)
                finally:
                    try:
                        deployer.teardown(args.component)
                    except Exception:
                        console.print(
                            "[roxie] Teardown after subshell exit encountered errors",
                            style="bold yellow",
                        )
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

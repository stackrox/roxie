"""Main entry point for the roxie deployment tool."""

import argparse
import contextlib
import os
import subprocess
import sys
import tempfile
from subprocess import CalledProcessError

from rich.console import Console

from deployer import ACSDeployer
from deployer_helm import ACSDeployerHelm
from deployer_operator import ACSDeployerOperator
from errors import RoxieError


def main() -> int:
    """Main function for roxie deployment tool"""
    parser = argparse.ArgumentParser(prog="roxie", description="roxie - Advanced Cluster Security Deployment Tool")
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
        "--port-forwarding",
        action="store_true",
        help="Enable localhost port-forward for Central in the spawned subshell.",
    )
    deploy_parser.add_argument(
        "--exposure",
        choices=["loadbalancer", "none"],
        default="loadbalancer",
        help="Central exposure backend (default: loadbalancer).",
    )
    deploy_parser.add_argument(
        "--resources",
        choices=["small", "default"],
        default="default",
        help="Resource sizing preset for deployments (currently no-op).",
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
            "If not provided, roxie will spawn a subshell with these variables set."
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

    # Parse arguments strictly: unknown args cause failure. Helm args must follow '--' and are captured below.
    args = parser.parse_args()

    if not args.command:
        parser.print_help()
        return 0

    console = Console()

    try:
        deployer: ACSDeployer
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
                    raise RoxieError(
                        "Already in a roxie sub-shell (ROXIE_SHELL environment variable is set), please exit the shell and try again."
                    )

            # Persist subshell port-forwarding preference
            exposure_val = getattr(args, "exposure", "loadbalancer")
            deployer.port_forwarding_enabled = bool(getattr(args, "port_forwarding", False)) or exposure_val == "none"
            deployer.deploy(
                args.component,
                resources=getattr(args, "resources", "default"),
                exposure=exposure_val,
            )

            # Spawn subshell only for central/both when --envrc is not used
            if args.component in ("central", "both"):
                if envrc_provided:
                    env_content = f"""
export API_ENDPOINT="{deployer.central_endpoint}"
export ROX_ENDPOINT="{deployer.central_endpoint}"
export ROX_BASE_URL="https://{deployer.central_endpoint}"
export ROX_ADMIN_PASSWORD="{deployer.central_password}"
export ROX_CA_CERT_FILE="{deployer.rox_ca_cert_file}"
"""
                    with open(deployer.central_env_file, "w") as f:
                        f.write(env_content)
                else:
                    shell = str(args.shell or os.environ.get("ROXIE_USER_SHELL"))
                    deployer.logger.print_with_timestamp(f"Spawning sub-shell: {shell}", style="bold cyan")
                    banner = (
                        "\n[roxie] Entering a subshell with ACS environment variables set.\n"
                        "\n"
                        "[roxie] Environment is set up for talking to ACS Central. Examples:\n"
                        "\n"
                        "[roxie]   * roxctl central whoami\n"
                        "[roxie]   * roxcurl /v1/clusters\n"
                        "\n"
                        "[roxie] Central UI: http://localhost:8080 (username: admin, password: see $ROX_ADMIN_PASSWORD)\n"
                        "\n"
                    )
                    console.print(banner, style="bold cyan")

                    if getattr(deployer, "central_endpoint", ""):
                        env["API_ENDPOINT"] = deployer.central_endpoint
                        env["ROX_ENDPOINT"] = deployer.central_endpoint  # For roxctl
                        env["ROX_BASE_URL"] = f"https://{deployer.central_endpoint}"  # For roxcurl
                    if getattr(deployer, "central_password", ""):
                        env["ROX_ADMIN_PASSWORD"] = deployer.central_password
                    ca_file = getattr(deployer, "rox_ca_cert_file", "")
                    if ca_file:
                        env["ROX_CA_CERT_FILE"] = ca_file
                    env["ROXIE_SHELL"] = "1"
                    env["name"] = f"acs@{deployer.kube_context}"

                    # Create a temporary HAProxy config and tie HAProxy process lifetime to the subshell via ExitStack
                    with contextlib.ExitStack() as cleanup:
                        endpoint = str(getattr(deployer, "central_endpoint", ""))
                        ca_file = str(getattr(deployer, "rox_ca_cert_file", ""))

                        if endpoint and ca_file:
                            haproxy_cfg_path = None
                            tmp = tempfile.NamedTemporaryFile(
                                mode="w", suffix=".cfg", prefix="roxie-haproxy-", delete=False
                            )
                            try:
                                haproxy_cfg_path = tmp.name
                                tmp.write(
                                    f"""
global
    log /dev/null local0

defaults
    log     global
    mode    http
    timeout connect 5s
    timeout client  30s
    timeout server  30s

frontend http_front
    bind *:8080
    default_backend https_back

backend https_back
    server srv1 {endpoint} ssl verify required ca-file {ca_file}
"""
                                )
                            finally:
                                tmp.close()

                            env["ROXIE_HAPROXY_CFG_FILE"] = haproxy_cfg_path
                            cleanup.callback(lambda: os.path.exists(haproxy_cfg_path) and os.remove(haproxy_cfg_path))

                            # Start HAProxy in the background; silence stdout/stderr
                            try:
                                haproxy_proc = subprocess.Popen(
                                    ["haproxy", "-f", haproxy_cfg_path],
                                    stdout=subprocess.DEVNULL,
                                    stderr=subprocess.DEVNULL,
                                )

                                def _stop_haproxy() -> None:
                                    try:
                                        haproxy_proc.terminate()
                                        try:
                                            haproxy_proc.wait(timeout=3)
                                        except Exception:
                                            haproxy_proc.kill()
                                    except Exception as e:
                                        print("Failed to stop haproxy: ", e)
                                        pass

                                cleanup.callback(_stop_haproxy)
                            except Exception as e:
                                deployer.logger.print_with_timestamp(
                                    f"Failed to start haproxy: {e}", style="bold yellow"
                                )
                        else:
                            deployer.logger.print_with_timestamp(
                                "HAProxy config not created (missing endpoint or CA cert)", style="dim yellow"
                            )

                        # Spawn the subshell with the environment prepared
                        subprocess.run([shell, "-i"], check=False, env=env)

        elif args.command == "teardown":
            deployer.teardown(args.component)
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

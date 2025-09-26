import os
import signal
import socket
import subprocess
import time


class PortForwardManager:
    """Manage a kubectl port-forward subprocess and expose a localhost endpoint.

    The port-forward process is started in its own process group so it can be
    cleanly terminated. This class is intentionally small and self-contained.
    """

    def __init__(self, kubectl: str, logger) -> None:
        self._kubectl = kubectl
        self._logger = logger
        self._proc: subprocess.Popen[str] | None = None
        self._local_port: int | None = None

    def _find_free_local_port(self, preferred_port: int = 8443) -> int:
        s = socket.socket(socket.AF_INET, socket.SOCK_STREAM)
        try:
            s.bind(("127.0.0.1", preferred_port))
            s.close()
            return preferred_port
        except OSError:
            s.close()
            with socket.socket(socket.AF_INET, socket.SOCK_STREAM) as s2:
                s2.bind(("127.0.0.1", 0))
                return int(s2.getsockname()[1])

    def _wait_tcp_ready(self, host: str, port: int, timeout_seconds: float = 10.0) -> bool:
        start = time.time()
        while time.time() - start < timeout_seconds:
            try:
                with socket.create_connection((host, port), timeout=0.3):
                    return True
            except OSError:
                time.sleep(0.2)
        return False

    def start(
        self,
        namespace: str,
        service_name: str,
        remote_port: int = 443,
        preferred_local_port: int = 8443,
    ) -> str:
        """Start port-forward to the given service; returns "127.0.0.1:<port>".

        If already running, this is a no-op and returns the existing endpoint.
        """
        if self._proc and self._proc.poll() is None and self._local_port:
            return f"127.0.0.1:{self._local_port}"

        local_port = self._find_free_local_port(preferred_local_port)
        proc = subprocess.Popen(
            [
                self._kubectl,
                "-n",
                namespace,
                "port-forward",
                f"svc/{service_name}",
                f"{local_port}:{remote_port}",
                "--address",
                "127.0.0.1",
            ],
            stdout=subprocess.DEVNULL,
            stderr=subprocess.DEVNULL,
            text=True,
            start_new_session=True,
        )

        if not self._wait_tcp_ready("127.0.0.1", local_port, timeout_seconds=20):
            try:
                os.killpg(proc.pid, signal.SIGTERM)
            except Exception as e:
                self._logger.print_with_timestamp(f"Port-forward TERM failed: {e}", style="dim yellow")
            try:
                proc.wait(timeout=2)
            except Exception:
                try:
                    os.killpg(proc.pid, signal.SIGKILL)
                except Exception as e2:
                    self._logger.print_with_timestamp(f"Port-forward KILL failed: {e2}", style="dim yellow")
            raise RuntimeError("Port-forward to Central did not become ready")

        self._proc = proc
        self._local_port = local_port
        endpoint = f"127.0.0.1:{local_port}"
        self._logger.print_with_timestamp(f"✓ Port-forward active at https://{endpoint}", style="bold green")
        return endpoint

    def stop(self) -> None:
        if not self._proc:
            return
        try:
            if self._proc.poll() is None:
                try:
                    os.killpg(self._proc.pid, signal.SIGTERM)
                except Exception as e:
                    self._logger.print_with_timestamp(f"Port-forward TERM failed: {e}", style="dim yellow")
                try:
                    self._proc.wait(timeout=2)
                except Exception:
                    try:
                        os.killpg(self._proc.pid, signal.SIGKILL)
                    except Exception as e2:
                        self._logger.print_with_timestamp(f"Port-forward KILL failed: {e2}", style="dim yellow")
        finally:
            self._proc = None
            self._local_port = None

    def is_running(self) -> bool:
        return bool(self._proc and self._proc.poll() is None)

    def get_local_endpoint(self) -> str | None:
        if self._local_port and self.is_running():
            return f"127.0.0.1:{self._local_port}"
        return None

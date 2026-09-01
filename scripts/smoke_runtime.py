"""Exercise the built process on an ephemeral loopback port; never deploys."""
import json
import pathlib
import signal
import socket
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

binary = pathlib.Path("bin/northway").resolve()
version = json.loads(subprocess.check_output([str(binary), "version"], text=True))
assert version["os"] == "linux", version

def expect_failure(args, message):
    result = subprocess.run([str(binary), *args], capture_output=True, text=True, timeout=5)
    assert result.returncode == 1, (args, result.returncode, result.stderr)
    assert result.stdout == "", result.stdout
    records = [json.loads(line) for line in result.stderr.splitlines()]
    assert len(records) == 1, records
    record = records[0]
    assert record["level"] == "ERROR" and record["msg"] == "command failed", record
    assert message in record["error"], record

expect_failure(["serve", "--listen=127.0.0.1:0", "--shutdown-timeout=0s", "--log-level=info"], "shutdown timeout must be between")
expect_failure(["serve", "--listen=127.0.0.1:0", "--poll-tenant=00000000-0000-4000-8000-000000000001"], "poll tenant requires configured storage")
expect_failure(["backup"], "backup requires database and output paths")
expect_failure(["healthcheck", "extra"], "healthcheck takes no arguments")
with socket.socket() as occupied:
    occupied.bind(("127.0.0.1", 0))
    occupied.listen()
    expect_failure(["serve", "--listen=127.0.0.1:" + str(occupied.getsockname()[1]), "--shutdown-timeout=1s", "--log-level=info"], "listen:")

opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
with tempfile.TemporaryFile(mode="w+", encoding="utf-8") as logs:
    process = subprocess.Popen([str(binary), "serve", "--listen=127.0.0.1:0", "--shutdown-timeout=1s", "--log-level=info"], stdout=subprocess.DEVNULL, stderr=logs)
    try:
        address = None
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            logs.seek(0)
            diagnostics = logs.read()
            for line in diagnostics.splitlines():
                try:
                    record = json.loads(line)
                except json.JSONDecodeError:
                    continue  # Preserve raw/partial lines for failure diagnostics.
                if isinstance(record, dict) and record.get("msg") == "server listening":
                    address = record.get("address")
            if process.poll() is not None:
                logs.seek(0)
                raise RuntimeError("server exited before binding: " + logs.read())
            if address:
                break
            time.sleep(0.05)
        assert address is not None, "server never reported its bound address"
        for path, expected, status in [("/healthz", 200, "alive"), ("/readyz", 503, "not_ready"), ("/statusz", 200, "disabled")]:
            try:
                response = opener.open("http://" + address + path, timeout=2)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                assert response.status == expected, (path, response.status)
                assert json.load(response)["status"] == status
        process.send_signal(signal.SIGTERM)
        assert process.wait(timeout=5) == 0, "SIGTERM did not exit cleanly"
        print("PASS: built CLI version, JSON fatal errors/nonzero exits, ephemeral HTTP health/readiness/status and SIGTERM shutdown")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait(timeout=5)

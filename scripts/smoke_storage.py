"""Exercise migrate/serve/restart against a private, real database file."""
import json
import os
import pathlib
import signal
import subprocess
import tempfile
import time
import urllib.request
import urllib.error

binary = pathlib.Path("bin/northway").resolve()
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
with tempfile.TemporaryDirectory(prefix="northway-storage-") as directory:
    os.chmod(directory, 0o700)
    path = str(pathlib.Path(directory) / "northway.sqlite")
    missing = subprocess.run([str(binary), "serve", "--database=" + path], capture_output=True, timeout=10)
    assert missing.returncode == 1 and not pathlib.Path(path).exists(), "serve implicitly created storage"
    for _ in range(2):
        migration = subprocess.run([str(binary), "migrate", "--database=" + path], capture_output=True, text=True, timeout=20)
        assert migration.returncode == 0, migration.stderr
        assert json.loads(migration.stderr)["msg"] == "migrations complete"
    assert pathlib.Path(path).stat().st_mode & 0o777 == 0o600
    for _ in range(2):
        with tempfile.TemporaryFile(mode="w+") as logs:
            process = subprocess.Popen([str(binary), "serve", "--database=" + path, "--listen=127.0.0.1:0", "--log-level=info"], stderr=logs)
            try:
                address = None
                deadline = time.monotonic() + 10
                while time.monotonic() < deadline:
                    logs.seek(0)
                    for line in logs.read().splitlines():
                        try:
                            row = json.loads(line)
                        except json.JSONDecodeError:
                            continue
                        if row.get("msg") == "server listening":
                            address = row["address"]
                    if process.poll() is not None:
                        logs.seek(0)
                        raise RuntimeError(logs.read())
                    if address:
                        break
                    time.sleep(0.05)
                assert address, "storage server did not bind"
                with opener.open("http://" + address + "/readyz", timeout=2) as response:
                    assert response.status == 200 and json.load(response)["status"] == "ready"
                for route in ["/v1/feed-queries", "/v1/feedback", "/v1/snapshots/00000001-0000-4000-8000-000000000000"]:
                    try:
                        opener.open("http://" + address + route, timeout=2)
                        raise AssertionError("unauthenticated product route accepted")
                    except urllib.error.HTTPError as response:
                        assert response.code == 401 and json.load(response)["code"] == "unauthorized"
                overlap = subprocess.run([str(binary), "migrate", "--database=" + path], capture_output=True, text=True, timeout=5)
                assert overlap.returncode == 1 and "already owned" in json.loads(overlap.stderr)["error"]
                process.send_signal(signal.SIGTERM)
                assert process.wait(timeout=5) == 0
            finally:
                if process.poll() is None:
                    process.kill()
                    process.wait(timeout=5)
    print("PASS: explicit/idempotent migration, private file, storage readiness, exclusive ownership and restart")

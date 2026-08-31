"""Exercise the built process on an ephemeral loopback port; never deploys."""
import json
import pathlib
import signal
import subprocess
import tempfile
import time
import urllib.error
import urllib.request

binary = pathlib.Path("bin/northway").resolve()
version = json.loads(subprocess.check_output([str(binary), "version"], text=True))
assert version["os"] == "linux", version
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
with tempfile.TemporaryFile(mode="w+", encoding="utf-8") as logs:
    process = subprocess.Popen([str(binary), "serve", "--listen=127.0.0.1:0", "--shutdown-timeout=1s", "--log-level=info"], stdout=subprocess.DEVNULL, stderr=logs)
    try:
        address = None
        deadline = time.monotonic() + 10
        while time.monotonic() < deadline:
            logs.seek(0)
            for line in logs.read().splitlines():
                record = json.loads(line)
                if record.get("msg") == "server listening":
                    address = record["address"]
            if address:
                break
            if process.poll() is not None:
                raise RuntimeError("server exited before binding")
            time.sleep(0.05)
        assert address is not None, "server never reported its bound address"
        for path, expected, status in [("/healthz", 200, "alive"), ("/readyz", 503, "not_ready")]:
            try:
                response = opener.open("http://" + address + path, timeout=2)
            except urllib.error.HTTPError as error:
                response = error
            with response:
                assert response.status == expected, (path, response.status)
                assert json.load(response)["status"] == status
        process.send_signal(signal.SIGTERM)
        assert process.wait(timeout=5) == 0, "SIGTERM did not exit cleanly"
        print("PASS: built CLI version, ephemeral HTTP health/readiness and SIGTERM shutdown")
    finally:
        if process.poll() is None:
            process.kill()
            process.wait(timeout=5)

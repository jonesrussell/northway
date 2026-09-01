"""Exercise operator commands without printing the generated disposable key."""
import json
import os
from pathlib import Path
import stat
import subprocess
import tempfile

binary = Path("bin/northway").resolve()
tenant = "00000001-0000-4000-8000-000000000000"

with tempfile.TemporaryDirectory(prefix="northway-identity-") as directory:
    root = Path(directory)
    root.chmod(0o700)
    database = root / "northway.sqlite"
    output = root / "client.key"
    env = dict(os.environ, NORTHWAY_DATABASE_PATH=str(database))

    def run(*args, success=True):
        result = subprocess.run([str(binary), *args], env=env, capture_output=True, timeout=10)
        if (result.returncode == 0) != success:
            raise AssertionError("unexpected operator command exit status")
        return result

    run("migrate")
    run("tenant", "create", "--tenant", tenant)
    result = run("key", "create", "--tenant", tenant, "--scopes", "feeds:read", "--output", str(output))
    secret = output.read_bytes().strip()
    assert len(secret) == 80 and secret not in result.stdout + result.stderr
    assert stat.S_IMODE(output.stat().st_mode) == 0o600
    key_id = json.loads(result.stdout)["key_id"]
    refused = run("key", "create", "--tenant", tenant, "--scopes", "feeds:read", "--output", str(output), success=False)
    assert output.read_bytes().strip() == secret
    assert secret not in refused.stdout + refused.stderr
    assert json.loads(refused.stderr)["level"] == "ERROR"
    run("tenant", "create", "--tenant", tenant)
    run("key", "revoke", "--tenant", tenant, "--key-id", key_id)
    run("key", "revoke", "--tenant", tenant, "--key-id", key_id)

print("PASS: operator tenant/key provisioning, private one-time handoff, no overwrite, redacted errors and revocation")

"""Run the pinned, checksum-verified sqlc release; check without editing bindings."""
import hashlib
import io
import pathlib
import platform
import shutil
import subprocess
import sys
import tarfile
import tempfile
import urllib.request

VERSION = "1.31.1"
CHECKSUMS = {
    "x86_64": ("amd64", "497ae4fcdfa64c5b0c311ffe4c2bd991e43991e82e5367792ed78bc2dca27354"),
    "aarch64": ("arm64", "b7cae247740d0c51a1e657479e5b2d21e6fef428f596682a01bc55bf4ab8a23d"),
}
if platform.system() != "Linux" or platform.machine() not in CHECKSUMS:
    raise SystemExit("sqlc harness requires Linux amd64/arm64 (use WSL on Windows)")
arch, digest = CHECKSUMS[platform.machine()]
binary = pathlib.Path(f"bin/tools/sqlc-{VERSION}-{arch}").resolve()
if not binary.exists():
    asset = f"sqlc_{VERSION}_linux_{arch}.tar.gz"
    with urllib.request.urlopen(f"https://github.com/sqlc-dev/sqlc/releases/download/v{VERSION}/{asset}", timeout=60) as response:
        archive = response.read(50 * 1024 * 1024 + 1)
    if hashlib.sha256(archive).hexdigest() != digest:
        raise SystemExit("sqlc archive checksum mismatch")
    with tarfile.open(fileobj=io.BytesIO(archive)) as tar:
        member = tar.getmember("sqlc")
        if not member.isfile():
            raise SystemExit("unexpected sqlc archive member")
        binary.parent.mkdir(parents=True, exist_ok=True)
        binary.write_bytes(tar.extractfile(member).read())
        binary.chmod(0o755)
if subprocess.check_output([str(binary), "version"], text=True).strip() != "v" + VERSION:
    raise SystemExit("cached sqlc version mismatch")
if sys.argv[1:] == ["generate"]:
    subprocess.run([str(binary), "generate"], check=True)
elif sys.argv[1:] == ["check"]:
    with tempfile.TemporaryDirectory(prefix="northway-sqlc-") as tmp:
        root = pathlib.Path.cwd()
        config = (root / "sqlc.yaml").read_text()
        shutil.copytree(root / "db", pathlib.Path(tmp) / "db")
        config_path = pathlib.Path(tmp) / "sqlc.yaml"
        config_path.write_text(config)
        subprocess.run([str(binary), "generate", "-f", str(config_path)], check=True)
        def contents(path):
            return {p.name: p.read_bytes() for p in path.glob("*.go")}
        if contents(root / "internal/sqlite/sqlc") != contents(pathlib.Path(tmp) / "internal/sqlite/sqlc"):
            raise SystemExit("generated SQL drift; run make generate and review the diff")
    print("PASS: pinned sqlc regeneration matches committed bindings")
else:
    raise SystemExit("usage: python3 scripts/sqlc.py generate|check")

"""Use a disposable Docker volume; never mount host data or deploy."""
import json
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request
import uuid

image, revision = sys.argv[1:]
def docker(*args):
    return subprocess.check_output(["docker", *args], text=True).strip()

volume = "northway-smoke-" + uuid.uuid4().hex
container = None
created = False
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
limits = ["--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=128m", "--cpus=1"]
mount = ["--mount", f"type=volume,source={volume},target=/data"]
try:
    assert docker("volume", "create", volume) == volume
    created = True
    docker("run", "--rm", *limits, *mount, image, "migrate", "--database=/data/northway.sqlite")
    container = docker("run", "--detach", *limits, *mount, "--publish", "127.0.0.1::8080", image, "serve", "--database=/data/northway.sqlite")
    assert re.fullmatch(r"[0-9a-f]{64}", container)
    for run in range(2):
        if run:
            docker("start", container)
        info = json.loads(docker("inspect", container))[0]
        assert info["Config"]["User"] == "65532:65532"
        assert info["Config"]["Labels"]["org.opencontainers.image.revision"] == revision
        assert json.loads(docker("exec", container, "/northway", "version"))["revision"] == revision
        assert info["HostConfig"]["ReadonlyRootfs"]
        binding = info["NetworkSettings"]["Ports"]["8080/tcp"][0]
        assert binding["HostIp"] == "127.0.0.1"
        url = "http://127.0.0.1:" + binding["HostPort"] + "/readyz"
        deadline = time.monotonic() + 10
        while True:
            try:
                with opener.open(url, timeout=1) as response:
                    assert response.status == 200 and json.load(response)["status"] == "ready"
                break
            except (urllib.error.URLError, TimeoutError):
                if time.monotonic() >= deadline:
                    raise
                time.sleep(0.1)
        docker("stop", "--time", "12", container)
        state = json.loads(docker("inspect", container))[0]["State"]
        assert state["ExitCode"] == 0 and not state["OOMKilled"], state
    print("PASS: non-root migration, storage volume, readiness and restart at 128 MiB smoke limit")
finally:
    if container and re.fullmatch(r"[0-9a-f]{64}", container):
        subprocess.run(["docker", "rm", "--force", container], check=True, stdout=subprocess.DEVNULL)
    if created:
        subprocess.run(["docker", "volume", "rm", volume], check=True, stdout=subprocess.DEVNULL)

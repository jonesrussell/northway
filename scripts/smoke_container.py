"""Smoke an explicitly named local image; clean up only the container we create."""
import json
import re
import subprocess
import sys
import time
import urllib.error
import urllib.request

image = sys.argv[1]
def docker(*args):
    return subprocess.check_output(["docker", *args], text=True).strip()

container = docker("run", "--detach", "--read-only", "--cap-drop=ALL", "--security-opt=no-new-privileges", "--pids-limit=64", "--memory=128m", "--cpus=1", "--publish", "127.0.0.1::8080", image)
if not re.fullmatch(r"[0-9a-f]{64}", container):
    raise RuntimeError("Docker did not return a container ID")
opener = urllib.request.build_opener(urllib.request.ProxyHandler({}))
try:
    info = json.loads(docker("inspect", container))[0]
    assert info["Config"]["User"] == "65532:65532", "container runs with unexpected identity"
    assert info["HostConfig"]["ReadonlyRootfs"], "root filesystem is writable"
    binding = info["NetworkSettings"]["Ports"]["8080/tcp"][0]
    assert binding["HostIp"] == "127.0.0.1", "smoke port is not private"
    base = "http://127.0.0.1:" + binding["HostPort"]
    deadline = time.monotonic() + 10
    while True:
        try:
            with opener.open(base + "/healthz", timeout=1) as response:
                assert response.status == 200
                assert json.load(response)["status"] == "alive"
            break
        except (urllib.error.URLError, TimeoutError):
            if time.monotonic() >= deadline:
                raise
            time.sleep(0.1)
    try:
        response = opener.open(base + "/readyz", timeout=2)
    except urllib.error.HTTPError as error:
        response = error
    with response:
        assert response.status == 503, "foundation must not claim application readiness"
        assert json.load(response)["status"] == "not_ready"
    docker("stop", "--time", "3", container)
    state = json.loads(docker("inspect", container))[0]["State"]
    assert state["ExitCode"] == 0 and not state["OOMKilled"], state
    print("PASS: non-root/read-only/capability-dropped container; health, fail-closed readiness, clean stop")
finally:
    subprocess.run(["docker", "rm", "--force", container], check=True, stdout=subprocess.DEVNULL)

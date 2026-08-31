"""Validate real authenticated HTTP responses from real SQLite against schemas."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
from jsonschema import Draft202012Validator, FormatChecker
# jsonschema without its format extras silently skips date-time validation.
# Fail before running fixtures if the developer environment is incomplete.
format_probe=Draft202012Validator({"type":"string","format":"date-time"},format_checker=FormatChecker())
if format_probe.is_valid("yesterday"):
    raise SystemExit("date-time validation unavailable; install requirements-dev.txt including jsonschema[format]")
root=Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix="northway-http-") as directory:
    path=Path(directory)/"responses.json"
    env=dict(os.environ,NORTHWAY_TEST_HTTP_RESPONSES=str(path))
    subprocess.run(["go","test","-count=1","-timeout=60s","./internal/app","-run","^TestHTTPContracts$"],cwd=root,env=env,check=True)
    responses=json.loads(path.read_text(encoding="utf-8"))
    validators={name:Draft202012Validator(json.loads((root/f"api/schemas/{name}.schema.json").read_text(encoding="utf-8")),format_checker=FormatChecker()) for name in ["feed-snapshot","problem"]}
    for response in responses:
        validators["problem" if "code" in response else "feed-snapshot"].validate(response)
    assert len(responses)>=15 and any(r.get("items")==[] for r in responses)
print(f"PASS: {len(responses)} real authenticated HTTP responses satisfy snapshot/problem schemas")

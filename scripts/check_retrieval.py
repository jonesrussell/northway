"""Validate a real service/SQLite response against the unchanged draft schema."""
import json
import os
from pathlib import Path
import subprocess
import tempfile
from jsonschema import Draft202012Validator, FormatChecker

# Incomplete optional format dependencies must not weaken standalone checks.
for name,invalid in [("date-time","yesterday"),("uri","not a uri"),("uuid","not-a-uuid")]:
    probe=Draft202012Validator({"type":"string","format":name},format_checker=FormatChecker())
    if probe.is_valid(invalid):
        raise SystemExit(f"{name} validation unavailable; install requirements-dev.txt including jsonschema[format]")
root=Path(__file__).resolve().parents[1]
with tempfile.TemporaryDirectory(prefix="northway-response-") as directory:
    path=Path(directory)/"response.json"
    env=dict(os.environ,NORTHWAY_TEST_RESPONSE=str(path))
    subprocess.run(["go","test","-count=1","-timeout=60s","./internal/sqlite","-run","^TestRetrievalResponseContract$"],cwd=root,env=env,check=True)
    result=json.loads(path.read_text(encoding="utf-8"))
    schema=json.loads((root/"api/schemas/feed-snapshot.schema.json").read_text(encoding="utf-8"))
    Draft202012Validator(schema,format_checker=FormatChecker()).validate(result)
    assert len(result["items"])==20
    assert all(i["published_at"] is None and i["evidence"][0]["basis"]=="source_metadata" for i in result["items"])
print("PASS: real SQLite/retrieval response satisfies snapshot schema at maximum item/title/source/link bounds")

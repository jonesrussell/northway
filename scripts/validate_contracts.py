"""Validate draft contract fixtures and local file links; no runtime claims."""
from copy import deepcopy
import json
from pathlib import Path
import re
from urllib.parse import unquote, urlsplit

from jsonschema import Draft202012Validator, FormatChecker

ROOT = Path(__file__).resolve().parents[1]
validators = {}
examples = {}
for path in sorted((ROOT / "api/schemas").glob("*.schema.json")):
    name = path.name.removesuffix(".schema.json")
    schema = json.loads(path.read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    example = json.loads((ROOT / "api/examples" / f"{name}.json").read_text(encoding="utf-8"))
    validator.validate(example)
    validators[name], examples[name] = validator, example

cases = []
def reject(name, mutate):
    value = deepcopy(examples[name])
    mutate(value)
    assert not validators[name].is_valid(value), f"accepted invalid {name}: {value}"
    cases.append(name)

reject("feed-query", lambda x: x.update(tenant_id="other"))
reject("feed-query", lambda x: x.update(limit=21))
reject("feed-query", lambda x: x.update(limit=0))
reject("feed-query", lambda x: x.update(feed_id="not-a-uuid"))
reject("feed-query", lambda x: x["context"].update(secret="not-allowed"))
reject("feed-query", lambda x: x["context"].update(intent="x" * 2001))
reject("feed-snapshot", lambda x: x["items"][0].update(evidence=[]))
reject("feed-snapshot", lambda x: x["items"][0].update(url="javascript:alert(1)"))
reject("feed-snapshot", lambda x: x.update(generated_at="yesterday"))
reject("feed-snapshot", lambda x: x["ranking"].update(mode="unvalidated"))
reject("feedback", lambda x: x.update(action="undo"))
reject("feedback", lambda x: x.update(reverses_event_id=x["event_id"]))
reject("problem", lambda x: x.update(code="internal_secret"))
undo = deepcopy(examples["feedback"])
undo.update(action="undo", reverses_event_id=undo["event_id"])
validators["feedback"].validate(undo)  # shape only; server rejects self-reversal

links = 0
for path in ROOT.rglob("*.md"):
    if ".git" in path.parts:
        continue
    for raw in re.findall(r"\[[^\]]*\]\(([^)]+)\)", path.read_text(encoding="utf-8")):
        target = raw.strip("<>").split(' "', 1)[0]
        parsed = urlsplit(target)
        if parsed.scheme or target.startswith("#"):
            continue
        local = (path.parent / unquote(parsed.path)).resolve()
        assert local.is_relative_to(ROOT), f"link escapes repo: {path}: {target}"
        assert local.exists(), f"broken file link: {path}: {target}"
        links += 1

print(f"PASS: {len(examples)} schema fixtures, {len(cases)} rejected cases, undo shape, {links} local file links")
print("These checks do not establish runtime authorization, evidence truth, or service readiness.")

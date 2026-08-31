"""Offline proposal/fixture checks only. Never fetch URLs or activate sources."""
from copy import deepcopy
import json
from pathlib import Path
import re
from urllib.parse import urlsplit
import xml.etree.ElementTree as ET

from jsonschema import Draft202012Validator, FormatChecker
from jsonschema.exceptions import ValidationError

ROOT = Path(__file__).resolve().parents[1]


def check_proposal(value, validator):
    validator.validate(value)
    ids, urls = set(), set()
    for source in value["sources"]:
        if source["id"] in ids or source["feed_url"] in urls:
            raise ValueError("duplicate source identity or feed URL")
        ids.add(source["id"])
        urls.add(source["feed_url"])
        for raw in [source["feed_url"], source["publisher_evidence_url"],
                    source["rights_review"]["reference_url"]]:
            url = urlsplit(raw)
            if (url.scheme != "https" or not url.hostname or url.username is not None
                    or url.password is not None or url.port not in (None, 443)
                    or url.query or url.fragment or any(ord(c) < 33 for c in raw)):
                raise ValueError("proposal URL must be plain HTTPS without credentials")
    # This is NOT a runtime SSRF guard, DNS check, or publisher-rights decision.


def check_fixtures():
    folder = ROOT / "testdata/feeds"
    roots = {}
    for name in ["atom-initial.xml", "atom-updated.xml", "rss-initial.xml"]:
        data = (folder / name).read_bytes()
        if len(data) > 16384 or b"<!DOCTYPE" in data.upper() or b"<!ENTITY" in data.upper():
            raise ValueError("fixtures must be small, authored XML without DTD/entities")
        roots[name] = ET.fromstring(data)
        for node in roots[name].iter():
            values = list(node.attrib.values()) + [node.text or ""]
            for text in values:
                # Includes escaped HTML in Atom summaries; no request is made.
                for raw in re.findall(r'https?://[^\s<>"\']+', text):
                    if not (urlsplit(raw).hostname or "").endswith(".invalid"):
                        raise ValueError("fixture contains a non-reserved network URL")
    ns = {"a": "http://www.w3.org/2005/Atom"}
    initial = roots["atom-initial.xml"].findall("a:entry", ns)
    updated = roots["atom-updated.xml"].findall("a:entry", ns)
    assert len(initial) == len(updated) == 2
    initial_ids = [x.findtext("a:id", namespaces=ns) for x in initial]
    updated_ids = [x.findtext("a:id", namespaces=ns) for x in updated]
    assert initial_ids == updated_ids and len(set(initial_ids)) == 2 and all(initial_ids)
    assert initial[0].findtext("a:title", namespaces=ns) != updated[0].findtext("a:title", namespaces=ns)
    assert initial[1].find("a:published", ns) is None
    assert ET.tostring(initial[1]) == ET.tostring(updated[1])
    rss = roots["rss-initial.xml"].findall("channel/item")
    assert len(rss) == 1 and rss[0].findtext("guid")
    assert rss[0].find("enclosure") is not None
    try:
        ET.fromstring((folder / "malformed.xml").read_bytes())
    except ET.ParseError:
        pass
    else:
        raise AssertionError("malformed fixture unexpectedly parses")


def main():
    schema = json.loads((ROOT / "catalogue/proposal.schema.json").read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    proposal = json.loads((ROOT / "catalogue/php-pilot.json").read_text(encoding="utf-8"))
    check_proposal(proposal, validator)
    mutations = [
        lambda x: x.update(enabled=True),
        lambda x: x.update(provider_export_allowed=True),
        lambda x: x.update(commercial_use_approved=True),
        lambda x: x.update(acquisition_location="pi"),
        lambda x: x["sources"][0].update(enabled=True),
        lambda x: x["sources"][0]["rights_review"].update(status="approved"),
        lambda x: x["sources"][0]["availability"].update(xml_parsed=True),
        lambda x: x["sources"][0].update(feed_url="http://example.invalid/feed"),
        lambda x: x["sources"][0].update(feed_url="https://user:password@example.invalid/feed"),
        lambda x: x["sources"][1].update(id=x["sources"][0]["id"]),
        lambda x: x["sources"][1].update(feed_url=x["sources"][0]["feed_url"]),
        lambda x: x["poll_policy"].update(follow_article_links=True),
        lambda x: x["proposed_retained_fields"].append("body"),
        lambda x: x.update(secret="synthetic-forbidden-field"),
    ]
    for mutate in mutations:
        bad = deepcopy(proposal)
        mutate(bad)
        try:
            check_proposal(bad, validator)
        except (ValueError, ValidationError):
            continue
        raise AssertionError("unsafe proposal mutation passed")
    check_fixtures()
    print(f"PASS: {len(proposal['sources'])} disabled source proposals, {len(mutations)} rejections, 4 synthetic XML fixtures; no network")


if __name__ == "__main__":
    main()

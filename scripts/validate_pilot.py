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


def check_fixture_urls(text):
    for raw in re.findall(r'https?://[^\s<>"\']+', text, flags=re.IGNORECASE):
        if not (urlsplit(raw).hostname or "").endswith(".invalid"):
            raise ValueError("fixture contains a non-reserved network URL")


def screen_fixture(data):
    if len(data) > 16384:
        raise ValueError("fixture exceeds the authored-input size limit")
    text = data.decode("utf-8")
    if "\x00" in text or "<!DOCTYPE" in text.upper() or "<!ENTITY" in text.upper():
        raise ValueError("fixtures must be UTF-8 XML without NUL/DTD/entities")
    # Atom's fixed namespace is an identifier, not a fetch destination. Only
    # remove namespace bindings; the same URL in content must still be rejected.
    network_text = re.sub(
        r"""\bxmlns(?::[A-Za-z_][\w.-]*)?\s*=\s*(['"])http://www\.w3\.org/2005/Atom\1""",
        "", text)
    check_fixture_urls(network_text)
    return text


def check_fixtures():
    folder = ROOT / "testdata/feeds"
    roots = {}
    for name in ["atom-initial.xml", "atom-updated.xml", "rss-initial.xml", "malformed.xml"]:
        text = screen_fixture((folder / name).read_bytes())
        try:
            root = ET.fromstring(text)
        except ET.ParseError:
            if name == "malformed.xml":
                continue
            raise
        if name == "malformed.xml":
            raise AssertionError("malformed fixture unexpectedly parses")
        roots[name] = root
        for node in roots[name].iter():
            values = list(node.attrib.values()) + [node.text or "", node.tail or ""]
            for text in values:
                # Also inspect decoded character references/escaped markup.
                check_fixture_urls(text)
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


def check_fixture_rejections():
    cases = [
        b"x" * 16385,
        b'<!DOCTYPE rss [<!ENTITY x "expanded">]><rss>&x;</rss>',
        b"<rss><item/>HTTPS://example.com/</rss>",
        b"<rss><item>https://example.com/truncated",
        '<!DOCTYPE rss [<!ENTITY x "expanded">]><rss/>'.encode("utf-16"),
        '<!DOCTYPE rss [<!ENTITY x "expanded">]><rss/>'.encode("utf-16-le"),
    ]
    for data in cases:
        try:
            screen_fixture(data)
        except ValueError:
            continue
        raise AssertionError("unsafe fixture bytes passed the pre-parse screen")
    return len(cases)


def main():
    schema = json.loads((ROOT / "catalogue/proposal.schema.json").read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    proposal = json.loads((ROOT / "catalogue/php-pilot.json").read_text(encoding="utf-8"))
    check_proposal(proposal, validator)
    mutations = [
        lambda x: x.update(enabled=True),
        lambda x: x["sources"].append(dict(deepcopy(x["sources"][0]), id="unapproved-source", feed_url="https://example.invalid/feed")),
        lambda x: x["sources"][0].update(id="unapproved-source", feed_url="https://example.invalid/feed"),
        lambda x: x["sources"][0].update(feed_url=x["sources"][1]["feed_url"]),
        lambda x: x["sources"][3]["availability"].update(conditional_get_verified=True),
        lambda x: x.update(provider_export_allowed=True),
        lambda x: x.update(commercial_use_approved=True),
        lambda x: x.update(acquisition_location="external_unapproved"),
        lambda x: x["sources"][0].update(enabled=True),
        lambda x: x["sources"][0]["rights_review"].update(status="approved"),
        lambda x: x["sources"][0]["availability"].update(conditional_get_verified=False),
        lambda x: x["sources"][0]["availability"].update(decoded_bytes=2097153),
        lambda x: x["owner_approval"].update(deployment_authorized=True),
        lambda x: x["owner_approval"].update(scheduled_polling_enabled=True),
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
    fixture_rejections = check_fixture_rejections()
    print(f"PASS: {len(proposal['sources'])} disabled source proposals, {len(mutations)} proposal rejections, 4 synthetic XML fixtures, {fixture_rejections} fixture rejections; no network")


if __name__ == "__main__":
    main()

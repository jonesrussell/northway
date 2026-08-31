"""Offline candidate and synthetic-input checks; never fetch or activate a source."""
from collections import Counter
from copy import deepcopy
import json
from pathlib import Path
from urllib.parse import urlsplit
import xml.etree.ElementTree as ET

from jsonschema import Draft202012Validator, FormatChecker
from jsonschema.exceptions import ValidationError

from validate_pilot import screen_fixture, check_fixture_urls

ROOT = Path(__file__).resolve().parents[1]


def validate(value, validator, bootstrap):
    validator.validate(value)
    ids, urls = set(), set()
    for source in value["sources"]:
        if source["id"] in ids or source["feed_url"] in urls:
            raise ValueError("duplicate source identity")
        ids.add(source["id"])
        urls.add(source["feed_url"])
        for raw in [source["feed_url"], source["publisher_evidence_url"],
                    source["rights_review"]["reference_url"]]:
            u = urlsplit(raw)
            if (u.scheme != "https" or not u.hostname or u.username is not None
                    or u.password is not None or u.port not in (None, 443)
                    or u.query or u.fragment or any(ord(c) < 33 for c in raw)):
                raise ValueError("candidate URL must be plain HTTPS")
        observed = source["availability"]
        parsed = observed["outcome"] == "feed_parsed"
        if parsed:
            if (observed["http_status"] != 200 or observed["entry_count"] is None
                    or observed["decoded_bytes"] is None):
                raise ValueError("parsed outcome requires response evidence")
        elif observed["entry_count"] is not None or observed["conditional_http_status"] is not None:
            raise ValueError("unparsed source cannot claim parsed entries/conditional result")
        if observed["outcome"] == "not_probed":
            if (observed["vantage"] != "not_probed"
                    or observed["http_status"] is not None
                    or observed["decoded_bytes"] is not None):
                raise ValueError("unprobed source cannot claim network evidence")
        elif observed["vantage"] != "workstation_wsl":
            raise ValueError("probe vantage must be explicit")
        if observed["outcome"] == "timeout" and observed["http_status"] is not None:
            raise ValueError("this timeout record has no HTTP response")
        if observed["outcome"] == "http_refused" and observed["http_status"] != 403:
            raise ValueError("refusal record must preserve observed 403")
    areas = Counter(s["interest_area"] for s in value["sources"])
    if areas != Counter({x: 2 for x in value["interest_areas"]}):
        raise ValueError("candidate set must cover all five interests")
    policy = value["proposed_policy"]
    bootstrap_attempts = len(bootstrap["sources"]) * 86400 // bootstrap["poll_policy"]["interval_seconds"]
    if bootstrap_attempts != policy["bootstrap_attempts_per_day"]:
        raise ValueError("bootstrap budget reference drift")
    if bootstrap_attempts + len(ids) * 86400 // policy["new_source_interval_seconds"] > policy["max_total_attempts_per_day"]:
        raise ValueError("combined poll plan exceeds total attempt budget")
    # URL shape and recorded observations are not SSRF protection or rights clearance.


def main():
    schema = json.loads((ROOT / "catalogue/personal.schema.json").read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    value = json.loads((ROOT / "catalogue/personal-pilot.json").read_text(encoding="utf-8"))
    bootstrap = json.loads((ROOT / "catalogue/php-pilot.json").read_text(encoding="utf-8"))
    validate(value, validator, bootstrap)
    mutations = [
        lambda x: x.update(enabled=True),
        lambda x: x.update(provider_export_allowed=True),
        lambda x: x.update(commercial_use_approved=True),
        lambda x: x.update(source_selection_status="approved"),
        lambda x: x["sources"][0].update(enabled=True),
        lambda x: x["sources"][0].update(operator_approved=True),
        lambda x: x["sources"][0].update(feed_url="https://other.invalid/feed"),
        lambda x: x["sources"].append(deepcopy(x["sources"][0])),
        lambda x: x["sources"].__setitem__(1, deepcopy(x["sources"][0])),
        lambda x: x["sources"][0].update(interest_area="world"),
        lambda x: x["sources"][0]["rights_review"].update(status="approved"),
        lambda x: x["sources"][7]["rights_review"].update(status="review_pending"),
        lambda x: x["sources"][8]["rights_review"].update(status="review_pending"),
        lambda x: x["sources"][0]["availability"].update(pi_reachability_verified=True),
        lambda x: x["sources"][0]["availability"].update(http_status=403),
        lambda x: x["sources"][7]["availability"].update(http_status=200),
        lambda x: x["sources"][6]["availability"].update(conditional_http_status=304),
        lambda x: x["retained_fields"].append("body"),
        lambda x: x["proposed_policy"].update(max_total_attempts_per_day=181),
        lambda x: x["proposed_policy"].update(max_total_decoded_bytes_per_day=134217728),
        lambda x: x["proposed_policy"].update(automatic_retries=1),
        lambda x: x.update(secret="synthetic-forbidden-field"),
    ]
    for mutation in mutations:
        invalid = deepcopy(value)
        mutation(invalid)
        try:
            validate(invalid, validator, bootstrap)
        except (ValueError, ValidationError):
            continue
        raise AssertionError("unsafe candidate mutation passed")

    queries = json.loads((ROOT / "testdata/personal/queries.json").read_text(encoding="utf-8"))
    query_schema = json.loads((ROOT / "api/schemas/feed-query.schema.json").read_text(encoding="utf-8"))
    query_validator = Draft202012Validator(query_schema, format_checker=FormatChecker())
    assert queries["synthetic"] is True and len(queries["cases"]) == 5
    assert {x["interest_area"] for x in queries["cases"]} == set(value["interest_areas"])
    for case in queries["cases"]:
        query_validator.validate(case["request"])
        assert case["request"]["context"]["technologies"] == []
        assert case["request"]["limit"] == 5
    text = screen_fixture((ROOT / "testdata/personal/topics.xml").read_bytes())
    root = ET.fromstring(text)
    for node in root.iter():
        for text in list(node.attrib.values()) + [node.text or "", node.tail or ""]:
            check_fixture_urls(text)
    items = root.findall("channel/item")
    assert len(items) == 5 and len({x.findtext("guid") for x in items}) == 5
    assert {x.findtext("category") for x in items} == set(value["interest_areas"])
    assert sum(x.find("pubDate") is None for x in items) == 1
    print(f"PASS: 10 disabled candidates, {len(mutations)} rejected changes, 5 no-project query shapes, synthetic RSS; no network")


if __name__ == "__main__":
    main()

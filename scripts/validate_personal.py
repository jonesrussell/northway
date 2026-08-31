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



def validate_active(selection, active, validator):
    validator.validate(active)
    selected = {source["id"]: source for source in selection["sources"]}
    catalogue_by_url = {source["feed_url"]: source for source in selection["sources"]}
    active_by_url = {source["feed_url"]: source for source in active["sources"]}
    if len(active_by_url) != len(active["sources"]) or not set(active_by_url).issubset(catalogue_by_url):
        raise ValueError("active profile escaped owner-selected endpoints")
    feeds_by_id = {feed["id"]: feed for feed in active["feeds"]}
    for url, source in active_by_url.items():
        candidate = catalogue_by_url[url]
        if (source["title"] != candidate["name"]
                or source["publisher_group"] != candidate["publisher_group"]
                or source["categories"] != [candidate["interest_area"]]
                or not candidate["operator_approved"]):
            raise ValueError("active source identity is not owner-selected")
        if candidate["rights_review"]["status"] == "permission_required":
            raise ValueError("active profile includes a permission-held source")
        if candidate["availability"]["outcome"] in {"http_refused", "not_probed", "timeout"}:
            raise ValueError("active profile includes an unavailable or unprobed source")
    active_catalogue_ids = {catalogue_by_url[url]["id"] for url in active_by_url}
    excluded_entries = {entry["catalogue_id"]: entry["reason"] for entry in active["excluded"]}
    if (active_catalogue_ids & set(excluded_entries)
            or active_catalogue_ids | set(excluded_entries) != set(selected)):
        raise ValueError("active and excluded sources must partition the exact selection")
    expected_reasons = {
        "cbc-canada": "repeated_transport_timeout_from_workstation",
        "variety": "publisher_terms_restrict_automated_and_ai_use",
        "anishinabek-news": "feed_returned_403_and_permission_unconfirmed",
        "aptn-news": "publisher_terms_require_prior_consent_for_automated_collection",
        "bbc-world": "current_metadata_analysis_permission_unresolved",
    }
    if excluded_entries != expected_reasons:
        raise ValueError("active-profile exclusion rationale drift")
    feed_ids = set(feeds_by_id)
    if len(feed_ids) != len(active["feeds"]):
        raise ValueError("duplicate runnable feed identity")
    for source in active["sources"]:
        if set(source["feed_ids"]) - feed_ids:
            raise ValueError("active source references unknown saved feed")
        if any(not set(source["categories"]).issubset(feeds_by_id[feed_id]["categories"])
               for feed_id in source["feed_ids"]):
            raise ValueError("active source category escaped saved feed policy")
    activation = (ROOT / active["activation_record"]).read_text(encoding="utf-8")
    rows = [tuple(cell.strip() for cell in line.split("|")[1:-1])
            for line in activation.splitlines() if line.startswith("| ") and "https://" in line]
    approved = {(row[1], row[2]) for row in rows}
    expected = {(source["title"], source["feed_url"]) for source in active["sources"]}
    if approved != expected:
        raise ValueError("activation record/runnable source drift")

def main():
    schema = json.loads((ROOT / "catalogue/personal.schema.json").read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(schema)
    validator = Draft202012Validator(schema, format_checker=FormatChecker())
    value = json.loads((ROOT / "catalogue/personal-pilot.json").read_text(encoding="utf-8"))
    bootstrap = json.loads((ROOT / "catalogue/php-pilot.json").read_text(encoding="utf-8"))
    validate(value, validator, bootstrap)
    active_schema = json.loads((ROOT / "catalogue/personal-local-v1.schema.json").read_text(encoding="utf-8"))
    Draft202012Validator.check_schema(active_schema)
    active = json.loads((ROOT / "catalogue/personal-local-v1.json").read_text(encoding="utf-8"))
    active_validator = Draft202012Validator(active_schema, format_checker=FormatChecker())
    validate_active(value, active, active_validator)
    def swap_active(target_id, replaced_id):
        changed = deepcopy(active)
        candidate = next(source for source in value["sources"] if source["id"] == target_id)
        index = next(i for i, source in enumerate(changed["sources"])
                     if next(item for item in value["sources"]
                             if item["feed_url"] == source["feed_url"])["id"] == replaced_id)
        replaced = changed["sources"][index]
        changed["sources"][index] = {
            "id": "ffffffff-ffff-4fff-8fff-ffffffffffff",
            "title": candidate["name"], "feed_url": candidate["feed_url"],
            "feed_ids": replaced["feed_ids"], "interval_seconds": 14400,
            "max_bytes": 2097152, "personal_use_basis": "owner_directed_official_feed",
            "publisher_group": candidate["publisher_group"],
            "categories": [candidate["interest_area"]],
        }
        for entry in changed["excluded"]:
            if entry["catalogue_id"] == target_id:
                entry["catalogue_id"] = replaced_id
                entry["reason"] = "publisher_terms_unverified"
        return changed
    active_invalid = [swap_active("variety", "global-entertainment"),
                      swap_active("cbc-canada", "global-canada"), deepcopy(active)]
    active_invalid[-1]["excluded"][0]["reason"] = "temporarily_unavailable"
    for invalid in active_invalid:
        try:
            validate_active(value, invalid, active_validator)
        except (ValueError, ValidationError):
            continue
        raise AssertionError("unsafe active-profile mutation passed")
    # Keep the human approval record aligned with the schema-pinned selection.
    record = (ROOT / value["owner_approval"]["record"]).read_text(encoding="utf-8")
    rows = [tuple(cell.strip() for cell in line.split("|")[1:-1])
            for line in record.splitlines() if line.startswith("| ") and "https://" in line]
    expected_rows = [(source["interest_area"], source["name"], source["feed_url"])
                     for source in value["sources"]]
    assert sorted(rows) == sorted(expected_rows), "approval record/source selection drift"
    mutations = [
        lambda x: x.update(enabled=True),
        lambda x: x.update(schema_version=1),
        lambda x: x.pop("owner_approval"),
        lambda x: x["owner_approval"].update(scope="collection_and_provider_export"),
        lambda x: x.update(source_selection_status="candidates_pending_individual_approval"),
        lambda x: x["sources"][2]["rights_review"].update(status="review_pending"),
        lambda x: x.update(provider_export_allowed=True),
        lambda x: x.update(commercial_use_approved=True),
        lambda x: x.update(source_selection_status="approved"),
        lambda x: x["sources"][0].update(enabled=True),
        lambda x: x["sources"][0].update(operator_approved=False),
        lambda x: x["sources"][0].update(feed_url="https://other.invalid/feed"),
        lambda x: x["sources"].append(deepcopy(x["sources"][0])),
        lambda x: x["sources"].__setitem__(1, deepcopy(x["sources"][0])),
        lambda x: x["sources"][0].update(interest_area="world"),
        lambda x: x["sources"][0]["rights_review"].update(status="approved"),
        lambda x: x["sources"][0]["rights_review"].update(status="review_pending"),
        lambda x: x["sources"][2]["rights_review"].update(reference_url="https://other.invalid/terms"),
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
    print(f"PASS: 10 owner-selected catalogue sources, {len(mutations)} rejected changes, 5 no-project query shapes, runnable 5-source profile with 3 rejected activations, synthetic RSS; no network")


if __name__ == "__main__":
    main()

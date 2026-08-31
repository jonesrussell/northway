"""Check the actual Go import graph, including tests. Unknown layers fail closed."""
import json
import subprocess
import sys

MODULE = "github.com/jonesrussell/northway"
FEATURES = {"identity", "source", "article", "feed", "ingest", "query", "ranking", "feedback", "usage", "schedule"}
ALLOWED = {
    "db": set(),
    "app": FEATURES | {"httpapi", "sqlite", "fetch", "providers"},
    "httpapi": FEATURES,
    "identity": set(),
    "source": {"identity"},
    "article": {"identity", "source"},
    "feed": {"identity", "source", "article"},
    "ingest": {"identity", "source", "article", "usage"},
    "query": {"identity", "source", "article", "feed", "ranking", "usage"},
    "ranking": {"identity", "article"},
    "feedback": {"identity", "article", "feed", "usage"},
    "usage": {"identity"},
    "schedule": {"identity", "ingest", "query", "usage"},
    "sqlite": FEATURES | {"db"},
    "fetch": {"source", "article", "ingest"},
    "providers": {"ranking", "ingest", "article"},
}
EXTERNAL = {
    "github.com/pressly/goose/v3": {"sqlite"},
    "modernc.org/sqlite": {"sqlite"},
    "github.com/mmcdole/gofeed": {"fetch"},
    "github.com/anthropics/anthropic-sdk-go": {"providers"},
    "golang.org/x/sync": {"app", "ingest", "query", "schedule"},
    "golang.org/x/time": {"fetch", "ingest", "httpapi"},
}

def decode_stream(text):
    decoder = json.JSONDecoder()
    while text.strip():
        text = text.lstrip()
        value, end = decoder.raw_decode(text)
        yield value
        text = text[end:]

def canonical(path):
    return path.split(" ", 1)[0].removesuffix("_test")

def layer(path):
    path = canonical(path)
    if path == MODULE + "/db":
        return "db"
    prefix = MODULE + "/internal/"
    if path.startswith(prefix):
        return path[len(prefix):].split("/", 1)[0]
    if path == MODULE + "/cmd/northway":
        return "cmd"
    return None

def violations(packages):
    errors = []
    standard = {canonical(p["ImportPath"]) for p in packages if p.get("Standard")}
    for package in packages:
        source = canonical(package["ImportPath"])
        if (source != MODULE and not source.startswith(MODULE + "/")) or source.endswith(".test"):
            continue
        source_layer = layer(source)
        if source_layer not in ALLOWED and source_layer != "cmd":
            errors.append(f"unregistered package: {source}")
            continue
        for raw in package.get("Imports", []):
            target = canonical(raw)
            if target.startswith(MODULE + "/internal/sqlite/sqlc") and source_layer != "sqlite":
                errors.append(f"generated SQL outside adapter: {source} -> {target}")
                continue
            if target == "database/sql" or target.startswith("database/sql/"):
                if source_layer != "sqlite":
                    errors.append(f"SQL outside storage adapter: {source} -> {target}")
                continue
            if target in standard:
                continue
            if target.startswith(MODULE + "/"):
                target_layer = layer(target)
                allowed = {"app"} if source_layer == "cmd" else ALLOWED[source_layer] | {source_layer}
                if target_layer not in allowed:
                    errors.append(f"forbidden import: {source} -> {target}")
            else:
                allowed = any((target == prefix or target.startswith(prefix + "/")) and source_layer in layers for prefix, layers in EXTERNAL.items())
                if not allowed:
                    errors.append(f"unapproved external import: {source} -> {target}")
    return errors

if __name__ == "__main__":
    output = subprocess.check_output(["go", "list", "-deps", "-test", "-json", "./..."], text=True)
    packages = list(decode_stream(output))
    errors = violations(packages)
    if errors:
        print("\n".join(errors), file=sys.stderr)
        sys.exit(1)
    print("PASS: Go import boundaries (including test packages)")

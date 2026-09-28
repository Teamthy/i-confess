#!/usr/bin/env python3
"""Generate web/lib/versions.json from server/internal/bible/registry.go.

The reviewed translation registry lives in Go: internal/bible/registry.go is
the source of truth for every edition the platform has reviewed — identity,
coverage, licence, and the exact source file and SHA-256 its text came from.
The web app renders that registry as provenance cards on the translations
page so a reader can see what has been reviewed even when this deployment's
catalogue does not import it.

Parsing the Go source (rather than calling the API) means the file can be
regenerated offline, in CI, without booting the server. server/cmd/
bible-registry writes the same document straight from the Go values; the Go
test (TestWebRegistryJSONIsCurrent) compares the checked-in file by decoded
content, not bytes, so both generators are legitimate.

Usage:
    python3 scripts/gen-bible-registry.py [--check]

Writes web/lib/versions.json: two-space indent, UTF-8, trailing newline,
versions sorted by sort_order.
"""
import json
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
BIBLE_DIR = ROOT / "server" / "internal" / "bible"
REGISTRY_GO = BIBLE_DIR / "registry.go"
OUTPUT = ROOT / "web" / "lib" / "versions.json"


def go_unquote(raw: str) -> str:
    """Decode the inside of a Go quoted string literal."""
    try:
        return json.loads('"' + raw + '"')
    except ValueError:
        return raw.replace('\\"', '"').replace("\\\\", "\\")


def string_const(go_sources: str) -> dict:
    """Map identifier -> string value for simple `Name = "value"` consts.

    Coverage and format are declared as constants (CoverageFull, FormatOSIS,
    PublicDomainURL, …); entries reference the identifier, so the identifiers
    are resolved here rather than hard-coded.
    """
    return dict(re.findall(r'(\w+)\s*=\s*"((?:[^"\\]|\\.)*)"', go_sources))


def versions_block(text: str) -> str:
    """Return the body of `var Versions = []Version{ … }`."""
    start = text.index("var Versions = []Version{")
    open_brace = text.index("{", start + len("var Versions = []Version") - 1)
    depth = 0
    for i in range(open_brace, len(text)):
        if text[i] == "{":
            depth += 1
        elif text[i] == "}":
            depth -= 1
            if depth == 0:
                return text[open_brace + 1 : i]
    raise SystemExit("gen-bible-registry: unbalanced braces in Versions")


def entry_literals(block: str) -> list:
    """Split the block into the individual `{ … }` composite literals."""
    entries = []
    depth = 0
    start = None
    for i, ch in enumerate(block):
        if ch == "{":
            if depth == 0:
                start = i
            depth += 1
        elif ch == "}":
            depth -= 1
            if depth == 0 and start is not None:
                entries.append(block[start + 1 : i])
                start = None
    return entries


def field_str(entry: str, name: str):
    m = re.search(r"\b" + name + r':\s*"((?:[^"\\]|\\.)*)"', entry)
    return go_unquote(m.group(1)) if m else None


def field_ident(entry: str, name: str, consts: dict):
    m = re.search(r"\b" + name + r":\s*(\w+)", entry)
    if not m:
        return None
    ident = m.group(1)
    if ident in consts:
        return consts[ident]
    if ident.isdigit():
        return ident
    raise SystemExit(f"gen-bible-registry: unresolved constant {ident!r} for {name}")


def field_string_list(entry: str, name: str) -> list:
    m = re.search(r"\b" + name + r":\s*\[\]string\{([^}]*)\}", entry)
    if not m:
        return None
    return [go_unquote(s) for s in re.findall(r'"((?:[^"\\]|\\.)*)"', m.group(1))]


def status_overrides(text: str) -> dict:
    """Read `if v.ID == "…" { … v.Status = "…" }` rules out of init()."""
    overrides = {}
    for m in re.finditer(
        r'if\s+v\.ID\s*==\s*"(\w+)"\s*\{([^}]*)\}', text
    ):
        vid, body = m.group(1), m.group(2)
        status = re.search(r'v\.Status\s*=\s*"(\w+)"', body)
        if status:
            overrides[vid] = status.group(1)
    return overrides


def main() -> int:
    text = REGISTRY_GO.read_text(encoding="utf-8")
    # Consts may live outside registry.go (formats do); scan the package.
    consts = {}
    for path in sorted(BIBLE_DIR.glob("*.go")):
        consts.update(string_const(path.read_text(encoding="utf-8")))

    overrides = status_overrides(text)
    versions = []
    for entry in entry_literals(versions_block(text)):
        vid = field_str(entry, "ID")
        if not vid:
            raise SystemExit("gen-bible-registry: entry without ID")
        record = {
            "id": vid,
            "name": field_str(entry, "Name"),
            "abbreviation": field_str(entry, "Abbrev"),
            "language": field_str(entry, "Language"),
            "language_name": field_str(entry, "LanguageName"),
            "coverage": field_ident(entry, "Coverage", consts),
        }
        year = field_str(entry, "Year")
        if year:
            record["year"] = year
        record["licence"] = field_str(entry, "Licence")
        licence_url = field_ident(entry, "LicenceURL", consts)
        if licence_url:
            record["licence_url"] = licence_url
        note = field_str(entry, "LicenceNote")
        if note:
            record["licence_note"] = note
        for list_field in ("OmittedBooks", "OmittedChapters"):
            items = field_string_list(entry, list_field)
            if items:
                key = re.sub(r"(?<!^)(?=[A-Z])", "_", list_field).lower()
                record[key] = items
        record["attribution"] = field_str(entry, "Attribution")
        record["source_file"] = field_str(entry, "SourceFile")
        record["sha256"] = field_str(entry, "SHA256")
        record["bytes"] = int(re.search(r"\bBytes:\s*(\d+)", entry).group(1))
        record["format"] = field_ident(entry, "Format", consts)
        record["sort_order"] = int(re.search(r"\bSortOrder:\s*(\d+)", entry).group(1))
        if re.search(r"\bDefault:\s*true", entry):
            record["default"] = True
        record["status"] = overrides.get(vid, "active")
        versions.append(record)

    versions.sort(key=lambda v: v["sort_order"])
    document = {
        "version_count": len(versions),
        "language_count": len({v["language"] for v in versions}),
        "versions": versions,
    }
    rendered = json.dumps(document, indent=2, ensure_ascii=False) + "\n"

    if "--check" in sys.argv:
        if not OUTPUT.exists() or OUTPUT.read_text(encoding="utf-8") != rendered:
            print(f"gen-bible-registry: {OUTPUT} is stale", file=sys.stderr)
            return 1
        print(f"gen-bible-registry: {OUTPUT} is current")
        return 0

    OUTPUT.parent.mkdir(parents=True, exist_ok=True)
    OUTPUT.write_text(rendered, encoding="utf-8")
    print(
        f"gen-bible-registry: wrote {OUTPUT.relative_to(ROOT)} "
        f"({document['version_count']} versions, {document['language_count']} languages)"
    )
    return 0


if __name__ == "__main__":
    sys.exit(main())

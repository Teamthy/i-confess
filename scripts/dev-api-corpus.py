#!/usr/bin/env python3
"""Extract the seed of record from the Go server into the JSON corpus that
scripts/dev-api.mjs serves.

This exists because the development sandbox cannot run Go: the real server
seeds PostgreSQL from server/internal/seed/*_canonical.go, and the fixture has
to serve the same rows from the same source. Parsing the Go literals here
instead of duplicating content in JS keeps the two from drifting while Go is
unavailable. The assertions below mirror the guarantees
categories_canonical_test.go and confessions_canonical_test.go make of the
source, so a parse that silently mis-slices fails loudly.

Writes /tmp/cats.json and /tmp/confs.json by default.

Usage: python3 scripts/dev-api-corpus.py [--out-dir /tmp]
"""
import argparse
import json
import re
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
SEED = ROOT / "server" / "internal" / "seed"

STR = r'"((?:[^"\\]|\\.)*)"'


def unescape(s: str) -> str:
    return (
        s.replace('\\"', '"')
        .replace("\\n", "\n")
        .replace("\\t", "\t")
        .replace("\\\\", "\\")
    )


def balanced_from(text: str, brace_at: int) -> str:
    """The brace-balanced block starting at text[brace_at] == '{'. Strings are
    skipped so braces inside confession text cannot fool the counter."""
    depth = 0
    i = brace_at
    n = len(text)
    while i < n:
        c = text[i]
        if c == '"':
            i += 1
            while i < n and text[i] != '"':
                i += 2 if text[i] == "\\" else 1
        elif c == "{":
            depth += 1
        elif c == "}":
            depth -= 1
            if depth == 0:
                return text[brace_at : i + 1]
        i += 1
    raise ValueError("unbalanced braces")


def entry_blocks(text: str, header: str) -> list[str]:
    """Every `{Category:` / `\n\t{` entry inside a Go slice literal."""
    out = []
    i = 0
    while True:
        j = text.find(header, i)
        if j == -1:
            return out
        b = text.rfind("{", 0, j + len(header))
        out.append(balanced_from(text, b))
        i = b + 1


def parse_categories() -> list[dict]:
    src = (SEED / "categories_canonical.go").read_text(encoding="utf-8")
    anchor = src.index("var CanonicalCategories")
    out = []
    for b in entry_blocks(src[anchor:], '\n\t{'):
        item = {}
        for key in ("Name", "Slug", "Description", "Tagline", "Icon"):
            m = re.search(key + r":\s*" + STR, b)
            item[key.lower()] = unescape(m.group(1)) if m else ""
        if item.get("name"):
            out.append(item)
    return out


def parse_confessions() -> list[dict]:
    src = (SEED / "confessions_canonical.go").read_text(encoding="utf-8")
    anchor = src.index("var CanonicalConfessions")
    out = []
    for b in entry_blocks(src[anchor:], "{Category:"):
        item = {}
        for key in ("Category", "Title", "Short", "Medium", "Long"):
            m = re.search(key + r":\s*" + STR, b)
            item[key.lower()] = unescape(m.group(1)) if m else ""
        m = re.search(r"Intensity:\s*(\d+)", b)
        item["intensity"] = int(m.group(1)) if m else 3
        refs = []
        for rm in re.finditer(
            r"\{Book:\s*" + STR + r",\s*Chapter:\s*(\d+),\s*Verse:\s*" + STR
            + r",\s*Translation:\s*" + STR + r",\s*IsDirectQuote:\s*(true|false)\}",
            b,
        ):
            refs.append(
                {
                    "book": unescape(rm.group(1)),
                    "chapter": int(rm.group(2)),
                    "verse": unescape(rm.group(3)),
                    "translation": unescape(rm.group(4)),
                    "is_direct_quote": rm.group(5) == "true",
                }
            )
        item["scriptures"] = refs
        if item.get("title"):
            out.append(item)
    return out


def main() -> None:
    ap = argparse.ArgumentParser()
    ap.add_argument("--out-dir", default="/tmp")
    args = ap.parse_args()
    out = Path(args.out_dir)
    cats = parse_categories()
    confs = parse_confessions()
    names = {c["name"] for c in cats}
    slugs = {c["slug"] for c in cats}
    assert len(cats) == 39, f"expected 39 canonical categories, parsed {len(cats)}"
    assert len(names) == 39 and len(slugs) == 39, "names/slugs must be unique (seed test rule 1)"
    assert all(c["description"] and c["tagline"] for c in cats), "empty description/tagline (seed test rule 2)"
    assert len(confs) >= 78, f"expected >=2 confessions per category, parsed {len(confs)}"
    assert all(c["scriptures"] for c in confs), "a canonical confession lost its scripture refs"
    per_cat: dict[str, int] = {}
    for c in confs:
        assert c["category"] in names, f"confession cites unknown category {c['category']!r}"
        assert len(c["short"]) < len(c["medium"]) < len(c["long"]), f"variant lengths not increasing for {c['title']!r}"
        per_cat[c["category"]] = per_cat.get(c["category"], 0) + 1
    titles = [c["title"] for c in confs]
    assert len(titles) == len(set(titles)), "titles must be unique"
    (out / "cats.json").write_text(json.dumps(cats, indent=1), encoding="utf-8")
    (out / "confs.json").write_text(json.dumps(confs, indent=1), encoding="utf-8")
    print(f"wrote {len(cats)} categories and {len(confs)} confessions to {out}")


if __name__ == "__main__":
    main()

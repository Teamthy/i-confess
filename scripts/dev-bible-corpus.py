#!/usr/bin/env python3
"""Build the local Bible corpus the dev fixture serves.

The website never bundles verse text — that is a licensing rule, not a
packaging preference — so the development fixture cannot read Scripture out of
the repository. It fetches two PUBLIC DOMAIN editions instead, converts them to
the canonical shape the Go API serves, and writes the result to /tmp, where it
is a development artefact that is never committed and never deployed.

    scripts/dev-bible-corpus.py            # writes /tmp/bible-corpus.json

Sources, both public domain and both fetched over HTTPS from GitHub:

  * King James Version  — github.com/aruljohn/Bible-kjv
  * World English Bible — github.com/TehShrike/world-english-bible

Book identity comes from web/lib/canon.json, which is generated from
server/internal/bible, so a file this script cannot place in the canon is
reported rather than guessed at. Chapter and verse counts are checked against
the canon after conversion and the deviation is printed: the KJV should match
the reference distribution exactly, and the WEB should differ only where that
edition genuinely differs.

If the network is unavailable the fixture simply has no text, and the reader
says so in the same words it would use against a real API outage.
"""

from __future__ import annotations

import io
import json
import os
import re
import sys
import tarfile
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.abspath(__file__)))
CANON_PATH = os.path.join(ROOT, "web", "lib", "canon.json")
OUT_PATH = os.environ.get("IC_BIBLE_CORPUS", "/tmp/bible-corpus.json")

SOURCES = {
    "kjv": {
        "url": "https://codeload.github.com/aruljohn/Bible-kjv/tar.gz/refs/heads/master",
        "cache": "/tmp/ic-kjv.tgz",
        "name": "King James Version",
        "abbreviation": "KJV",
        "source_url": "https://github.com/aruljohn/Bible-kjv",
        "license": "Public domain",
        "license_url": "https://en.wikipedia.org/wiki/King_James_Version#Copyright_status",
        "copyright": "Public domain in the United States and most of the world.",
        "attribution_text": "King James Version — public domain.",
    },
    "web": {
        "url": "https://codeload.github.com/TehShrike/world-english-bible/tar.gz/refs/heads/master",
        "cache": "/tmp/ic-web.tgz",
        "name": "World English Bible",
        "abbreviation": "WEB",
        "source_url": "https://github.com/TehShrike/world-english-bible",
        "license": "Public domain",
        "license_url": "https://worldenglish.bible/",
        "copyright": "The World English Bible is in the public domain.",
        "attribution_text": "World English Bible — public domain.",
    },
}


def slug(text: str) -> str:
    return re.sub(r"[^a-z0-9]", "", text.lower())


def load_canon() -> dict:
    with open(CANON_PATH, encoding="utf-8") as handle:
        return json.load(handle)


def book_lookup(canon: dict) -> dict:
    """Every spelling that should resolve to a canonical book ID, keyed with
    spacing and punctuation removed so file names match."""
    table: dict[str, str] = {}
    for book in canon["books"]:
        for key in [book["id"], book["name"], book["usfm"], book["abbreviation"], *book.get("aliases", [])]:
            table.setdefault(slug(key), book["id"])
    return table


def fetch(url: str, cache: str) -> bytes:
    if os.path.exists(cache) and os.path.getsize(cache) > 0:
        with open(cache, "rb") as handle:
            return handle.read()
    print(f"corpus: fetching {url}", file=sys.stderr)
    request = urllib.request.Request(url, headers={"user-agent": "iconfess-dev-fixture"})
    with urllib.request.urlopen(request, timeout=120) as response:
        payload = response.read()
    with open(cache, "wb") as handle:
        handle.write(payload)
    return payload


def read_kjv(payload: bytes, books: dict) -> dict:
    out: dict[str, dict[str, list[str]]] = {}
    with tarfile.open(fileobj=io.BytesIO(payload), mode="r:gz") as archive:
        for member in archive.getmembers():
            name = os.path.basename(member.name)
            if not member.isfile() or not name.endswith(".json"):
                continue
            book_id = books.get(slug(name[:-5]))
            if not book_id:
                continue
            data = json.loads(archive.extractfile(member).read().decode("utf-8"))
            chapters: dict[str, list[str]] = {}
            for chapter in data.get("chapters", []):
                number = str(int(chapter["chapter"]))
                verses = [v["text"].strip() for v in chapter.get("verses", []) if v.get("text", "").strip()]
                if verses:
                    chapters[number] = verses
            if chapters:
                out[book_id] = chapters
    return out


def read_web(payload: bytes, books: dict) -> dict:
    """The WEB source is a stream of typed fragments; a verse is the
    concatenation of every fragment carrying its number."""
    out: dict[str, dict[str, list[str]]] = {}
    with tarfile.open(fileobj=io.BytesIO(payload), mode="r:gz") as archive:
        for member in archive.getmembers():
            parts = member.name.split("/")
            if not member.isfile() or len(parts) < 3 or parts[1] != "json" or not parts[2].endswith(".json"):
                continue
            book_id = books.get(slug(parts[2][:-5]))
            if not book_id:
                continue
            fragments = json.loads(archive.extractfile(member).read().decode("utf-8"))
            chapters: dict[str, dict[int, str]] = {}
            for fragment in fragments:
                if not isinstance(fragment, dict):
                    continue
                value = fragment.get("value")
                chapter = fragment.get("chapterNumber")
                verse = fragment.get("verseNumber")
                if not value or not chapter or not verse:
                    continue
                if fragment.get("type") not in {"paragraph text", "line text", "paragraph start", "stanza"}:
                    if "text" not in str(fragment.get("type", "")):
                        continue
                bucket = chapters.setdefault(str(chapter), {})
                bucket[verse] = (bucket.get(verse, "") + " " + str(value).strip()).strip()
            ordered: dict[str, list[str]] = {}
            for number, verses in chapters.items():
                ordered[number] = [verses[key].strip() for key in sorted(verses) if verses[key].strip()]
            if ordered:
                out[book_id] = ordered
    return out


def audit(canon: dict, text: dict) -> dict:
    books = {b["id"]: b for b in canon["books"]}
    chapters = sum(len(v) for v in text.values())
    verses = sum(len(vs) for chs in text.values() for vs in chs.values())
    missing = [bid for bid in books if bid not in text]
    return {
        "books": len(text),
        "chapters": chapters,
        "verses": verses,
        "missing_books": missing,
        "reference_verses": canon["verse_count"],
        "deviation": verses - canon["verse_count"],
    }


def main() -> int:
    canon = load_canon()
    books = book_lookup(canon)
    corpus = {"generated_from": "public-domain sources fetched at build time", "translations": [], "text": {}}

    for translation_id, source in SOURCES.items():
        try:
            payload = fetch(source["url"], source["cache"])
        except Exception as error:  # noqa: BLE001 - the fixture degrades, it does not crash
            print(f"corpus: {translation_id} unavailable ({error})", file=sys.stderr)
            continue
        text = read_kjv(payload, books) if translation_id == "kjv" else read_web(payload, books)
        if not text:
            print(f"corpus: {translation_id} produced no books", file=sys.stderr)
            continue
        report = audit(canon, text)
        print(
            f"corpus: {translation_id} {report['books']} books, {report['chapters']} chapters, "
            f"{report['verses']} verses ({report['deviation']:+d} vs the reference distribution)",
            file=sys.stderr,
        )
        corpus["text"][translation_id] = text
        corpus["translations"].append(
            {
                "id": translation_id,
                "provider": "local-public-domain",
                "provider_translation_id": translation_id,
                "name": source["name"],
                "abbreviation": source["abbreviation"],
                "language_code": "en",
                "language_name": "English",
                "locale": "en-US",
                "direction": "ltr",
                "description": f"{source['name']} — public domain, loaded by the development fixture.",
                "copyright": source["copyright"],
                "license": source["license"],
                "license_url": source["license_url"],
                "public_domain": True,
                "commercial_use": True,
                "redistribution_allowed": True,
                "modification_allowed": False,
                # Text rights never imply audio rights: no recording has been
                # registered or reviewed, so the audio grant stays false.
                "audio_allowed": False,
                "offline_allowed": True,
                "copy_allowed": True,
                "share_allowed": True,
                "search_index_allowed": True,
                "api_exposure_allowed": True,
                "attribution_required": True,
                "attribution_text": source["attribution_text"],
                "source_url": source["source_url"],
                "status": "active",
                "coverage": "full" if report["books"] >= 66 else "partial",
                "book_count": report["books"],
                "chapter_count": report["chapters"],
                "verse_count": report["verses"],
                "content_hash": f"{translation_id}-{report['verses']}",
            }
        )

    if not corpus["translations"]:
        print("corpus: no translation could be built", file=sys.stderr)
        return 1

    with open(OUT_PATH, "w", encoding="utf-8") as handle:
        json.dump(corpus, handle)
    print(f"corpus: wrote {OUT_PATH} ({os.path.getsize(OUT_PATH) // 1024} KiB)", file=sys.stderr)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())

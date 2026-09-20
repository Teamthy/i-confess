#!/usr/bin/env python3
"""Static consistency checks for the Dart sources.

This is not an analyzer and does not pretend to be one. The Dart and Flutter
SDKs are unreachable from this sandbox (the PHASE 23 C-1 condition), so
`flutter analyze` and `dart test` are owed to CI. What this script does is
catch the specific class of mistake that costs a CI round trip: referring to a
model, constructor parameter, repository method or design token that does not
exist.

It reads the declarations out of the source rather than hard-coding a list, so
it cannot drift: deleting a model makes every use of it fail here.

Usage: python3 scripts/check_dart_symbols.py
"""
import re
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CLIENT = ROOT / "clients/dart/lib/src"
MOBILE = ROOT / "apps/mobile/lib"

failures = []
checks = 0


def check(name, condition, detail=""):
    global checks
    checks += 1
    if not condition:
        failures.append(f"{name}{'  -- ' + detail if detail else ''}")


def read(path):
    return path.read_text(encoding="utf-8")


# ---------------------------------------------------------------------------
# Model declarations
# ---------------------------------------------------------------------------

models_src = read(CLIENT / "models.dart")
declared_models = set(re.findall(r"^final class (\w+)", models_src, re.M))

for required in ["UserCollection", "CollectionItem", "Favorite", "UserConfession"]:
    check(f"model {required} is declared", required in declared_models)

# Each model must parse from JSON, or a repository cannot decode into it.
for model in ["UserCollection", "CollectionItem", "Favorite", "UserConfession"]:
    check(
        f"{model} has a fromJson factory",
        re.search(rf"factory {model}\.fromJson", models_src) is not None,
    )

# UserConfession must not be confused with the editorial Confession: the two
# read different keys, and swapping them renders empty rows.
uc = models_src[models_src.index("final class UserConfession"):]
check(
    "UserConfession reads the server's 'text' field",
    "_str(json, 'text')" in uc,
    "the editorial model reads short_text/description instead",
)
check("UserConfession reads its moderation status", "_str(json, 'status'" in uc)


# ---------------------------------------------------------------------------
# Repository methods used by the mobile library feature
# ---------------------------------------------------------------------------

repo_src = read(CLIENT / "repository.dart")


def method_names(class_name, source):
    """Method names declared on one class, up to the next top-level class."""
    start = source.index(f"final class {class_name}")
    rest = source[start + 1:]
    end = rest.find("\nfinal class ")
    body = rest if end == -1 else rest[:end]
    return set(re.findall(r"(?:Future<[^>]*(?:>>)?>|void)\s+(\w+)\s*\(", body)) | set(
        re.findall(r"\b(\w+)\s*\([^)]*\)\s*(?:async\s*)?[{=]", body)
    )


library_methods = method_names("LibraryRepository", repo_src)
for required in [
    "collections",
    "collection",
    "createCollection",
    "updateCollection",
    "deleteCollection",
    "addToCollection",
    "removeFromCollection",
    "reorderCollection",
    "favorites",
    "removeFavorite",
    "myConfessions",
    "submitConfession",
]:
    check(f"LibraryRepository.{required} exists", required in library_methods)


# ---------------------------------------------------------------------------
# Endpoints the repositories call must exist on the typed client
# ---------------------------------------------------------------------------

endpoints_src = read(CLIENT / "endpoints.dart")
declared_endpoints = set(re.findall(r"Future<Map<String, dynamic>> (\w+)\(", endpoints_src))

# ApiClient's own session helpers are not generated endpoints; they are
# transport methods the repositories are entitled to call.
transport_methods = set(
    re.findall(r"Future<[\w<>?, ]+> (\w+)\(", read(CLIENT / "api_client.dart"))
)
called = set(re.findall(r"\bapi\.(\w+)\(", repo_src))
missing = sorted(called - declared_endpoints - transport_methods)
check(
    "every api.* call in repository.dart has a typed endpoint",
    not missing,
    f"undeclared: {missing}",
)

# The submit route is a user-facing endpoint the library depends on.
check(
    "postMeConfessionsByIdSubmit is on the typed client",
    "postMeConfessionsByIdSubmit" in declared_endpoints,
)
check(
    "getMeFavorites accepts a type filter",
    re.search(r"getMeFavorites\(\{String\? type\}\)", endpoints_src) is not None,
)


# ---------------------------------------------------------------------------
# Every path the typed client calls must be a real server route
# ---------------------------------------------------------------------------

import json  # noqa: E402

routes = json.loads((ROOT / "design/routes.json").read_text())
live = {re.sub(r"\{[^}]+\}", "{x}", r["path"]) for r in routes}

client_paths = set()
for raw_path in re.findall(r"(?:get|post|patch|put|delete)\('(/[^']*)'", endpoints_src):
    # Query strings are not part of a route's identity, and some are built by
    # interpolation. Strip them before comparing against the route table.
    path = raw_path.split("?", 1)[0]
    # A whole interpolated segment (`/search$qs`) carries no literal path.
    path = re.sub(r"\$\{[^}]*\}", "{x}", path)
    path = re.sub(r"\$\w+", "{x}", path)
    # `get('/search$qs')` interpolates a whole query string onto the path, so
    # the trailing placeholder is not a path segment.
    for suffix in ("/{x}", "{x}"):
        if path.endswith(suffix) and path[: -len(suffix)] in live:
            path = path[: -len(suffix)]
            break
    client_paths.add(path)

unknown = sorted(client_paths - live)
check(
    "every path the typed client calls exists in the route table",
    not unknown,
    f"not served: {unknown}",
)


# ---------------------------------------------------------------------------
# Cache keys
# ---------------------------------------------------------------------------

cache_src = read(CLIENT / "cache.dart")
cache_keys = set(re.findall(r"static const (\w+) =", cache_src))
for required in ["collections", "favorites", "myConfessions"]:
    check(f"CacheKeys.{required} is declared", required in cache_keys)

check(
    "JsonCache exposes delete, so a write can invalidate a read",
    re.search(r"Future<void> delete\(String key\)", cache_src) is not None,
)

# Distinct cache keys for the two kinds of collection.
check(
    "the editorial collections use their own cache key",
    "'published_collections'" in repo_src,
    "sharing CacheKeys.collections would cross the two surfaces",
)


# ---------------------------------------------------------------------------
# Mobile feature: symbols and tokens
# ---------------------------------------------------------------------------

screen_src = read(MOBILE / "src/features/library/library_screen.dart")
providers_src = read(MOBILE / "src/features/library/library_providers.dart")

tokens_src = read(MOBILE / "src/core/theme/tokens.dart")
declared_tokens = set(re.findall(r"static const (?:\w+(?:<[^>]+>)?\s+)?(\w+)\s*=", tokens_src))
used_tokens = set(re.findall(r"IConfess\.(\w+)", screen_src))
unknown_tokens = sorted(used_tokens - declared_tokens)
check(
    "every IConfess token the library screen uses is declared",
    not unknown_tokens,
    f"unknown: {unknown_tokens}",
)

theme_src = read(MOBILE / "src/core/theme/theme.dart")
surface_fields = set(re.findall(r"^  final Color (\w+);", theme_src, re.M))
used_surfaces = set(re.findall(r"surfaces\.(\w+)", screen_src))
unknown_surfaces = sorted(used_surfaces - surface_fields)
check(
    "every AppSurfaces field the library screen uses is declared",
    not unknown_surfaces,
    f"unknown: {unknown_surfaces}",
)

# Providers referenced by the screen must exist.
declared_providers = set(re.findall(r"^final (\w+Provider) =", providers_src, re.M))
used_providers = set(re.findall(r"\b(\w+Provider)\b", screen_src))
core_providers = set(
    re.findall(r"^final (\w+Provider) =", read(MOBILE / "src/core/di/providers.dart"), re.M)
)
unknown_providers = sorted(used_providers - declared_providers - core_providers)
check(
    "every provider the library screen watches is declared",
    not unknown_providers,
    f"unknown: {unknown_providers}",
)

# Actions the screen calls must exist on LibraryActions.
actions_body = providers_src[providers_src.index("class LibraryActions"):]
# Return types nest generics (`Future<WriteResult<UserCollection>>`), so match
# balanced-ish angle brackets rather than a single level.
declared_actions = set(re.findall(r"Future<[\w<>?, ]+>\s+(\w+)\s*\(", actions_body))
used_actions = set(re.findall(r"libraryActionsProvider\)\s*\.\s*(\w+)\(", screen_src))
used_actions |= set(re.findall(r"\.read\(libraryActionsProvider\)\n?\s*\.(\w+)\(", screen_src))
unknown_actions = sorted(used_actions - declared_actions)
check(
    "every LibraryActions method the screen calls exists",
    not unknown_actions,
    f"unknown: {unknown_actions}",
)

# Model fields the screen reads must exist on the model.
def fields_of(model):
    body = models_src[models_src.index(f"final class {model}"):]
    end = body.find("\nfinal class ")
    if end != -1:
        body = body[:end]
    return set(re.findall(r"^  final [\w<>?, ]+ (\w+);", body, re.M)) | set(
        re.findall(r"^  (?:bool|String|int) get (\w+)", body, re.M)
    )


for model, prefix in [
    ("UserCollection", "collection"),
    ("Favorite", "favorite"),
    ("UserConfession", "confession"),
    ("CollectionItem", "item"),
]:
    declared = fields_of(model)
    used = set(re.findall(rf"\b{prefix}\.(\w+)\b", screen_src))
    # Method calls and Object members are not model fields.
    used -= {"toString", "hashCode", "runtimeType", "noSuchMethod"}
    unknown_fields = sorted(f for f in used if f not in declared)
    check(
        f"every {model} member the screen reads is declared",
        not unknown_fields,
        f"unknown: {unknown_fields}",
    )


# ---------------------------------------------------------------------------
# Routing
# ---------------------------------------------------------------------------

routes_dart = read(MOBILE / "src/core/routing/routes.dart")
used_routes = set(re.findall(r"AppRoutes\.(\w+)", screen_src))
declared_routes = set(re.findall(r"static (?:const|String) (\w+)", routes_dart))
unknown_routes = sorted(used_routes - declared_routes)
check(
    "every AppRoutes member the library screen uses is declared",
    not unknown_routes,
    f"unknown: {unknown_routes}",
)

# A screen that imports nothing it uses will not compile; check the imports
# that carry the symbols this file depends on.
for symbol, module in [
    ("UserCollection", "iconfess_api"),
    ("ErrorMapper", "error_mapper.dart"),
    ("EmptyState", "states.dart"),
    ("AppScaffold", "screen.dart"),
    ("AppRoutes", "routes.dart"),
    ("characters", "characters/characters.dart"),
]:
    if symbol == "characters":
        needed = ".characters" in screen_src
    else:
        needed = re.search(rf"\b{symbol}\b", screen_src) is not None
    check(
        f"library_screen.dart imports {module} for {symbol}",
        (not needed) or module in screen_src,
    )


# ---------------------------------------------------------------------------
# Third-party API surface
#
# This script derives its expectations from first-party source, so it is blind
# to the shape of packages it cannot see. That blindness shipped a real compile
# error once: `AsyncValue.valueOrNull` does not exist in the pinned Riverpod,
# and CI caught it only after a full Flutter build. Rather than model Riverpod,
# hold the codebase to the one spelling it already uses everywhere — a
# convention check, which is enforceable without the package.
# ---------------------------------------------------------------------------

feature_dart = sorted((MOBILE / "src/features").rglob("*.dart"))
assert feature_dart, "no feature sources found; the scan path is wrong"
offenders = []
for path in feature_dart:
    for lineno, line in enumerate(read(path).splitlines(), 1):
        # `ref.watch(x).valueOrNull` / `async.valueOrNull` unwraps an
        # AsyncValue directly. The established spelling is `.asData?.value`.
        # A `.valueOrNull` reached THROUGH `.asData?.value` is the Loadable's
        # own, and correct.
        for m in re.finditer(r"(\w+)\.valueOrNull", line):
            recv = m.group(1)
            before = line[: m.start()]
            if recv == "value" and before.rstrip().endswith("asData?."):
                continue
            if recv == "value" and "asData!" in before:
                continue
            if recv in {"async"} or re.search(r"ref\.watch\([^)]*\)$", before):
                offenders.append(f"{path.name}:{lineno}")

check(
    "AsyncValue is unwrapped with .asData?.value, not .valueOrNull",
    not offenders,
    f"AsyncValue has no valueOrNull in the pinned Riverpod: {offenders}",
)


# ---------------------------------------------------------------------------
# autoDispose + ref-after-await
#
# A Provider holding an actions object is reached with `ref.read` inside a tap
# handler. A `read` registers no listener, so an autoDispose provider is
# disposed immediately and any `ref.invalidate` after an await throws
# UnmountedRefException. The write has already reached the server by then, so
# the symptom is the worst kind: saved, and never shown. This shipped once.
# ---------------------------------------------------------------------------

ref_after_await = []
for path in feature_dart:
    src = read(path)
    # Providers of a plain object (not Future/Stream) that auto-dispose.
    for m in re.finditer(r"final (\w+) = Provider\.autoDispose<(\w+)>", src):
        provider, held = m.group(1), m.group(2)
        cls = re.search(r"class %s\b.*?(?=\nclass |\Z)" % re.escape(held), src, re.S)
        if not cls:
            continue
        body = cls.group(0)
        # Does any async method touch the ref after an await?
        for meth in re.finditer(r"async \{(.*?)\n  \}", body, re.S):
            code = meth.group(1)
            if "await" in code and re.search(r"_?ref\.(invalidate|read|refresh)", code):
                after = code.split("await", 1)[1]
                if re.search(r"_?ref\.(invalidate|read|refresh)", after):
                    ref_after_await.append(f"{path.name}:{provider}")
                    break

check(
    "no autoDispose Provider uses its ref after an await",
    not ref_after_await,
    "disposed before the await returns, so the invalidate throws: "
    f"{sorted(set(ref_after_await))}",
)


# ---------------------------------------------------------------------------
# Widget test: keys it drives must be keys the screen renders
# ---------------------------------------------------------------------------

test_path = ROOT / "apps/mobile/test/library_test.dart"
if test_path.exists():
    test_src = read(test_path)

    screen_keys = set(re.findall(r"ValueKey\('([^'$]+)'\)", screen_src))
    # Keys built by interpolation (`ValueKey('submit-${c.id}')`) contribute a
    # prefix, since the id comes from the fixture.
    screen_prefixes = set(re.findall(r"ValueKey\('([^'$]*)\$", screen_src))

    unresolved = []
    for key in sorted(set(re.findall(r"ValueKey\('([^']+)'\)", test_src))):
        if key in screen_keys:
            continue
        if any(key.startswith(p) for p in screen_prefixes if p):
            continue
        unresolved.append(key)
    check(
        "every widget key the test drives is rendered by the screen",
        not unresolved,
        f"never rendered: {unresolved}",
    )

    # Methods the test calls on the scripted client must exist on it.
    fake_src = read(ROOT / "apps/mobile/test/support/fake_api_client.dart")
    fake_members = set(re.findall(r"\b(\w+)\s*\(", fake_src)) | set(
        re.findall(r"final [\w<>, ]+ (\w+)\s*=", fake_src)
    )
    used_fake = set(re.findall(r"\bapi\.(\w+)\b", test_src))
    unknown_fake = sorted(used_fake - fake_members)
    check(
        "every FakeApiClient member the test uses is declared",
        not unknown_fake,
        f"unknown: {unknown_fake}",
    )

    # Providers the test invalidates must exist.
    # Providers are declared across the feature and core trees, so the whole
    # lib/ directory is the authority rather than one file.
    all_providers = set()
    for dart in MOBILE.rglob("*.dart"):
        all_providers |= set(re.findall(r"^final (\w+Provider) =", read(dart), re.M))
        # Riverpod generators and notifier providers are declared differently.
        all_providers |= set(
            re.findall(r"^final (\w+Provider) =\s*$", read(dart), re.M)
        )

    used_test_providers = set(re.findall(r"\b(\w+Provider)\b", test_src))
    unknown_test_providers = sorted(used_test_providers - all_providers)
    check(
        "every provider the test references is declared",
        not unknown_test_providers,
        f"unknown: {unknown_test_providers}",
    )


# ---------------------------------------------------------------------------
# Report
# ---------------------------------------------------------------------------

print(f"Dart symbol check: {checks} assertions")
for failure in failures:
    print(f"  FAIL  {failure}")

if failures:
    print(f"\nFAILED: {len(failures)} of {checks}")
    sys.exit(1)

print(f"PASSED: {checks}/{checks}")

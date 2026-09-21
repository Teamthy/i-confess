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
    m = re.search(rf"^(?:final )?class {class_name}\b", source, re.M)
    if m is None:
        raise SystemExit(f"check_dart_symbols: class {class_name} is not declared")
    start = m.start()
    rest = source[start + 1:]
    end = re.search(r"^(?:final )?class \w+", rest, re.M)
    body = rest if end is None else rest[: end.start()]
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

# PHASE 36 / G-29: the trial lifecycle is a typed client surface, not a
# paywall-only visual. Keep the three mutations/reads and their status model
# tied to the server routes so a future trial UI cannot silently call a 404.
check("TrialStatus model is declared", "final class TrialStatus" in models_src)
check("TrialStatus has a fromJson factory", "factory TrialStatus.fromJson" in models_src)
for required in [
    "postSubscriptionsTrial",
    "getSubscriptionsTrialStatus",
    "postSubscriptionsTrialConvert",
]:
    check(f"{required} is on the typed client", required in declared_endpoints)

subscription_methods = method_names("SubscriptionRepository", repo_src)
for required in ["startTrial", "trialStatus", "convertTrial"]:
    check(f"SubscriptionRepository.{required} exists", required in subscription_methods)

trial_ia = (ROOT / "design/ia.json").read_text()
for endpoint in [
    "POST /subscriptions/trial",
    "GET /subscriptions/trial/status",
    "POST /subscriptions/trial/convert",
]:
    check(f"trial endpoint {endpoint} is wired in IA", endpoint in trial_ia)

check(
    "TrialStatus distinguishes the entitled projection",
    "final bool entitled;" in models_src,
)

# PHASE 41: the journey is measured, and each day states what it teaches.
# Without the engagement read the trial could be displayed but not counted,
# and without intent/flags a client can only guess which surface a day wants.
check(
    "getSubscriptionsTrialEngagement is on the typed client",
    "getSubscriptionsTrialEngagement" in declared_endpoints,
)
check(
    "SubscriptionRepository.trialEngagement exists",
    "trialEngagement" in subscription_methods,
)
check("TrialEngagement model is declared", "final class TrialEngagement" in models_src)
check(
    "TrialEngagement has a fromJson factory",
    "factory TrialEngagement.fromJson" in models_src,
)
check(
    "TrialEngagement counts completed days, not elapsed ones",
    "final int daysCompleted;" in models_src and "final int daysTotal;" in models_src,
)
check(
    "TrialEngagement carries the funnel behind the numbers",
    "final Map<String, int> funnel;" in models_src,
)
check(
    "TrialDayCompletion names the session that completed the day",
    "final class TrialDayCompletion" in models_src and "final String sessionId;" in models_src,
)
for field in ["final String intent;", "final List<String> categories;", "final int duration;"]:
    check(f"TrialDay carries {field.split()[-1].rstrip(';')}", field in models_src)
for flag in ["sessionCount", "personalized", "premiumVoice", "custom", "summary"]:
    check(f"TrialDay exposes the {flag} flag", f"this.{flag}" in models_src)
check(
    "trial engagement is wired in IA",
    "GET /subscriptions/trial/engagement" in trial_ia,
)

# The paywall renders the measured journey. The call to action was declared on
# the client and never sent by the server, so every journey row rendered
# without one; and a tick must mean a session was finished, not that a day
# elapsed on the clock.
premium_screen_src = read(MOBILE / "src/features/premium/premium_screen.dart")
premium_providers_src = read(MOBILE / "src/features/premium/premium_providers.dart")
check(
    "the paywall renders each day's call to action",
    "day.cta" in premium_screen_src,
)
check(
    "the paywall has a measured-journey provider",
    "trialEngagementProvider" in premium_providers_src,
)
check(
    "the paywall marks completed days from the engagement read",
    "completed.contains(day.day)" in premium_screen_src,
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
# Community (G-40) — public UGC reader
# ---------------------------------------------------------------------------

check(
    "getCommunityConfessions is on the typed client",
    "getCommunityConfessions" in declared_endpoints,
)

community_repo_methods = method_names("CommunityRepository", repo_src)
for required in ["feed", "confessions", "react"]:
    check(f"CommunityRepository.{required} exists", required in community_repo_methods)

# Mobile community screen symbols
community_screen_src = read(MOBILE / "src/features/community/community_screen.dart")
check(
    "community_screen.dart declares communityFeedProvider",
    "communityFeedProvider" in read(MOBILE / "src/features/community/community_screen.dart"),
)
check(
    "community_screen.dart declares communityConfessionsProvider",
    "communityConfessionsProvider" in community_screen_src,
)
check(
    "community_screen.dart renders both tabs",
    "Stories" in community_screen_src and "Testimonies" in community_screen_src,
)

# The community endpoint must be wired in IA
ia_src = (ROOT / "design/ia.json").read_text()
check(
    "community screen declares GET /community/confessions in IA",
    "GET /community/confessions" in ia_src,
)

# ---------------------------------------------------------------------------
# Library gestures (G-43, G-44, G-45) — PHASE 34
# ---------------------------------------------------------------------------

library_screen_src = read(MOBILE / "src/features/library/library_screen.dart")
library_providers_src = read(MOBILE / "src/features/library/library_providers.dart")
confession_detail_src = read(MOBILE / "src/features/confession/confession_detail_screen.dart")

check(
    "collection detail reorders with a ReorderableListView",
    "ReorderableListView.builder" in library_screen_src,
)
# The row itself navigates, so the whole-row drag handle must be off and an
# explicit grip must own the drag. Flutter's delayed variant is the one in use:
# a long-press starts the drag so a tap on the grip cannot be swallowed as one.
check(
    "rows expose an explicit drag handle (rows also navigate)",
    "ReorderableDragStartListener" in library_screen_src
    or "ReorderableDelayedDragStartListener" in library_screen_src,
)
check(
    "the whole-row drag handle is off, because the row navigates",
    "buildDefaultDragHandles: false" in library_screen_src,
)
check(
    "the cover dialog exists and is wired to the menu",
    "_CoverUrlDialog" in library_screen_src and "'cover'" in library_screen_src,
)

library_actions = method_names("LibraryActions", library_providers_src)
for required in ["reorderCollection", "addToCollection", "updateCover", "removeFromCollection"]:
    check(f"LibraryActions.{required} exists", required in library_actions)

check(
    "LibraryRepository.updateCollection carries cover_url through",
    "'cover_url': coverUrl" in repo_src,
)

# G-45: the favourites tab navigates every kind it lists, not only
# confessions; a dead row is the defect this phase closed.
for route in [
    "AppRoutes.confessionDetail",
    "AppRoutes.categoryDetail",
    "AppRoutes.playerWithId",
    "AppRoutes.builderVoice",
]:
    check(
        f"favourite rows reach {route.split('.')[-1]}",
        route in library_screen_src,
    )

# G-43: the add gesture lives on the confession page itself.
check(
    "confession detail offers add-to-collection",
    "_AddToCollectionButton" in confession_detail_src
    and "addToCollection(" in confession_detail_src,
)
check(
    "confession detail reaches the library actions",
    "library/library_providers.dart" in read(
        MOBILE / "src/features/confession/confession_detail_screen.dart"
    ),
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

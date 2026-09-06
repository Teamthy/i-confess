#!/usr/bin/env python3
"""Information architecture tests. PHASE 06.

Three things fail quietly in a nav graph, and none of them are visible in a
screenshot:

  - a screen nobody can reach, because the link to it was removed
  - a screen nobody can leave, because its only back path was removed
  - a screen that calls an endpoint the backend does not have, which renders
    as a spinner forever

The third is the expensive one. It is checked here against design/routes.json,
which is exported from the running handler by TestExportRouteTable rather than
transcribed by hand.

Usage: python3 design/test_ia.py
"""
import json
import sys
from collections import deque
from pathlib import Path

HERE = Path(__file__).parent
sys.path.insert(0, str(HERE))
import generate  # noqa: E402

IA = json.loads((HERE / "ia.json").read_text())
ROUTES = json.loads((HERE / "routes.json").read_text())

failures = []


def check(name, condition, detail=""):
    print(f"  {'PASS' if condition else 'FAIL'}  {name}"
          + (f"  -- {detail}" if detail and not condition else ""))
    if not condition:
        failures.append(name)


screens = IA["screens"]
by_id = {s["id"]: s for s in screens}

# The live endpoint set, unprefixed. /v1/ twins are registered too but the
# Flutter client uses the unprefixed forms.
live = {f"{r['method']} {r['path']}" for r in ROUTES if not r["path"].startswith("/v1/")}

print("Section 12 - tab bar")
_, tokens, _ = generate.load()
mandated = [t.strip() for t in tokens["navigation.bottom"].split(",")]
declared = IA["tabs"]["order"]
check("five tabs declared", len(declared) == 5, f"got {len(declared)}")
check("tabs match the navigation.bottom token", declared == mandated,
      f"declared {declared}, token {mandated}")
check("tab order matches the token exactly", declared == mandated)
tabbed = [s["id"] for s in screens if "tab" in s]
check("exactly one screen per tab", sorted(tabbed) == sorted(declared),
      f"tabbed screens {sorted(tabbed)}")
check("every tab id names a real screen", all(t in by_id for t in declared),
      f"missing {[t for t in declared if t not in by_id]}")
# The retired shell labelled these Discover and Create. Section 12 names them
# EXPLORE and CONFESS, and the words carry the product's meaning: you are not
# browsing content, and you are not creating a document.
check("labels are explore and confess, not discover and create",
      "explore" in declared and "confess" in declared)

print("\nReachability")
roots = [s["id"] for s in screens if not s["parents"]]
check("there is at least one entry point", len(roots) > 0)

forward = {}
for s in screens:
    for p in s["parents"]:
        forward.setdefault(p, []).append(s["id"])

reached = set()
queue = deque(roots)
while queue:
    node = queue.popleft()
    if node in reached:
        continue
    reached.add(node)
    queue.extend(forward.get(node, []))

orphans = sorted(set(by_id) - reached)
check("no orphan screens", not orphans, f"unreachable: {orphans}")

dangling = sorted({p for s in screens for p in s["parents"]} - set(by_id))
check("every parent names a real screen", not dangling, f"unknown parents: {dangling}")

stranded = [s["id"] for s in screens
            if not s["parents"] and "tab" not in s
            and s["id"] not in ("onboarding", "auth/sign-in", "template/share")]
check("no dead-end screens", not stranded,
      f"no way back: {stranded}")

print("\nEndpoint coverage")
missing = {}
for s in screens:
    bad = [e for e in s.get("endpoints", []) if e not in live]
    if bad:
        missing[s["id"]] = bad
check("every screen's endpoints exist in the running API", not missing,
      json.dumps(missing, indent=4))

no_api = [s["id"] for s in screens if not s.get("endpoints")]
check("every screen declares its endpoints", not no_api,
      f"screens with none: {no_api}")

# Backend surface with no home in the UI is either dead code or a missing
# screen. Worth knowing which, so this reports rather than silently passing.
used = {e for s in screens for e in s.get("endpoints", [])}
unused = sorted(live - used)
non_ui = [u for u in unused
          if not any(k in u for k in
                     ("/health", "/healthz", "/admin/", "/auth/logout",
                      "/auth/refresh", "/me/history", "/v1/"))]
check("every user-facing endpoint has a screen", not non_ui,
      f"{len(non_ui)} unreferenced: {non_ui}")

print("\nCore loop")
def depth_from(start):
    """Shortest number of navigations from start to each screen."""
    dist = {start: 0}
    q = deque([start])
    while q:
        n = q.popleft()
        for nxt in forward.get(n, []):
            if nxt not in dist:
                dist[nxt] = dist[n] + 1
                q.append(nxt)
    return dist

from_home = depth_from("home")
from_confess = depth_from("confess")
check("a session is reachable from home in 2 taps or fewer",
      from_home.get("player", 99) <= 2,
      f"home -> player is {from_home.get('player', 'unreachable')}")
check("the builder reaches the player within 4 taps",
      from_confess.get("player", 99) <= 4,
      f"confess -> player is {from_confess.get('player', 'unreachable')}")
# Tab-to-tab is one tap by construction in a bottom nav, so asserting it would
# only ever test the test. The property that actually matters is that the
# paywall is not on the critical path: a free user must be able to reach a
# session without going through it.
def reach_excluding(start, blocked):
    dist = {start: 0}
    q = deque([start])
    while q:
        n = q.popleft()
        for nxt in forward.get(n, []):
            if nxt in blocked or nxt in dist:
                continue
            dist[nxt] = dist[n] + 1
            q.append(nxt)
    return dist

free_path = reach_excluding("home", {"paywall"})
check("a free user can reach a session without the paywall",
      "player" in free_path, "player is only reachable via paywall")
check("the paywall is reachable from home", "paywall" in from_home,
      "no path from home to paywall")
check("the builder is reachable without the paywall",
      "confess/create" in reach_excluding("confess", {"paywall"}),
      "session creation is gated behind purchase")

print("\nStructure")
check("screen ids are unique", len(by_id) == len(screens),
      f"{len(screens)} screens, {len(by_id)} unique ids")
check("depth is consistent with parents",
      all(s["depth"] == 0 or any(
          by_id[p]["depth"] < s["depth"] for p in s["parents"] if p in by_id)
          for s in screens),
      "a screen deeper than none of its parents cannot be navigated to in order")
check("no screen is its own parent",
      all(s["id"] not in s["parents"] for s in screens))

print()
if failures:
    print(f"IA CHECK FAILED: {len(failures)} assertion(s)")
    for f in failures:
        print(f"  - {f}")
    sys.exit(1)
print(f"IA CHECK PASSED: {len(screens)} screens, {len(roots)} entry points, "
      f"{len(used)} endpoints wired.")

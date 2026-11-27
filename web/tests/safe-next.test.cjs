// Open-redirect guard for ?next= after sign-in and sign-up.
const test = require("node:test");
const assert = require("node:assert/strict");
const { safeNext } = require("../node_modules/.cache/safe-next-test/safe-next.js");

const O = "https://iconfess.example";
const q = (v) => "?next=" + encodeURIComponent(v);

test("keeps same-origin paths with query and hash", () => {
  assert.equal(safeNext(q("/app/journal"), O), "/app/journal");
  assert.equal(safeNext(q("/app/search?q=grace#top"), O), "/app/search?q=grace#top");
});

test("falls back to /app when next is missing or relative", () => {
  assert.equal(safeNext("", O), "/app");
  assert.equal(safeNext(q("app"), O), "/app");
  assert.equal(safeNext(q("https://evil.com"), O), "/app");
});

test("refuses protocol-relative and backslash forms", () => {
  for (const v of ["//evil.com", "/\\evil.com", "/\\/evil.com", "/%5Cevil.com", "\\\\evil.com"]) {
    const out = safeNext(q(v), O);
    assert.ok(out === "/app" || out.startsWith("/"), v);
    assert.ok(!/evil\.com/.test(new URL(out, O).host), v + " -> " + out);
  }
  assert.equal(safeNext(q("/\\evil.com"), O), "/app");
  assert.equal(safeNext(q("//evil.com"), O), "/app");
});

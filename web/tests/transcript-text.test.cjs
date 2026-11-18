const test = require("node:test");
const assert = require("node:assert/strict");
const React = require("react");
const { renderToStaticMarkup } = require("react-dom/server");
const { TranscriptText } = require("../node_modules/.cache/transcript-test/transcript-text.js");

function render(text, enabled) {
  return renderToStaticMarkup(React.createElement(TranscriptText, { text, enabled }));
}

test("shows available text without claiming or simulating synchronization", () => {
  const html = render("First sentence.\nSecond sentence!", true);
  assert.match(html, /First sentence\.\nSecond sentence!/);
  assert.match(html, /not synchronized and may differ from the recording/);
  assert.doesNotMatch(html, /tr-line|aria-current|<mark/);
});

test("shows no text while the preference is off", () => {
  const html = render("Private words should not be displayed.", false);
  assert.match(html, /Text is hidden/);
  assert.doesNotMatch(html, /Private words/);
});

test("reports missing or whitespace-only text honestly", () => {
  for (const text of ["", " \n "]) {
    const html = render(text, true);
    assert.match(html, /Text is not available/);
    assert.doesNotMatch(html, /not synchronized/);
  }
});

test("text is escaped, not interpreted as markup", () => {
  const html = render('<script>alert("hello")</script>', true);
  assert.doesNotMatch(html, /<script>/);
  assert.match(html, /&lt;script&gt;/);
});

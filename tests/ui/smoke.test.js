const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

test("UI test runner is available", () => {
  assert.equal(true, true);
});

test("recommendation grids keep incomplete rows at card width", () => {
  const css = fs.readFileSync(
    path.resolve(__dirname, "..", "..", "plugin", "stashRecommendations", "ui", "recommendations.css"),
    "utf8"
  );

  assert.match(css, /\.stash-recommendations__grid\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill,/s);
});

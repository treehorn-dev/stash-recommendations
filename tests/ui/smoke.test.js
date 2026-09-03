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

test("recommendation cards use native-sized columns and clamp verbose content", () => {
  const css = fs.readFileSync(
    path.resolve(__dirname, "..", "..", "plugin", "stashRecommendations", "ui", "recommendations.css"),
    "utf8"
  );

  assert.match(css, /\.stash-recommendations__grid\s*\{[^}]*minmax\(270px,\s*1fr\)/s);
  assert.match(css, /\.stash-recommendations__entity-card\s*\{[^}]*align-content:\s*start[^}]*max-height:\s*25rem/s);
  assert.match(css, /\.stash-recommendations__entity-card\s+\.stash-composables-entity-card__description\s*\{[^}]*-webkit-line-clamp:\s*3/s);
  assert.match(css, /\.stash-recommendations__entity-card\s+\.stash-composables-entity-card__attribute dd\s*\{[^}]*-webkit-line-clamp:\s*1/s);
});

test("recommendation cards present count controls in a separated native-style rail", () => {
  const css = fs.readFileSync(
    path.resolve(__dirname, "..", "..", "plugin", "stashRecommendations", "ui", "recommendations.css"),
    "utf8"
  );

  assert.match(css, /\.stash-recommendations__entity-card\s+\.stash-composables-entity-card__count-rail\s*\{[^}]*border-top:/s);
  assert.match(css, /\.stash-recommendations__entity-card\s+\.stash-composables-entity-card__count-trigger\s*\{[^}]*background:/s);
});

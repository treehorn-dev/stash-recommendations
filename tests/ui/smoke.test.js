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

test("recommendation cards retain native fixed-width columns when a scene tab has one result", () => {
  const css = fs.readFileSync(
    path.resolve(__dirname, "..", "..", "plugin", "stashRecommendations", "ui", "recommendations.css"),
    "utf8"
  );

  assert.match(css, /\.stash-recommendations__grid\s*\{[^}]*grid-template-columns:\s*repeat\(auto-fill,\s*minmax\(270px,\s*320px\)\)[^}]*justify-content:\s*start/s);
  assert.match(css, /@media\s*\(max-width:\s*576px\)\s*\{\s*\.stash-recommendations__grid\s*\{[^}]*grid-template-columns:\s*1fr/s);
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

test("recommendation cards use flexible details and poster-inset badges", () => {
  const css = fs.readFileSync(
    path.resolve(__dirname, "..", "..", "plugin", "stashRecommendations", "ui", "recommendations.css"),
    "utf8"
  );

  assert.match(css, /.stash-recommendations__entity-card\s*\{[^}]*display:\s*flex[^}]*flex-direction:\s*column/s);
  assert.match(css, /.stash-recommendations__entity-card\s+\.stash-composables-entity-card__body\s*\{[^}]*flex:\s*1/s);
  assert.match(css, /.stash-recommendations__poster-badges\s*\{[^}]*left:/s);
  assert.match(css, /.stash-recommendations__poster-badges\s*\{[^}]*max-height:/s);
});

test("recommendation cards use native scene-card heatmap and overlay placement", () => {
  const css = fs.readFileSync(
    path.resolve(__dirname, "..", "..", "plugin", "stashRecommendations", "ui", "recommendations.css"),
    "utf8"
  );

  assert.match(css, /\.stash-recommendations__entity-card\s+\.stash-composables-entity-card__media-rail\s*\{[^}]*margin-top:\s*0/s);
  assert.match(css, /\.stash-recommendations__heatmap\s*\{[^}]*height:\s*auto[^}]*width:\s*100%/s);
  assert.match(css, /\.stash-recommendations__poster-overlay--studio\s*\{[^}]*right:\s*0\.7rem[^}]*top:\s*0\.7rem/s);
  assert.match(css, /\.stash-recommendations__poster-overlay--duration\s*\{[^}]*bottom:\s*1rem[^}]*right:\s*0\.7rem/s);
  assert.match(css, /\.stash-recommendations__poster-overlay--speed\s*\{[^}]*bottom:\s*1rem[^}]*left:\s*0\.7rem/s);
});

test("recommendation cards retain outline score semantics", () => {
  const css = fs.readFileSync(
    path.resolve(__dirname, "..", "..", "plugin", "stashRecommendations", "ui", "recommendations.css"),
    "utf8"
  );

  assert.match(css, /\.stash-recommendations__badge-cell--score:not\(\.stash-recommendations__badge-cell--local\)\s*\{[^}]*background:\s*transparent/s);
  assert.match(css, /\.stash-recommendations__badge-cell--rating-100-18\s*\{[^}]*#ff4812/s);
  assert.match(css, /\.stash-recommendations__badge-cell--rating-100-2\s*\{[^}]*#9e8974/s);
});

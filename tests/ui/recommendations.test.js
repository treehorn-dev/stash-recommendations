const test = require("node:test");
const assert = require("node:assert/strict");
const fs = require("node:fs");
const path = require("node:path");

const modulePath = path.resolve(
  __dirname,
  "..",
  "..",
  "plugin",
  "stashRecommendations",
  "ui",
  "recommendations.js"
);

const {
  fetchForYou,
  fetchRelated,
  resolveLocalRecommendations,
} = require(modulePath);

test("remote-only results stay hidden until enabled", () => {
  const items = [
    {
      content_key: { endpoint: "https://box.example/graphql", stash_id: "1" },
      canonical_url: "https://box.example/scenes/1",
      reasons: ["content_group"],
      score: 0.7,
    },
  ];

  assert.deepEqual(resolveLocalRecommendations(items, () => [], false), []);
  assert.equal(resolveLocalRecommendations(items, () => [], true)[0].kind, "remote");
});

test("resolveLocalRecommendations prefers local scene cards when available", () => {
  const items = [
    {
      content_key: { endpoint: "https://box.example/graphql", stash_id: "scene-1" },
      canonical_url: "https://box.example/scenes/scene-1",
      reasons: ["session_cooccurrence"],
      score: 0.95,
    },
  ];

  const resolved = resolveLocalRecommendations(
    items,
    (item) => [{ id: "44", title: "Local Scene", stash_ids: [item.content_key] }],
    false
  );

  assert.equal(resolved.length, 1);
  assert.equal(resolved[0].kind, "local");
  assert.equal(resolved[0].scene.id, "44");
  assert.deepEqual(resolved[0].reasons, ["session_cooccurrence"]);
});

test("fetchRelated proxies all content keys through the raw plugin task", async () => {
  const calls = [];
  const items = [{ content_key: { endpoint: "https://box.example/graphql", stash_id: "scene-2" } }];

  const result = await fetchRelated(
    [
      { endpoint: "https://box.example/graphql", stash_id: "scene-1" },
      { endpoint: "https://box-2.example/graphql", stash_id: "scene-9" },
    ],
    async (args) => {
      calls.push(args);
      return { items, model_version: "model-1" };
    }
  );

  assert.deepEqual(calls, [
    {
      mode: "fetch-related",
      content_keys: [
        { endpoint: "https://box.example/graphql", stash_id: "scene-1" },
        { endpoint: "https://box-2.example/graphql", stash_id: "scene-9" },
      ],
      limit: 20,
    },
  ]);
  assert.equal(result.model_version, "model-1");
  assert.deepEqual(result.items, items);
});

test("fetchForYou uses the plugin task proxy and limit override", async () => {
  const calls = [];

  await fetchForYou(8, async (args) => {
    calls.push(args);
    return { items: [], model_version: "model-2" };
  });

  assert.deepEqual(calls, [{ mode: "fetch-for-you", limit: 8 }]);
});

test("plugin UI registers route, nav, scene tab, and hides API key markup", () => {
  const source = fs.readFileSync(modulePath, "utf8");

  assert.equal(source.includes('PluginApi.register.route("/plugins/stash-recommendations"'), true);
  assert.equal(source.includes('PluginApi.patch.before("MainNavBar.MenuItems"'), true);
  assert.equal(source.includes('PluginApi.patch.before("ScenePage.Tabs"'), true);
  assert.equal(source.includes('PluginApi.patch.before("ScenePage.TabContent"'), true);
  assert.equal(source.includes("Loading recommendations..."), true);
  assert.equal(source.includes("Configure the StashRec service URL and API key."), true);
  assert.equal(source.includes("No recommendations yet."), true);
  assert.equal(source.includes("api_key_configured"), true);
  assert.equal(source.includes("settings.api_key"), false);
});

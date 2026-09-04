const test = require("node:test");
const assert = require("node:assert/strict");
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
  describeLocalScene,
  formatRecommendationReasons,
  sceneCountRailEntries,
  recommendationBadgeCells,
  cachePredictedRatings,
  clearPredictedRatings,
  createPredictedRatingsCache,
  registerPredictedRatingProvider,
  fetchForYou,
  forYouFilterFields,
  forYouFiltersKey,
  fetchRelated,
  forYouOffsetForPage,
  indexLocalSceneMatches,
  localSceneLookupBatches,
  partitionRecommendations,
  resolveLocalRecommendations,
  shouldFetchAnotherForYouBatch,
} = require(modulePath);

test("registers cached prediction values only when Better Scene Card is available", () => {
  const scores = new Map([["stale", 4.9], ["43", 3.3], ["44", 3.5]]);
  clearPredictedRatings(scores);
  assert.equal(scores.size, 0);
  cachePredictedRatings([
    { kind: "local", item: { predicted_rating: 4.2 }, scene: { id: "42" } },
    { kind: "local", item: { predicted_rating: "invalid" }, scene: { id: "43" } },
    { kind: "local", item: { predicted_rating: null }, scene: { id: "44" } },
  ], scores);
  assert.equal(scores.get("42"), 4.2);
  assert.equal(scores.has("43"), false);
  assert.equal(scores.has("44"), false);

  const calls = [];
  const root = {
    StashBetterSceneCard: {
      registerValue(name, provider) {
        calls.push({ name, provider });
        return true;
      },
    },
  };
  assert.equal(registerPredictedRatingProvider(root, scores), true);
  assert.equal(calls[0].name, "stash-recommendations.predicted-rating");
  assert.equal(calls[0].provider.get({ scene: { id: "42" } }), 4.2);
  assert.equal(calls[0].provider.get({ scene: { id: "missing" } }), undefined);
  assert.equal(registerPredictedRatingProvider({}, scores), false);
});

test("replaces prior values when a scene no longer has a finite prediction", () => {
  const scores = new Map([["42", 4.5], ["43", 3.5], ["unchanged", 2.5]]);
  cachePredictedRatings([
    { kind: "local", item: { predicted_rating: null }, scene: { id: "42" } },
    { kind: "local", item: { predicted_rating: "not-a-number" }, scene: { id: "43" } },
  ], scores);

  assert.equal(scores.has("42"), false);
  assert.equal(scores.has("43"), false);
  assert.equal(scores.get("unchanged"), 2.5);
});

test("prediction cache invalidates stale asynchronous loads by version", () => {
  const cache = createPredictedRatingsCache();
  const first = cache.beginLoad();
  cache.scores.set("42", 4.5);
  const second = cache.beginLoad();

  assert.equal(cache.scores.size, 0);
  assert.equal(cache.isCurrent(first), false);
  assert.equal(cache.isCurrent(second), true);
});

test("prediction cache retains prior page values during append pagination", () => {
  const cache = createPredictedRatingsCache();
  cache.beginLoad();
  cache.scores.set("page-one", 4.5);
  const append = cache.beginLoad({ append: true });

  assert.equal(cache.scores.get("page-one"), 4.5);
  assert.equal(cache.isCurrent(append), true);
});

test("local scene lookup batches by endpoint and preserves content-key matches", () => {
  const items = [
    { content_key: { endpoint: "https://one.example/graphql", stash_id: "scene-1" } },
    { content_key: { endpoint: "https://one.example/graphql", stash_id: "scene-2" } },
    { content_key: { endpoint: "https://two.example/graphql", stash_id: "scene-1" } },
    { content_key: { endpoint: "https://one.example/graphql", stash_id: "scene-1" } },
    { content_key: { endpoint: "", stash_id: "ignored" } },
  ];

  assert.deepEqual(localSceneLookupBatches(items, 1), [
    { endpoint: "https://one.example/graphql", stash_ids: ["scene-1"] },
    { endpoint: "https://one.example/graphql", stash_ids: ["scene-2"] },
    { endpoint: "https://two.example/graphql", stash_ids: ["scene-1"] },
  ]);

  const matches = indexLocalSceneMatches(items, [
    {
      id: "local-one",
      stash_ids: [
        { endpoint: "https://one.example/graphql", stash_id: "scene-1" },
        { endpoint: "https://two.example/graphql", stash_id: "scene-1" },
      ],
    },
    {
      id: "local-two",
      stash_ids: [{ endpoint: "https://one.example/graphql", stash_id: "scene-2" }],
    },
  ]);

  assert.deepEqual(matches, {
    "https://one.example/graphql::scene-1": [{ id: "local-one", stash_ids: [
      { endpoint: "https://one.example/graphql", stash_id: "scene-1" },
      { endpoint: "https://two.example/graphql", stash_id: "scene-1" },
    ] }],
    "https://one.example/graphql::scene-2": [{
      id: "local-two",
      stash_ids: [{ endpoint: "https://one.example/graphql", stash_id: "scene-2" }],
    }],
    "https://two.example/graphql::scene-1": [{ id: "local-one", stash_ids: [
      { endpoint: "https://one.example/graphql", stash_id: "scene-1" },
      { endpoint: "https://two.example/graphql", stash_id: "scene-1" },
    ] }],
  });
});

test("formatRecommendationReasons turns model reason codes into one user-facing explanation", () => {
  assert.equal(
    formatRecommendationReasons(["o_profile", "play_profile", "rating_profile"]),
    ""
  );
  assert.equal(
    formatRecommendationReasons(["content_similarity"]),
    "Similar content"
  );
  assert.equal(formatRecommendationReasons([]), "");
});

test("recommendationBadgeCells prefers a local rating and identifies unwatched scenes as new", () => {
  assert.deepEqual(
    recommendationBadgeCells({
      kind: "local",
      item: { watched: false },
      scene: { rating100: 90 },
    }),
    [
      { kind: "score", label: "Your rating", source: "local", tone: "rating-100-18", value: "4.5" },
      { kind: "new", label: "New" },
    ]
  );
});

test("recommendationBadgeCells uses outline semantics for predicted and public scores", () => {
  assert.deepEqual(
    recommendationBadgeCells({ item: { predicted_rating: 3.1, watched: true }, kind: "local", scene: {} }),
    [{ kind: "score", label: "Predicted rating", source: "predicted", tone: "rating-100-12", value: "3.1" }]
  );
  assert.deepEqual(
    recommendationBadgeCells({ item: { public_rating: 1.5, watched: true }, kind: "local", scene: {} }),
    [{ kind: "score", label: "Public average rating", source: "public", tone: "rating-100-6", value: "1.5" }]
  );
});

test("recommendationBadgeCells labels remote recommendations", () => {
  assert.deepEqual(
    recommendationBadgeCells({ kind: "remote", item: {}, url: "https://box.example/scenes/1" }),
    [{ kind: "remote", label: "Remote" }]
  );
});

test("describeLocalScene maps Stash metadata into generic card data", () => {
  const description = describeLocalScene({
    date: "2026-09-03",
    details: "A scene description.",
    files: [{ duration: 3723, video_codec: "h264" }, { video_codec: "hevc" }],
    groups: [{ group: { name: "Collection" } }],
    o_counter: 3,
    interactive_speed: 140,
    paths: { interactive_heatmap: "/scene/1/heatmap", preview: "/scene/1/preview", screenshot: "/scene/1/screenshot" },
    performers: [{ name: "Performer A" }, { name: "Performer B" }],
    play_count: 7,
    tags: [{ name: "Tag A" }, { name: "Tag B" }],
    title: "Scene title",
    studio: { id: "studio-1", name: "Studio One" },
  });

  assert.equal(description.attributes, undefined);
  assert.equal(description.countRail.find((entry) => entry.key === "performers").count, 2);
  assert.equal(description.countRail.find((entry) => entry.key === "media-video").label, "2 video files");
  assert.equal(description.description, "A scene description.");
  assert.deepEqual(description.thumbnail, {
    heatmap: "/scene/1/heatmap",
    preview: "/scene/1/preview",
    screenshot: "/scene/1/screenshot",
  });
  assert.equal(description.duration, "1:02:03");
  assert.equal(description.date, "2026-09-03");
  assert.equal(description.interactiveSpeed, 140);
  assert.deepEqual(description.studio, { id: "studio-1", name: "Studio One" });
  assert.equal(description.title, "Scene title");
});

test("describeLocalScene exposes a heatmap only for scenes with interactive speed", () => {
  const description = describeLocalScene({
    interactive: true,
    interactive_speed: 0,
    paths: { interactive_heatmap: "/scene/1/heatmap" },
  });

  assert.equal(description.thumbnail.heatmap, "");
});

test("sceneCountRailEntries preserves entity details and splits media by type", () => {
  const entries = sceneCountRailEntries({
    files: [
      { basename: "scene-a.mp4", video_codec: "h264" },
      { basename: "scene-b.mkv", video_codec: "hevc" },
      { audio_codec: "flac", basename: "audio.flac" },
    ],
    groups: [{ group: { id: "group-1", name: "Collection" } }],
    interactive: true,
    o_counter: 3,
    performers: [{ id: "performer-1", name: "Performer A" }, { id: "performer-2", name: "Performer B" }],
    play_count: 7,
    scene_markers: [{ id: "marker-1", title: "Marker A" }, { id: "marker-2", title: "Marker B" }],
    tags: [{ id: "tag-1", name: "Tag A" }, { id: "tag-2", name: "Tag B" }],
  });

  assert.deepEqual(entries, [
    {
      count: 2,
      items: [{ id: "performer-1", name: "Performer A" }, { id: "performer-2", name: "Performer B" }],
      key: "performers",
      kind: "performers",
      label: "2 performers",
    },
    {
      count: 2,
      items: [{ id: "tag-1", name: "Tag A" }, { id: "tag-2", name: "Tag B" }],
      key: "tags",
      kind: "tags",
      label: "2 tags",
    },
    {
      count: 1,
      items: [{ id: "group-1", name: "Collection" }],
      key: "groups",
      kind: "groups",
      label: "1 group",
    },
    {
      count: 2,
      items: [{ id: "marker-1", name: "Marker A" }, { id: "marker-2", name: "Marker B" }],
      key: "markers",
      kind: "markers",
      label: "2 markers",
    },
    { count: 7, items: [], key: "plays", kind: "plays", label: "7 plays" },
    { count: 3, items: [], key: "o-count", kind: "o", label: "3 O events" },
    {
      count: 2,
      items: [{ name: "scene-a.mp4" }, { name: "scene-b.mkv" }],
      key: "media-video",
      kind: "video",
      label: "2 video files",
    },
    {
      count: 1,
      items: [{ name: "audio.flac" }],
      key: "media-audio",
      kind: "audio",
      label: "1 audio file",
    },
    {
      count: 1,
      items: [{ name: "Interactive funscript" }],
      key: "media-funscript",
      kind: "funscript",
      label: "1 funscript",
    },
  ]);
});

test("sceneCountRailEntries preserves zero-count categories for card presentation policy", () => {
  const entries = sceneCountRailEntries({
    files: [],
    groups: [],
    o_counter: 0,
    performers: [],
    play_count: 0,
    tags: [],
  });

  assert.deepEqual(
    entries.map((entry) => [entry.key, entry.count]),
    [["performers", 0], ["tags", 0], ["groups", 0], ["markers", 0], ["plays", 0], ["o-count", 0]]
  );
});

test("describeLocalScene keeps every high-cardinality entity available to the count popover", () => {
  const description = describeLocalScene({
    files: [],
    groups: [{ group: { name: "Collection A" } }, { group: { name: "Collection B" } }, { group: { name: "Collection C" } }, { group: { name: "Collection D" } }],
    performers: [{ name: "Performer A" }, { name: "Performer B" }, { name: "Performer C" }, { name: "Performer D" }],
    tags: [{ name: "Tag A" }, { name: "Tag B" }, { name: "Tag C" }, { name: "Tag D" }, { name: "Tag E" }],
  });

  assert.equal(description.countRail.find((entry) => entry.key === "performers").count, 4);
  assert.equal(description.countRail.find((entry) => entry.key === "tags").count, 5);
  assert.equal(description.countRail.find((entry) => entry.key === "groups").count, 4);
  assert.equal(description.countRail.find((entry) => entry.key === "performers").items.at(-1).name, "Performer D");
  assert.equal(description.countRail.find((entry) => entry.key === "tags").items.at(-1).name, "Tag E");
});

test("partitionRecommendations separates watched, unwatched, and remote candidates", () => {
  const items = [
    { content_key: { endpoint: "https://box.example/graphql", stash_id: "watched" }, watched: true },
    { content_key: { endpoint: "https://box.example/graphql", stash_id: "unwatched" }, watched: false },
    { content_key: { endpoint: "https://box.example/graphql", stash_id: "remote" }, watched: false, canonical_url: "https://box.example/scenes/remote" },
  ];

  const sections = partitionRecommendations(
    items,
    (item) => item.content_key.stash_id === "remote" ? [] : [{ id: item.content_key.stash_id, title: item.content_key.stash_id }],
    true
  );

  assert.deepEqual(sections.watched.map((entry) => entry.scene.id), ["watched"]);
  assert.deepEqual(sections.unwatched.map((entry) => entry.scene.id), ["unwatched"]);
  assert.deepEqual(sections.remote.map((entry) => entry.url), ["https://box.example/scenes/remote"]);
});

test("partitionRecommendations classifies local play history as watched", () => {
  const sections = partitionRecommendations(
    [{ content_key: { endpoint: "https://box.example/graphql", stash_id: "scene-1" } }],
    () => [{ id: "scene-1", title: "Local Scene", play_history: ["2026-09-02T00:00:00Z"], o_history: [] }],
    false
  );

  assert.equal(sections.watched.length, 1);
  assert.equal(sections.unwatched.length, 0);
});

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

test("fetchForYou forwards numeric filters through the plugin task proxy", async () => {
  const calls = [];

  await fetchForYou(8, 16, async (args) => {
    calls.push(args);
    return { items: [], model_version: "model-2" };
  }, {
    rating: { operator: "gte", value: 4 },
    o_count: { operator: "not_null" },
  });

  assert.deepEqual(calls, [{
    mode: "fetch-for-you",
    limit: 8,
    offset: 16,
    filters: {
      rating: { operator: "gte", value: 4 },
      o_count: { operator: "not_null" },
    },
  }]);
});

test("For You exposes rating and O-count controls and changes the reload dependency", () => {
  const filters = {
    rating: { operator: "gte", value: 4 },
    o_count: { operator: "not_null" },
  };

  assert.deepEqual(forYouFilterFields(filters), [
    {
      key: "rating",
      label: "Rating",
      operators: ["gt", "gte", "lt", "lte", "eq", "is_null", "not_null"],
      value: { operator: "gte", value: 4 },
    },
    {
      key: "o_count",
      label: "O count",
      operators: ["gt", "gte", "lt", "lte", "eq", "is_null", "not_null"],
      value: { operator: "not_null" },
    },
  ]);
  assert.notEqual(forYouFiltersKey(filters), forYouFiltersKey({}));
});

test("forYouOffsetForPage loads the next server batch only after the current batch is exhausted", () => {
  assert.equal(forYouOffsetForPage(1, 24), 0);
  assert.equal(forYouOffsetForPage(4, 24), 0);
  assert.equal(forYouOffsetForPage(5, 24), 100);
  assert.equal(forYouOffsetForPage(3, 48), 100);
  assert.equal(forYouOffsetForPage(9, 24), 200);
});

test("For You keeps consuming complete server batches until a visible page can continue", () => {
  assert.equal(
    shouldFetchAnotherForYouBatch({ batchSize: 100, receivedCount: 100, visibleCount: 24 }),
    true
  );
  assert.equal(
    shouldFetchAnotherForYouBatch({ batchSize: 100, receivedCount: 100, visibleCount: 49 }),
    false
  );
  assert.equal(
    shouldFetchAnotherForYouBatch({ batchSize: 100, receivedCount: 37, visibleCount: 3 }),
    false
  );
});

test("plugin UI registers route, nav, scene tab, and never renders a seeded API key", () => {
  const state = {
    error: "",
    items: [],
    loading: false,
    modelVersion: "",
    status: {
      configured: true,
      settings: {
        api_key: "seeded-secret-api-key",
        api_key_configured: true,
        show_remote_results: false,
      },
      outbox: {
        pending: {},
        delivered: {},
        paused: {
          active: true,
          reason: "service authentication failed",
        },
      },
      metadata: {
        jobs: { pending: 3, in_progress: 0, completed: 7, failed: 0 },
        diagnostics: [{ endpoint: "https://box.example/graphql", stash_id: "scene-1", attempts: 2, last_error: "timed out" }],
      },
    },
  };
  const { routes, patches } = registerUi(state);
  const route = routes.find((entry) => entry.path === "/plugins/stash-recommendations");

  assert.ok(route);
  assert.deepEqual(
    patches.map((entry) => entry.name),
    ["ScenePage.Tabs", "ScenePage.TabContent", "MainNavBar.MenuItems"]
  );

  const markup = renderMarkup(route.component({}));

  assert.match(markup, /service authentication failed/);
  assert.match(markup, /Metadata: pending 3, in progress 0, completed 7/);
  assert.match(markup, /Metadata retry: scene-1 .* timed out/);
  assert.doesNotMatch(markup, /seeded-secret-api-key/);
});

test("For You renders local recommendations through the shared ranked collection surface", () => {
  const calls = [];
  const { routes } = registerUi(
    {
      error: "",
      items: [
        {
          kind: "local",
          item: { watched: false },
          scene: { id: "44", title: "Local Scene", paths: { screenshot: "/scene/44/screenshot" } },
          reasons: ["play_profile"],
          score: 0.9,
        },
      ],
      loading: false,
      modelVersion: "model-1",
      status: {
        configured: true,
        settings: { api_key_configured: true, show_remote_results: false },
        outbox: { pending: {}, delivered: {}, paused: { active: false, reason: null } },
      },
    },
    {
      renderRankedCollectionSurface(runtime, props) {
        calls.push(props);
        return runtime.React.createElement("ranked-collection", { title: props.title });
      },
    }
  );

  const route = routes.find((entry) => entry.path === "/plugins/stash-recommendations");
  route.component({});

  assert.equal(calls.length, 1);
  assert.equal(calls[0].title, "For You");
  assert.deepEqual(calls[0].ranked, [{ key: "44", score: 0.9 }]);
  assert.deepEqual(calls[0].pagination, {
    onPageChange: calls[0].pagination.onPageChange,
    onPageSizeChange: calls[0].pagination.onPageSizeChange,
    page: 1,
    pageSize: 24,
    pageSizeOptions: [12, 24, 48],
  });
  assert.deepEqual(calls[0].sort.options, [
    { label: "Recommended", value: "score" },
    { label: "Title: A-Z", value: "title-asc" },
    { label: "Title: Z-A", value: "title-desc" },
  ]);
  assert.equal(calls[0].sort.value, "score");
});

test("local recommendation lookup requests the native SceneCard collection fields", () => {
  const source = require("node:fs").readFileSync(modulePath, "utf8");
  const query = source.match(/query FindLocalRecommendationScenes[\s\S]*?\n    `;/)?.[0] || "";

  for (const field of ["files", "galleries", "groups", "performers", "scene_markers", "tags"]) {
    assert.match(query, new RegExp(`\\b${field}\\s*\\{`), `missing SceneCard field: ${field}`);
  }
  assert.match(query, /organized/);
  assert.match(query, /fingerprints\s*\{/);
  assert.match(query, /vtt/);
});

test("For You does not render local scenes through the generic entity card surface", () => {
  const cards = [];
  const { routes } = registerUi(
    {
      error: "",
      items: [
        {
          kind: "local",
          item: { watched: false },
          scene: {
            files: [{ duration: 3723, video_codec: "h264" }],
            id: "44",
            interactive_speed: 140,
            date: "2026-09-03",
            o_counter: 2,
            paths: { interactive_heatmap: "/scene/44/heatmap", preview: "/scene/44/preview", screenshot: "/scene/44/screenshot" },
            performers: [{ name: "Performer" }],
            play_count: 3,
            rating100: 90,
            scene_markers: [{ id: "marker-1", title: "Marker One" }],
            tags: [{ name: "Tag" }],
            title: "Local Scene",
            studio: { id: "studio-1", name: "Studio One" },
          },
          reasons: ["play_profile"],
          score: 0.9,
        },
      ],
      loading: false,
      modelVersion: "model-1",
      status: {
        configured: true,
        settings: { api_key_configured: true, show_remote_results: false },
        outbox: { pending: {}, delivered: {}, paused: { active: false, reason: null } },
      },
    },
    {
      renderEntityCard(runtime, props) {
        cards.push(props);
        return runtime.React.createElement("entity-card", null);
      },
      renderRankedCollectionSurface(runtime, props) {
        return props.renderItem({ item: props.items[0] }, 0);
      },
    }
  );

  routes.find((entry) => entry.path === "/plugins/stash-recommendations").component({});

  assert.equal(cards.length, 0);
});

function registerUi(state, StashPluginComponents) {
  const routes = [];
  const patches = [];
  const React = createReactHarness(state);
  const PluginApi = {
    React,
    libraries: {
      Apollo: {
        gql(strings, ...values) {
          return strings.reduce((joined, part, index) => joined + part + (values[index] ?? ""), "");
        },
      },
      Bootstrap: {
        Nav: { Item: "nav-item", Link: "nav-link" },
        Tab: { Pane: "tab-pane" },
        Button: "button",
      },
      FontAwesomeSolid: {
        faFile: "file",
        faFileAudio: "file-audio",
        faFileCode: "file-code",
        faFileVideo: "file-video",
        faEye: "eye",
        faFilm: "film",
        faMapMarkerAlt: "location-dot",
        faTag: "tag",
        faUser: "user",
      },
      ReactFontAwesome: {
        FontAwesomeIcon: "font-awesome-icon",
      },
      ReactRouterDOM: {
        NavLink: "nav-link",
      },
    },
    utils: {
      StashService: {
        getClient() {
          return {
            mutate: async () => ({ data: { runPluginOperation: JSON.stringify({ output: { items: [], model_version: "" } }) } }),
            query: async () => ({ data: {} }),
          };
        },
      },
    },
    register: {
      route(path, component) {
        routes.push({ path, component });
      },
    },
    patch: {
      before(name, apply) {
        patches.push({ name, apply });
      },
    },
  };

  require(modulePath).register(PluginApi, {
    location: { pathname: "/scenes/44", hash: "" },
    StashPluginComponents,
  });

  return { routes, patches };
}

function createReactHarness(state) {
  const Fragment = Symbol("Fragment");

  return {
    Fragment,
    createElement(type, props, ...children) {
      const normalizedProps = {
        ...(props || {}),
        children: children.length <= 1 ? children[0] : children,
      };
      if (typeof type === "function") {
        return type(normalizedProps);
      }
      return { type, props: normalizedProps };
    },
    useEffect() {},
    useState() {
      return [state, () => {}];
    },
  };
}

function renderMarkup(node) {
  if (node == null || node === false) {
    return "";
  }
  if (Array.isArray(node)) {
    return node.map(renderMarkup).join("");
  }
  if (typeof node === "string" || typeof node === "number") {
    return String(node);
  }
  if (typeof node.type === "symbol") {
    return renderMarkup(node.props?.children);
  }
  const props = node.props || {};
  const attributes = Object.entries(props)
    .filter(([key, value]) => key !== "children" && value != null && typeof value !== "function")
    .map(([key, value]) => ` ${key}=\"${String(value)}\"`)
    .join("");
  return `<${String(node.type)}${attributes}>${renderMarkup(props.children)}</${String(node.type)}>`;
}

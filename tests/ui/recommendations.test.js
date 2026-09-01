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
  assert.doesNotMatch(markup, /seeded-secret-api-key/);
});

function registerUi(state) {
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

  require(modulePath).register(PluginApi, { location: { pathname: "/scenes/44", hash: "" } });

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

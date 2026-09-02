(function (root, factory) {
  const api = factory();
  if (typeof module === "object" && module.exports) {
    module.exports = api;
  }
  if (root && root.PluginApi) {
    api.register(root.PluginApi, root);
  }
})(typeof globalThis !== "undefined" ? globalThis : this, function () {
  function fetchRelated(contentKeys, runPluginOperation, limit = 20) {
    return runPluginOperation({
      mode: "fetch-related",
      content_keys: contentKeys,
      limit,
    }).then(normalizeRecommendationResponse);
  }

  function fetchForYou(limit, runPluginOperation) {
    return runPluginOperation({
      mode: "fetch-for-you",
      limit,
    }).then(normalizeRecommendationResponse);
  }

  function resolveLocalRecommendations(items, findScenes, showRemote) {
    return (Array.isArray(items) ? items : []).flatMap((item) => {
      const scenes = findScenes(item) || [];
      if (scenes.length > 0) {
        return [
          {
            kind: "local",
            item,
            scene: scenes[0],
            reasons: Array.isArray(item.reasons) ? item.reasons : [],
            score: Number(item.score || 0),
          },
        ];
      }
      if (showRemote && item?.canonical_url) {
        return [
          {
            kind: "remote",
            item,
            url: item.canonical_url,
            reasons: Array.isArray(item.reasons) ? item.reasons : [],
            score: Number(item.score || 0),
          },
        ];
      }
      return [];
    });
  }

  function partitionRecommendations(items, findScenes, showRemote) {
    const sections = { watched: [], unwatched: [], remote: [] };
    for (const entry of resolveLocalRecommendations(items, findScenes, showRemote)) {
      if (entry.kind === "remote") {
        sections.remote.push(entry);
      } else if (entry.item?.watched === true || localSceneWasWatched(entry.scene)) {
        sections.watched.push(entry);
      } else {
        sections.unwatched.push(entry);
      }
    }
    return sections;
  }

  function localSceneWasWatched(scene) {
    return (Array.isArray(scene?.play_history) && scene.play_history.length > 0)
      || (Array.isArray(scene?.o_history) && scene.o_history.length > 0);
  }

  function normalizeRecommendationResponse(output) {
    if (!output || typeof output !== "object") {
      return { model_version: "", items: [] };
    }
    return {
      model_version: String(output.model_version || ""),
      items: Array.isArray(output.items) ? output.items : [],
    };
  }

  function register(PluginApi, root) {
    const React = PluginApi.React;
    const Apollo = PluginApi.libraries.Apollo;
    const { Nav, Tab, Button } = PluginApi.libraries.Bootstrap;
    const { NavLink } = PluginApi.libraries.ReactRouterDOM;
    const getClient = PluginApi.utils.StashService.getClient;
    const RUN_PLUGIN_OPERATION = Apollo.gql`
      mutation RunPluginOperation($plugin_id: ID!, $args: Map) {
        runPluginOperation(plugin_id: $plugin_id, args: $args)
      }
    `;
    const FIND_SCENE = Apollo.gql`
      query FindSceneForRecommendations($id: ID!) {
        findScene(id: $id) {
          id
          title
          stash_ids {
            endpoint
            stash_id
          }
        }
      }
    `;
    const FIND_LOCAL_SCENES = Apollo.gql`
      query FindLocalRecommendationScenes($endpoint: String!, $stash_id: String!) {
        findScenes(
          scene_filter: {
            stash_id_endpoint: {
              endpoint: $endpoint
              stash_id: $stash_id
              modifier: EQUALS
            }
          }
          filter: { per_page: 5 }
        ) {
          scenes {
            id
            title
            play_history
            o_history
            paths {
              screenshot
            }
            stash_ids {
              endpoint
              stash_id
            }
          }
        }
      }
    `;
    const PLUGIN_ID = "stashRecommendations";

    function keyFor(contentKey) {
      return `${contentKey.endpoint}::${contentKey.stash_id}`;
    }

    async function runPluginOperation(args) {
      const response = await getClient().mutate({
        mutation: RUN_PLUGIN_OPERATION,
        variables: {
          plugin_id: PLUGIN_ID,
          args,
        },
      });
      const output = response.data?.runPluginOperation;
      if (typeof output === "string") {
        const parsed = JSON.parse(output);
        return parsed.output ?? parsed;
      }
      return output?.output ?? output;
    }

    function resolveSceneId(props) {
      if (props.scene?.id || props.sceneId) {
        return String(props.scene?.id ?? props.sceneId);
      }
      const sources = [root.location?.pathname, root.location?.hash].filter(Boolean);
      for (const source of sources) {
        const match = String(source).match(/\/scenes\/(\d+)/);
        if (match?.[1]) {
          return match[1];
        }
      }
      return "";
    }

    async function loadScene(sceneId) {
      const response = await getClient().query({
        query: FIND_SCENE,
        variables: { id: sceneId },
        fetchPolicy: "network-only",
      });
      return response.data?.findScene ?? null;
    }

    async function loadLocalSceneMatches(items) {
      const matches = {};
      for (const item of Array.isArray(items) ? items : []) {
        const contentKey = item?.content_key;
        if (!contentKey?.endpoint || !contentKey?.stash_id) {
          continue;
        }
        const response = await getClient().query({
          query: FIND_LOCAL_SCENES,
          variables: {
            endpoint: contentKey.endpoint,
            stash_id: contentKey.stash_id,
          },
          fetchPolicy: "network-only",
        });
        matches[keyFor(contentKey)] = response.data?.findScenes?.scenes ?? [];
      }
      return matches;
    }

    function RecommendationState(props) {
      return React.createElement(
        "div",
        { className: "stash-recommendations__state" },
        props.children
      );
    }

    function RecommendationCard(props) {
      if (props.entry.kind === "local") {
        return React.createElement(
          "a",
          {
            className: "stash-recommendations__card",
            href: `/scenes/${props.entry.scene.id}`,
          },
          React.createElement("div", { className: "stash-recommendations__card-title" }, props.entry.scene.title || "[Untitled Scene]"),
          React.createElement(
            "div",
            { className: "stash-recommendations__card-meta" },
            props.entry.reasons.join(", ")
          )
        );
      }
      return React.createElement(
        "a",
        {
          className: "stash-recommendations__card stash-recommendations__card--remote",
          href: props.entry.url,
          rel: "noreferrer",
          target: "_blank",
        },
        React.createElement("div", { className: "stash-recommendations__card-title" }, "Open remote scene"),
        React.createElement(
          "div",
          { className: "stash-recommendations__card-meta" },
          props.entry.reasons.join(", ")
        )
      );
    }

    function OutboxSummary(props) {
      const outbox = props.status?.outbox ?? {};
      const pending = outbox.pending ?? {};
      const delivered = outbox.delivered ?? {};
      return React.createElement(
        "div",
        { className: "stash-recommendations__summary" },
        React.createElement(
          "div",
          null,
          `Pending: ratings ${pending.rating || 0}, plays ${pending.play || 0}, o ${pending.o || 0}, snapshots ${pending.snapshot || 0}`
        ),
        React.createElement(
          "div",
          null,
          `Delivered: ratings ${delivered.rating || 0}, plays ${delivered.play || 0}, o ${delivered.o || 0}, snapshots ${delivered.snapshot || 0}`
        )
      );
    }

    function useRecommendationState(loader, dependency) {
      const [state, setState] = React.useState({
        error: "",
        items: [],
        loading: true,
        modelVersion: "",
        status: null,
      });

      React.useEffect(() => {
        let active = true;
        async function runLoad() {
          setState({
            error: "",
            items: [],
            loading: true,
            modelVersion: "",
            status: null,
          });
          try {
            const status = await runPluginOperation({ mode: "status" });
            if (!active) {
              return;
            }
            if (!status?.configured) {
              setState({
                error: "",
                items: [],
                loading: false,
                modelVersion: "",
                status,
              });
              return;
            }
            if (status?.outbox?.paused?.active) {
              setState({
                error: "",
                items: [],
                loading: false,
                modelVersion: "",
                status,
              });
              return;
            }
            const response = await loader(status);
            const localMatches = await loadLocalSceneMatches(response.items);
            const resolved = resolveLocalRecommendations(
              response.items,
              (item) => localMatches[keyFor(item.content_key)] || [],
              Boolean(status.settings?.show_remote_results)
            );
            if (!active) {
              return;
            }
            setState({
              error: "",
              items: resolved,
              loading: false,
              modelVersion: response.model_version,
              status,
            });
          } catch (error) {
            if (!active) {
              return;
            }
            setState({
              error: String(error),
              items: [],
              loading: false,
              modelVersion: "",
              status: null,
            });
          }
        }
        runLoad();
        return function cleanup() {
          active = false;
        };
      }, [dependency]);

      return state;
    }

    function RecommendationPanel(props) {
      const state = useRecommendationState(props.loader, props.dependency);

      if (state.loading) {
        return React.createElement(RecommendationState, null, "Loading recommendations...");
      }
      if (state.error) {
        return React.createElement(RecommendationState, null, state.error);
      }
      if (!state.status?.configured || !state.status?.settings?.api_key_configured) {
        return React.createElement(
          RecommendationState,
          null,
          "Configure the StashRec service URL and API key."
        );
      }
      if (state.status?.outbox?.paused?.active) {
        return React.createElement(
          RecommendationState,
          null,
          state.status.outbox.paused.reason || "Service authentication is paused."
        );
      }
      if (!state.items.length) {
        return React.createElement(
          React.Fragment,
          null,
          React.createElement(OutboxSummary, { status: state.status }),
          React.createElement(RecommendationState, null, "No recommendations yet.")
        );
      }
      const sections = {
        watched: state.items.filter((entry) => entry.kind === "local" && (entry.item?.watched === true || localSceneWasWatched(entry.scene))),
        unwatched: state.items.filter((entry) => entry.kind === "local" && entry.item?.watched !== true && !localSceneWasWatched(entry.scene)),
        remote: state.items.filter((entry) => entry.kind === "remote"),
      };
      return React.createElement(
        "div",
        { className: "stash-recommendations" },
        React.createElement(OutboxSummary, { status: state.status }),
        React.createElement(
          "div",
          { className: "stash-recommendations__header" },
          React.createElement("h2", null, props.title),
          React.createElement("div", { className: "stash-recommendations__version" }, state.modelVersion || "")
        ),
        ["unwatched", "watched", "remote"].map((name) => {
          const entries = sections[name];
          if (!entries.length) return null;
          return React.createElement(
            "section",
            { className: "stash-recommendations__section", key: name },
            React.createElement("h3", null, props.sectionTitles[name]),
            React.createElement(
              "div",
              { className: "stash-recommendations__grid" },
              entries.map((entry, index) => React.createElement(RecommendationCard, {
                entry,
                key: `${entry.kind}:${entry.kind === "local" ? entry.scene.id : entry.url}:${index}`,
              }))
            )
          );
        })
      );
    }

    function SceneRecommendationsPanel(props) {
      const sceneId = resolveSceneId(props);
      return React.createElement(RecommendationPanel, {
        dependency: sceneId,
        loader: async function () {
          if (!sceneId) {
            return { model_version: "", items: [] };
          }
          const scene = props.scene ?? (await loadScene(sceneId));
          const contentKeys = Array.isArray(scene?.stash_ids) ? scene.stash_ids : [];
          if (!contentKeys.length) {
            return { model_version: "", items: [] };
          }
          return fetchRelated(contentKeys, runPluginOperation);
        },
        title: "Related Scenes",
        sectionTitles: {
          unwatched: "Watch next: unwatched items from your stash",
          watched: "Watch again from your stash",
          remote: "Grow your stash with related content",
        },
      });
    }

    function ForYouPage() {
      return React.createElement(RecommendationPanel, {
        dependency: "for-you",
        loader: function () {
          return fetchForYou(20, runPluginOperation);
        },
        title: "For You",
        sectionTitles: {
          unwatched: "Explore your stash",
          watched: "Watch again from your stash",
          remote: "Grow your stash",
        },
      });
    }

    PluginApi.register.route("/plugins/stash-recommendations", ForYouPage);

    PluginApi.patch.before("ScenePage.Tabs", function (props) {
      return [
        {
          children: React.createElement(
            React.Fragment,
            null,
            props.children,
            React.createElement(
              Nav.Item,
              null,
              React.createElement(Nav.Link, { eventKey: "stash-recommendations-scene-panel" }, "Recommendations")
            )
          ),
        },
      ];
    });

    PluginApi.patch.before("ScenePage.TabContent", function (props) {
      return [
        {
          children: React.createElement(
            React.Fragment,
            null,
            props.children,
            React.createElement(
              Tab.Pane,
              { eventKey: "stash-recommendations-scene-panel" },
              React.createElement(SceneRecommendationsPanel, props)
            )
          ),
        },
      ];
    });

    PluginApi.patch.before("MainNavBar.MenuItems", function (props) {
      return [
        {
          children: React.createElement(
            React.Fragment,
            null,
            props.children,
            React.createElement(
              Nav.Link,
              {
                as: "div",
                className: "col-4 col-sm-3 col-md-2 col-lg-auto",
                eventKey: "/plugins/stash-recommendations",
              },
              React.createElement(
                Button,
                {
                  as: NavLink,
                  className: "minimal p-4 p-xl-2 d-flex d-xl-inline-block flex-column justify-content-between align-items-center",
                  exact: true,
                  to: "/plugins/stash-recommendations",
                },
                React.createElement("span", null, "For You")
              )
            )
          ),
        },
      ];
    });
  }

  return {
    fetchForYou,
    fetchRelated,
    partitionRecommendations,
    register,
    resolveLocalRecommendations,
  };
});

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

  function fetchForYou(limit, offset, runPluginOperation) {
    return runPluginOperation({
      mode: "fetch-for-you",
      limit,
      offset,
    }).then(normalizeRecommendationResponse);
  }

  function forYouOffsetForPage(page, pageSize, batchSize = 100) {
    const lastRequiredItem = Math.max(0, Math.floor(Number(page) || 1) * Math.max(1, Math.floor(Number(pageSize) || 1)) - 1);
    return Math.floor(lastRequiredItem / batchSize) * batchSize;
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

  function recommendationBadgeCells(entry = {}) {
    if (entry.kind === "remote") {
      return [{ kind: "remote", label: "Remote" }];
    }

    function scoreTone(score) {
      // Match Stash's RatingBanner class selection: rating100 / 5, truncated.
      return `rating-100-${Math.trunc(Math.max(0, Math.min(100, score * 20)) / 5)}`;
    }

    function scoreCell(label, source, score) {
      const normalized = Math.max(0.5, Math.min(5, Number(score)));
      const rounded = Math.round(normalized * 10) / 10;
      return {
        kind: "score",
        label,
        source,
        tone: scoreTone(rounded),
        value: Number.isInteger(rounded) ? String(rounded) : rounded.toFixed(1),
      };
    }

    const cells = [];
    const rating100 = Number(entry.scene?.rating100);
    if (Number.isFinite(rating100) && rating100 > 0) {
      cells.push(scoreCell("Your rating", "local", rating100 / 20));
    } else if (Number.isFinite(Number(entry.item?.predicted_rating))) {
      cells.push(scoreCell("Predicted rating", "predicted", entry.item.predicted_rating));
    } else if (Number.isFinite(Number(entry.item?.public_rating))) {
      cells.push(scoreCell("Public average rating", "public", entry.item.public_rating));
    }
    if (entry.item?.watched !== true && !localSceneWasWatched(entry.scene)) {
      cells.push({ kind: "new", label: "New" });
    }
    return cells;
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

  function formatRecommendationReasons(reasons) {
    const labels = {
      content_similarity: "similar content",
      o_profile: "O history",
      play_profile: "watch history",
      rating_profile: "your ratings",
      session_cooccurrence: "what you watch together",
    };
    const priority = ["rating_profile", "play_profile", "o_profile", "session_cooccurrence", "content_similarity"];
    const known = new Set(Array.isArray(reasons) ? reasons.map(String) : []);
    const values = priority.filter((reason) => known.delete(reason)).map((reason) => labels[reason]);
    for (const reason of known) {
      values.push(reason.replace(/_/g, " "));
    }
    if (!values.length) return "";
    if (values.length === 1) return `Why: ${values[0]}`;
    const last = values.pop();
    return `Why: based on ${values.join(", ")}, and ${last}`;
  }

  function namedItems(values) {
    return (Array.isArray(values) ? values : [])
      .map((value) => String(value?.name || ""))
      .map((name, index) => ({ ...values[index], name }))
      .filter((value) => value.name);
  }

  function countLabel(count, singular, plural = `${singular}s`) {
    return `${count} ${count === 1 ? singular : plural}`;
  }

  function formatDuration(seconds) {
    const totalSeconds = Math.floor(Number(seconds));
    if (!Number.isFinite(totalSeconds) || totalSeconds <= 0) {
      return "";
    }
    const hours = Math.floor(totalSeconds / 3600);
    const minutes = Math.floor((totalSeconds % 3600) / 60);
    const remainder = totalSeconds % 60;
    if (hours > 0) {
      return `${hours}:${String(minutes).padStart(2, "0")}:${String(remainder).padStart(2, "0")}`;
    }
    return `${minutes}:${String(remainder).padStart(2, "0")}`;
  }

  function sceneCountRailEntries(scene = {}) {
    const performers = namedItems(scene.performers);
    const tags = namedItems(scene.tags);
    const groups = namedItems((Array.isArray(scene.groups) ? scene.groups : []).map((entry) => entry?.group));
    const markers = namedItems((Array.isArray(scene.scene_markers) ? scene.scene_markers : []).map((marker) => ({
      id: marker?.id,
      name: marker?.title || "Scene marker",
    })));
    const filesByType = new Map();
    for (const file of Array.isArray(scene.files) ? scene.files : []) {
      const kind = file?.video_codec ? "video" : file?.audio_codec ? "audio" : "file";
      const files = filesByType.get(kind) || [];
      files.push({ name: String(file?.basename || file?.id || "Media file") });
      filesByType.set(kind, files);
    }
    const entries = [
      { count: performers.length, items: performers, key: "performers", kind: "performers", label: countLabel(performers.length, "performer") },
      { count: tags.length, items: tags, key: "tags", kind: "tags", label: countLabel(tags.length, "tag") },
      { count: groups.length, items: groups, key: "groups", kind: "groups", label: countLabel(groups.length, "group") },
      { count: markers.length, items: markers, key: "markers", kind: "markers", label: countLabel(markers.length, "marker") },
      { count: Number(scene.play_count || 0), items: [], key: "plays", kind: "plays", label: countLabel(Number(scene.play_count || 0), "play") },
      { count: Number(scene.o_counter || 0), items: [], key: "o-count", kind: "o", label: countLabel(Number(scene.o_counter || 0), "O event") },
      ...[...filesByType.entries()].map(([kind, files]) => ({
        count: files.length,
        items: files,
        key: `media-${kind}`,
        kind,
        label: countLabel(files.length, `${kind} file`),
      })),
    ];
    if (scene.interactive) {
      entries.push({
        count: 1,
        items: [{ name: "Interactive funscript" }],
        key: "media-funscript",
        kind: "funscript",
        label: "1 funscript",
      });
    }
    return entries;
  }

  function describeLocalScene(scene = {}) {
    const primaryFile = (Array.isArray(scene.files) ? scene.files : []).find((file) => Number(file?.duration) > 0);
    const studio = scene.studio?.name
      ? { id: String(scene.studio.id || ""), name: String(scene.studio.name) }
      : null;
    return {
      countRail: sceneCountRailEntries(scene),
      description: String(scene.details || ""),
      duration: formatDuration(primaryFile?.duration),
      interactiveSpeed: Number.isFinite(Number(scene.interactive_speed)) ? Number(scene.interactive_speed) : null,
      studio,
      thumbnail: {
        heatmap: Number(scene.interactive_speed) > 0 ? (scene.paths?.interactive_heatmap || "") : "",
        preview: scene.paths?.preview || "",
        screenshot: scene.paths?.screenshot || "",
      },
      title: String(scene.title || "[Untitled Scene]"),
    };
  }

  function register(PluginApi, root) {
    const React = PluginApi.React;
    const Apollo = PluginApi.libraries.Apollo;
    const { Nav, Tab, Button } = PluginApi.libraries.Bootstrap;
    const { FontAwesomeIcon } = PluginApi.libraries.ReactFontAwesome;
    const {
      faFile,
      faFileAudio,
      faFileCode,
      faFileVideo,
      faEye,
      faFilm,
      faMapMarkerAlt,
      faTag,
      faUser,
    } = PluginApi.libraries.FontAwesomeSolid;
    const { NavLink } = PluginApi.libraries.ReactRouterDOM;
    const getClient = PluginApi.utils.StashService.getClient;
    const SharedComponents = root.StashPluginComponents;
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
            details
            rating100
            play_count
            o_counter
            interactive
            interactive_speed
            play_history
            o_history
            scene_markers {
              id
              title
            }
            paths {
              screenshot
              preview
              interactive_heatmap
            }
            files {
              audio_codec
              basename
              id
              duration
              video_codec
            }
            performers {
              id
              name
            }
            tags {
              id
              name
            }
            groups {
              group {
                id
                name
              }
            }
            studio {
              id
              name
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
    const countIcons = {
      audio: faFileAudio,
      file: faFile,
      funscript: faFileCode,
      groups: faFilm,
      markers: faMapMarkerAlt,
      performers: faUser,
      plays: faEye,
      tags: faTag,
      video: faFileVideo,
    };

    function SweatDropsIcon() {
      return React.createElement(
        "span",
        null,
        React.createElement(
          "svg",
          {
            "aria-hidden": "true",
            focusable: "false",
            height: "1em",
            preserveAspectRatio: "xMidYMid meet",
            style: { transform: "rotate(360deg)" },
            viewBox: "0 0 36 36",
            width: "1em",
            xmlns: "http://www.w3.org/2000/svg",
            xmlnsXlink: "http://www.w3.org/1999/xlink",
          },
          React.createElement("path", {
            d: "M22.855.758L7.875 7.024l12.537 9.733c2.633 2.224 6.377 2.937 9.77 1.518c4.826-2.018 7.096-7.576 5.072-12.413C33.232 1.024 27.68-1.261 22.855.758zm-9.962 17.924L2.05 10.284L.137 23.529a7.993 7.993 0 0 0 2.958 7.803a8.001 8.001 0 0 0 9.798-12.65zm15.339 7.015l-8.156-4.69l-.033 9.223c-.088 2 .904 3.98 2.75 5.041a5.462 5.462 0 0 0 7.479-2.051c1.499-2.644.589-6.013-2.04-7.523z",
            fill: "currentColor",
          }),
          React.createElement("rect", { fill: "rgba(0, 0, 0, 0)", height: "36", width: "36", x: "0", y: "0" })
        )
      );
    }

    function countRailContent(entry) {
      if (entry.items.length === 0) {
        return React.createElement("span", null, entry.label);
      }
      return React.createElement(
        "div",
        { className: "stash-recommendations__count-popover-content" },
        ...entry.items.map((item, index) => {
          const href = entry.kind === "performers"
            ? `/performers/${item.id}`
            : entry.kind === "tags"
              ? `/tags/${item.id}`
              : entry.kind === "groups"
                ? `/groups/${item.id}`
                : null;
          return href
            ? React.createElement("a", { href, key: item.id || index }, item.name)
            : React.createElement("span", { key: item.name || index }, item.name);
        })
      );
    }

    function countRail(entries) {
      return entries.map((entry) => ({
        content: countRailContent(entry),
        count: String(entry.count),
        icon: entry.kind === "o"
          ? React.createElement(SweatDropsIcon)
          : React.createElement(FontAwesomeIcon, { fixedWidth: true, icon: countIcons[entry.kind] || faFile }),
        key: entry.key,
        label: entry.label,
      }));
    }

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
        return props.NativeSceneCard
          ? React.createElement(props.NativeSceneCard, {
              scene: props.entry.scene,
              selecting: props.selecting,
              selected: props.selectedLocalIds.has(String(props.entry.scene.id)),
              onSelectedChanged: (selected) => props.onLocalSelectionChanged(
                String(props.entry.scene.id),
                selected,
              ),
            })
          : React.createElement("div", { className: "stash-recommendations__card-loading" }, "Loading scene card...");
      }

      if (SharedComponents?.renderEntityCard) {
        const badgeCells = recommendationBadgeCells(props.entry);
        const badgeRail = badgeCells.length
          ? React.createElement(
              "div",
              { className: "stash-recommendations__badge-cells" },
              ...badgeCells.map((cell) => React.createElement(
                "span",
                {
                  className: [
                    "stash-recommendations__badge-cell",
                    `stash-recommendations__badge-cell--${cell.kind}`,
                    cell.source ? `stash-recommendations__badge-cell--${cell.source}` : "",
                    cell.tone ? `stash-recommendations__badge-cell--${cell.tone}` : "",
                  ].filter(Boolean).join(" "),
                  key: cell.kind,
                  title: cell.label,
                },
                cell.value || cell.label
              ))
            )
          : null;
        if (props.entry.kind === "local") {
          const watched = props.entry.item?.watched === true || localSceneWasWatched(props.entry.scene);
          const card = describeLocalScene(props.entry.scene);
          const heatmap = card.thumbnail.heatmap
            ? React.createElement("img", {
                alt: "Motion heatmap",
                className: "stash-recommendations__heatmap",
                src: card.thumbnail.heatmap,
              })
            : null;
          const posterOverlay = React.createElement(
            "div",
            { className: "stash-recommendations__poster-overlays" },
            card.studio
              ? React.createElement(
                  "a",
                  {
                    className: "stash-recommendations__poster-overlay stash-recommendations__poster-overlay--studio",
                    href: `/studios/${card.studio.id}`,
                  },
                  card.studio.name
                )
              : null,
            card.duration
              ? React.createElement(
                  "span",
                  { className: "stash-recommendations__poster-overlay stash-recommendations__poster-overlay--duration" },
                  card.duration
                )
              : null,
            card.interactiveSpeed
              ? React.createElement(
                  "span",
                  { className: "stash-recommendations__poster-overlay stash-recommendations__poster-overlay--speed" },
                  card.interactiveSpeed
                )
              : null
          );
          return SharedComponents.renderEntityCard(
            { React },
            {
              badgeRail,
              className: "stash-recommendations__entity-card",
              countRail: countRail(card.countRail),
              description: card.description,
              footer: formatRecommendationReasons(props.entry.reasons)
                ? React.createElement("span", null, formatRecommendationReasons(props.entry.reasons))
                : null,
              mediaRail: heatmap,
              style: watched ? undefined : { borderColor: "var(--bs-warning, #d39e00)" },
              showZeroCounts: false,
              thumbnail: card.thumbnail.screenshot
                ? {
                    alt: "",
                    href: `/scenes/${props.entry.scene.id}`,
                    overlay: posterOverlay,
                    previewSrc: card.thumbnail.preview,
                    src: card.thumbnail.screenshot,
                  }
                : null,
              title: React.createElement(
                "a",
                { className: "stash-recommendations__entity-title", href: `/scenes/${props.entry.scene.id}` },
                card.title
              ),
            }
          );
        }
        return SharedComponents.renderEntityCard(
          { React },
          {
            badgeRail,
            className: "stash-recommendations__entity-card stash-recommendations__entity-card--remote",
            footer: formatRecommendationReasons(props.entry.reasons)
              ? React.createElement("span", null, formatRecommendationReasons(props.entry.reasons))
              : null,
            style: {
              backgroundColor: "rgba(255, 255, 255, 0.02)",
              borderColor: "rgba(255, 255, 255, 0.22)",
              textColor: "rgba(255, 255, 255, 0.72)",
            },
            title: React.createElement(
              "a",
              {
                className: "stash-recommendations__entity-title",
                href: props.entry.url,
                rel: "noreferrer",
                target: "_blank",
              },
              "Open remote scene"
            ),
          }
        );
      }
      if (props.entry.kind === "local") {
        const screenshot = props.entry.scene.paths?.screenshot;
        return React.createElement(
          "a",
          {
            className: "stash-recommendations__card",
            href: `/scenes/${props.entry.scene.id}`,
          },
          screenshot
            ? React.createElement("img", {
                alt: "",
                className: "stash-recommendations__card-image",
                loading: "lazy",
                src: screenshot,
              })
            : null,
          React.createElement("div", { className: "stash-recommendations__card-title" }, props.entry.scene.title || "[Untitled Scene]"),
          React.createElement(
            "div",
            { className: "stash-recommendations__card-meta" },
            formatRecommendationReasons(props.entry.reasons)
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
          formatRecommendationReasons(props.entry.reasons)
        )
      );
    }

    function recommendationTitle(entry) {
      if (entry.kind === "local") {
        return String(entry.scene?.title || "");
      }
      return String(entry.item?.title || entry.url || "");
    }

    function renderRecommendationSection(name, entries, title, controls, NativeSceneCard, selection) {
      if (!SharedComponents?.renderRankedCollectionSurface) {
        return React.createElement(
          "section",
          { className: "stash-recommendations__section", key: name },
          React.createElement("h3", null, title),
          React.createElement(
            "div",
            { className: "stash-recommendations__grid" },
            entries.map((entry, index) => React.createElement(RecommendationCard, {
              entry,
              key: `${entry.kind}:${entry.kind === "local" ? entry.scene.id : entry.url}:${index}`,
              NativeSceneCard,
              ...selection,
            }))
          )
        );
      }

      return SharedComponents.renderRankedCollectionSurface(
        { React, Spinner: "span" },
        {
          headingElement: "h3",
          items: entries,
          itemsClassName: "stash-recommendations__grid",
          keyOf: (entry) => entry.kind === "local" ? entry.scene.id : entry.url,
          ranked: entries.map((entry) => ({
            key: entry.kind === "local" ? entry.scene.id : entry.url,
            score: entry.score,
          })),
          renderItem: (record, index) => React.createElement(RecommendationCard, {
            entry: record.item,
            key: `${record.item.kind}:${record.item.kind === "local" ? record.item.scene.id : record.item.url}:${index}`,
            NativeSceneCard,
            ...selection,
          }),
          pagination: {
            onPageChange: controls.onPageChange,
            onPageSizeChange: controls.onPageSizeChange,
            page: controls.page,
            pageSize: controls.pageSize,
            pageSizeOptions: [12, 24, 48],
          },
          sort: {
            compare: controls.sort === "title-asc"
              ? (left, right) => recommendationTitle(left.item).localeCompare(recommendationTitle(right.item))
              : controls.sort === "title-desc"
                ? (left, right) => recommendationTitle(right.item).localeCompare(recommendationTitle(left.item))
                : undefined,
            onChange: controls.onSortChange,
            options: [
              { label: "Recommended", value: "score" },
              { label: "Title: A-Z", value: "title-asc" },
              { label: "Title: Z-A", value: "title-desc" },
            ],
            value: controls.sort,
          },
          title,
        }
      );
    }

    function OutboxSummary(props) {
      const outbox = props.status?.outbox ?? {};
      const pending = outbox.pending ?? {};
      const delivered = outbox.delivered ?? {};
      const metadata = props.status?.metadata ?? {};
      const metadataJobs = metadata.jobs ?? {};
      const diagnostic = Array.isArray(metadata.diagnostics) ? metadata.diagnostics[0] : null;
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
        ),
        React.createElement(
          "div",
          null,
          `Metadata: pending ${metadataJobs.pending || 0}, in progress ${metadataJobs.in_progress || 0}, completed ${metadataJobs.completed || 0}`
        ),
        diagnostic
          ? React.createElement(
              "div",
              null,
              `Metadata retry: ${diagnostic.stash_id} (${diagnostic.endpoint}): ${diagnostic.last_error}`
            )
          : null
      );
    }

    function useRecommendationState(loader, dependency, options = {}) {
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
          setState((previous) => options.append
            ? { ...previous, error: "", loading: true }
            : {
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
            setState((previous) => ({
              error: "",
              items: options.append ? [...previous.items, ...resolved] : resolved,
              loading: false,
              modelVersion: response.model_version,
              status,
            }));
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
      const componentsLoading = PluginApi.hooks.useLoadComponents([
        PluginApi.loadableComponents.SceneCard,
      ]);
      const state = useRecommendationState(props.loader, props.dependency, { append: props.append });
      const [selectedLocalIds, setSelectedLocalIds] = React.useState(() => new Set());
      const [sectionControls, setSectionControls] = React.useState({});
      function onLocalSelectionChanged(sceneId, selected) {
        setSelectedLocalIds((previous) => {
          const next = new Set(previous);
          if (selected) next.add(sceneId);
          else next.delete(sceneId);
          return next;
        });
      }

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
          React.Fragment,
          null,
          React.createElement(OutboxSummary, { status: state.status }),
          React.createElement(
            RecommendationState,
            null,
            state.status.outbox.paused.reason || "Service authentication is paused."
          )
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
        local: state.items.filter((entry) => entry.kind === "local"),
        watched: state.items.filter((entry) => entry.kind === "local" && (entry.item?.watched === true || localSceneWasWatched(entry.scene))),
        unwatched: state.items.filter((entry) => entry.kind === "local" && entry.item?.watched !== true && !localSceneWasWatched(entry.scene)),
        remote: state.items.filter((entry) => entry.kind === "remote"),
      };
      function controlsFor(name) {
        const current = sectionControls[name] ?? {};
        const update = (changes) => setSectionControls((previous) => ({
          ...previous,
          [name]: {
            page: 1,
            pageSize: 24,
            sort: "score",
            ...previous[name],
            ...changes,
          },
        }));
        return {
          onPageChange: (page) => {
            update({ page });
            props.onSectionPageChange?.(name, page, current.pageSize ?? 24);
          },
          onPageSizeChange: (pageSize) => {
            update({ page: 1, pageSize });
            props.onSectionPageChange?.(name, 1, pageSize);
          },
          onSortChange: (sort) => update({ page: 1, sort }),
          page: current.page ?? 1,
          pageSize: current.pageSize ?? 24,
          sort: current.sort ?? "score",
        };
      }
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
        (props.combineLocalSections ? ["local", "remote"] : ["unwatched", "watched", "remote"]).map((name) => {
          const entries = sections[name];
          if (!entries.length) return null;
          return renderRecommendationSection(
            name,
            entries,
            props.sectionTitles[name],
            controlsFor(name),
            componentsLoading ? null : PluginApi.components.SceneCard,
            {
              onLocalSelectionChanged,
              selectedLocalIds,
              selecting: selectedLocalIds.size > 0,
            },
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
      const [loadedOffset, setLoadedOffset] = React.useState(0);
      return React.createElement(RecommendationPanel, {
        append: loadedOffset > 0,
        dependency: `for-you:${loadedOffset}`,
        loader: function () {
          return fetchForYou(100, loadedOffset, runPluginOperation);
        },
        onSectionPageChange: function (section, page, pageSize) {
          if (section !== "local") {
            return;
          }
          setLoadedOffset((current) => Math.max(current, forYouOffsetForPage(page, pageSize)));
        },
        title: "For You",
        combineLocalSections: true,
        sectionTitles: {
          local: "For You",
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
    describeLocalScene,
    fetchForYou,
    fetchRelated,
    forYouOffsetForPage,
    formatRecommendationReasons,
    partitionRecommendations,
    recommendationBadgeCells,
    register,
    resolveLocalRecommendations,
    sceneCountRailEntries,
  };
});

(function (global) {
function cloneValue(value) {
  if (Array.isArray(value)) {
    return value.map((entry) => cloneValue(entry));
  }

  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, entry]) => [key, cloneValue(entry)])
    );
  }

  return value;
}

function createSearchPickerState({
  minQueryLength = 2,
  query = "",
  selected = null,
} = {}) {
  return {
    error: null,
    loading: false,
    minQueryLength,
    query: String(query),
    results: [],
    selected: cloneValue(selected),
  };
}

function setSearchPickerQuery(state, query) {
  return {
    ...state,
    error: null,
    query: String(query),
    results: [],
  };
}

function shouldShowSearchPickerResults(state) {
  return state.query.trim().length >= state.minQueryLength;
}

function setSearchPickerLoading(state) {
  return {
    ...state,
    error: null,
    loading: true,
  };
}

function resolveSearchPickerResults(state, results) {
  return {
    ...state,
    error: null,
    loading: false,
    results: Array.isArray(results) ? cloneValue(results) : [],
  };
}

function rejectSearchPickerResults(state, error) {
  return {
    ...state,
    error,
    loading: false,
  };
}

function selectPickerRecord(state, record) {
  return {
    ...state,
    error: null,
    results: [],
    selected: cloneValue(record),
  };
}

function clearPickerSelection(state) {
  return {
    ...state,
    error: null,
    selected: null,
  };
}


const DEFAULT_DISPLAY_MODES = Object.freeze(["grid", "list", "wall"]);

function getDefaultDisplayModes() {
  return [...DEFAULT_DISPLAY_MODES];
}

function createEntityPresentation(input = {}) {
  if (!input.adapter || typeof input.adapter !== "object") {
    throw new TypeError("createEntityPresentation requires an adapter");
  }

  const supportedModes = Array.isArray(input.supportedModes) && input.supportedModes.length
    ? [...input.supportedModes]
    : getDefaultDisplayModes();

  const renderers = {};
  for (const mode of supportedModes) {
    renderers[mode] = input.renderers?.[mode] ?? createDefaultModeRenderer(mode);
  }

  return {
    adapter: input.adapter,
    renderers,
    supportedModes,
  };
}

function resolveModeRenderer(presentation, displayMode = "grid") {
  if (!presentation || typeof presentation !== "object") {
    throw new TypeError("resolveModeRenderer requires a presentation");
  }

  if (presentation.renderers?.[displayMode]) {
    return presentation.renderers[displayMode];
  }

  const fallbackMode = presentation.supportedModes?.[0] ?? "grid";
  if (presentation.renderers?.[fallbackMode]) {
    return presentation.renderers[fallbackMode];
  }

  return createDefaultModeRenderer(fallbackMode);
}

function renderModeSurfaceItem(runtime, options = {}) {
  const renderer = resolveModeRenderer(options.presentation, options.displayMode);
  return renderer(runtime, options);
}

function createDefaultModeRenderer(mode) {
  return function renderDefaultMode(runtime, options = {}) {
    const { React } = runtime;
    const { adapter } = options.presentation;
    const context = options.context ?? {};
    const item = options.item;
    const image = adapter.imageOf?.(item, context) ?? null;
    const title = adapter.titleOf?.(item, context) ?? "";
    const meta = normalizeList(adapter.metaOf?.(item, context));
    const stats = normalizeList(adapter.statsOf?.(item, context));
    const badges = normalizeList(adapter.badgesOf?.(item, context));
    const key = adapter.keyOf?.(item, context) ?? context.index ?? title;

    return React.createElement(
      "article",
      {
        className: `stash-composables-mode-card stash-composables-mode-card--${mode}`,
        key,
      },
      image
        ? React.createElement("img", {
            alt: "",
            className: "stash-composables-mode-card__image",
            loading: "lazy",
            src: image,
          })
        : null,
      React.createElement(
        "div",
        { className: "stash-composables-mode-card__body" },
        React.createElement(
          "div",
          { className: "stash-composables-mode-card__title" },
          title
        ),
        meta.length
          ? React.createElement(
              "div",
              { className: "stash-composables-mode-card__meta" },
              ...meta.map((entry, index) =>
                React.createElement(
                  "div",
                  { key: `${key}-meta-${index}` },
                  stringifyMeta(entry)
                )
              )
            )
          : null,
        badges.length
          ? React.createElement(
              "div",
              { className: "stash-composables-mode-card__badges" },
              ...badges.map((badge, index) =>
                React.createElement(
                  "span",
                  {
                    className: "stash-composables-mode-card__badge",
                    key: `${key}-badge-${index}`,
                  },
                  stringifyBadge(badge)
                )
              )
            )
          : null,
        stats.length
          ? React.createElement(
              "div",
              { className: "stash-composables-mode-card__stats" },
              ...stats.map((stat, index) =>
                React.createElement(
                  "span",
                  {
                    className: "stash-composables-mode-card__stat",
                    key: `${key}-stat-${index}`,
                  },
                  stringifyStat(stat)
                )
              )
            )
          : null
      )
    );
  };
}

function normalizeList(value) {
  if (Array.isArray(value)) {
    return value.filter(Boolean);
  }

  if (value === null || value === undefined || value === "") {
    return [];
  }

  return [value];
}

function stringifyMeta(entry) {
  if (typeof entry === "string" || typeof entry === "number") {
    return String(entry);
  }

  if (entry && typeof entry === "object") {
    if (entry.label && entry.value !== undefined) {
      return `${entry.label}: ${entry.value}`;
    }

    if (entry.value !== undefined) {
      return String(entry.value);
    }
  }

  return "";
}

function stringifyBadge(entry) {
  if (typeof entry === "string" || typeof entry === "number") {
    return String(entry);
  }

  if (entry && typeof entry === "object") {
    return entry.label ?? "";
  }

  return "";
}

function stringifyStat(entry) {
  if (typeof entry === "string" || typeof entry === "number") {
    return String(entry);
  }

  if (entry && typeof entry === "object") {
    if (entry.label && entry.value !== undefined) {
      return `${entry.label}: ${entry.value}`;
    }

    if (entry.value !== undefined) {
      return String(entry.value);
    }
  }

  return "";
}



function renderCollectionSurface(runtime, props) {
  const { React, Spinner } = runtime;
  const items = Array.isArray(props.items) ? props.items : [];
  const displayMode = props.displayMode ?? "grid";
  const Heading = props.headingElement ?? "h1";

  return React.createElement(
    "div",
    { className: "stash-composables-collection" },
    React.createElement(
      "div",
      { className: "stash-composables-collection__header" },
      React.createElement(
        "div",
        { className: "stash-composables-collection__meta" },
        React.createElement(Heading, null, props.title),
        props.description ? React.createElement("div", null, props.description) : null
      ),
      props.renderActions ? props.renderActions() : null
    ),
    props.loading
      ? React.createElement(Spinner, { animation: "border" })
      : items.length
        ? React.createElement(
            "div",
            {
              className: [ "stash-composables-collection__items", props.itemsClassName ]
                .filter(Boolean)
                .join(" "),
            },
            ...items.map((item, index) => {
              if (typeof props.renderItem === "function") {
                return props.renderItem(item, index);
              }

              if (props.presentation) {
                return renderModeSurfaceItem(runtime, {
                  context: {
                    ...(props.renderItemContext ?? {}),
                    index,
                  },
                  displayMode,
                  item,
                  presentation: props.presentation,
                });
              }

              return item;
            })
          )
        : React.createElement(
            "div",
            { className: "stash-composables-collection__empty" },
            props.emptyMessage ?? "No items yet."
          )
  );
}


function resolveRankedItems(ranked, items, keyOf) {
  if (typeof keyOf !== "function") {
    throw new TypeError("resolveRankedItems requires keyOf");
  }

  const itemByKey = new Map(
    (Array.isArray(items) ? items : []).map((item) => [String(keyOf(item)), item])
  );

  return (Array.isArray(ranked) ? ranked : [])
    .map((entry, index) => ({
      entry,
      index,
      item: itemByKey.get(String(entry?.key ?? "")),
      score: Number(entry?.score ?? 0),
    }))
    .filter(({ item }) => item !== undefined)
    .sort((left, right) => right.score - left.score || left.index - right.index)
    .map(({ entry, item, score }) => ({ entry, item, score }));
}

function filterRankedItems(items, predicate) {
  if (typeof predicate !== "function") {
    return [...(Array.isArray(items) ? items : [])];
  }

  return (Array.isArray(items) ? items : []).filter(predicate);
}

function sortRankedItems(items, comparator) {
  const source = Array.isArray(items) ? items : [];
  if (typeof comparator !== "function") {
    return [...source];
  }

  return source
    .map((item, index) => ({ item, index }))
    .sort((left, right) => Number(comparator(left.item, right.item)) || left.index - right.index)
    .map(({ item }) => item);
}

function paginateRankedItems(items, options = {}) {
  const source = Array.isArray(items) ? items : [];
  const pageSize = Math.max(1, Number(options.pageSize ?? source.length) || 1);
  const pageCount = Math.max(1, Math.ceil(source.length / pageSize));
  const page = Math.min(pageCount, Math.max(1, Number(options.page ?? 1) || 1));
  const start = (page - 1) * pageSize;
  const itemsOnPage = source.slice(start, start + pageSize);

  return {
    end: source.length ? start + itemsOnPage.length : 0,
    items: itemsOnPage,
    page,
    pageCount,
    pageSize,
    start: source.length ? start + 1 : 0,
    total: source.length,
  };
}



function renderRankedControls(React, sort, pagination, page) {
  const sortOptions = Array.isArray(sort?.options) ? sort.options : [];
  const pageSizeOptions = Array.isArray(pagination?.pageSizeOptions)
    ? pagination.pageSizeOptions
    : [];
  if (!sortOptions.length && !pagination) {
    return null;
  }

  return React.createElement(
    "div",
    { className: "stash-composables-ranked-controls" },
    sortOptions.length
      ? React.createElement(
          "select",
          {
            "aria-label": "Sort recommendations",
            onChange: (event) => sort.onChange?.(event.target.value),
            value: sort.value,
          },
          ...sortOptions.map((option) => React.createElement(
            "option",
            { key: option.value, value: option.value },
            option.label
          ))
        )
      : null,
    pagination
      ? React.createElement(
          "div",
          { className: "stash-composables-ranked-controls__paging" },
          `${page.start}-${page.end} of ${page.total}`,
          React.createElement(
            "button",
            {
              "aria-label": "Previous page",
              disabled: page.page <= 1,
              onClick: () => pagination.onPageChange?.(page.page - 1),
              type: "button",
            },
            "Previous"
          ),
          React.createElement(
            "button",
            {
              "aria-label": "Next page",
              disabled: page.page >= page.pageCount,
              onClick: () => pagination.onPageChange?.(page.page + 1),
              type: "button",
            },
            "Next"
          )
        )
      : null,
    pagination && pageSizeOptions.length
      ? React.createElement(
          "select",
          {
            "aria-label": "Items per page",
            onChange: (event) => pagination.onPageSizeChange?.(Number(event.target.value)),
            value: String(page.pageSize),
          },
          ...pageSizeOptions.map((pageSize) => React.createElement(
            "option",
            { key: pageSize, value: String(pageSize) },
            `${pageSize} per page`
          ))
        )
      : null
  );
}

function renderRankedCollectionSurface(runtime, props = {}) {
  const resolved = resolveRankedItems(props.ranked, props.items, props.keyOf);
  const filtered = filterRankedItems(resolved, props.filterRecord);
  const sorted = sortRankedItems(filtered, props.sort?.compare);
  const page = paginateRankedItems(sorted, props.pagination);
  const controls = renderRankedControls(runtime.React, props.sort, props.pagination, page);

  return renderCollectionSurface(runtime, {
    ...props,
    items: page.items,
    renderActions: controls || props.renderActions
      ? () => runtime.React.createElement(
          "div",
          { className: "stash-composables-ranked-actions" },
          controls,
          props.renderActions ? props.renderActions() : null
        )
      : undefined,
    renderItemContext: {
      ...(props.renderItemContext ?? {}),
      totalRankedItems: page.total,
    },
  });
}


function cloneValue(value) {
  if (Array.isArray(value)) {
    return value.map((entry) => cloneValue(entry));
  }

  if (value && typeof value === "object") {
    return Object.fromEntries(
      Object.entries(value).map(([key, entry]) => [key, cloneValue(entry)])
    );
  }

  return value;
}

function createSceneTabState(value) {
  const nextValue = cloneValue(value ?? null);
  return {
    mode: "view",
    saving: false,
    error: null,
    value: nextValue,
    draft: cloneValue(nextValue),
  };
}

function getSceneTabActions(state) {
  if (state?.mode === "edit") {
    return [
      {
        id: "save",
        label: "Save",
        disabled: Boolean(state?.saving),
      },
      {
        id: "cancel",
        label: "Cancel",
        disabled: Boolean(state?.saving),
      },
    ];
  }

  return [{ id: "edit", label: "Edit", disabled: false }];
}

function createSceneTabController({ initialValue = null, onSave } = {}) {
  if (typeof onSave !== "function") {
    throw new TypeError("createSceneTabController requires an onSave function");
  }

  let state = createSceneTabState(initialValue);

  function setState(nextState) {
    state = nextState;
    return state;
  }

  return {
    cancel() {
      return setState(createSceneTabState(state.value));
    },
    edit() {
      return setState({
        ...state,
        mode: "edit",
        saving: false,
        error: null,
        draft: cloneValue(state.value),
      });
    },
    getState() {
      return state;
    },
    async save() {
      if (state.mode !== "edit" || state.saving) {
        return state.value;
      }

      const draft = cloneValue(state.draft);
      setState({
        ...state,
        saving: true,
        error: null,
      });

      try {
        const value = await onSave(draft);
        return setState(createSceneTabState(value));
      } catch (error) {
        setState({
          ...state,
          mode: "edit",
          saving: false,
          error,
          draft,
        });
        throw error;
      }
    },
    updateDraft(nextDraft) {
      return setState({
        ...state,
        mode: "edit",
        error: null,
        draft: cloneValue(nextDraft),
      });
    },
  };
}

function createSceneTabRegistration({ key, title, createController } = {}) {
  if (!key) {
    throw new TypeError("createSceneTabRegistration requires a key");
  }

  if (!title) {
    throw new TypeError("createSceneTabRegistration requires a title");
  }

  if (typeof createController !== "function") {
    throw new TypeError("createSceneTabRegistration requires a createController function");
  }

  return {
    key: String(key),
    eventKey: String(key),
    title: String(title),
    createController,
  };
}


function listItemIds(items) {
  return (items ?? []).map((item) => item.id);
}

function clampSelectedIds(selectedIds, items) {
  const available = new Set(listItemIds(items));
  return new Set(Array.from(selectedIds ?? []).filter((id) => available.has(id)));
}

function toggleSelectedId(selectedIds, id, selected) {
  const next = new Set(selectedIds ?? []);
  if (selected) {
    next.add(id);
  } else {
    next.delete(id);
  }
  return next;
}

function selectAllIds(items) {
  return new Set(listItemIds(items));
}

function clearSelectedIds() {
  return new Set();
}

function getDisplayMode(value, fallback = "grid") {
  return value === "list" ? "list" : fallback;
}

function removeSelectedItems(items, selectedIds) {
  const excluded = new Set(selectedIds ?? []);
  return (items ?? []).filter((item) => !excluded.has(item.id));
}

function moveSelectedItems(items, selectedIds, direction) {
  const selected = new Set(selectedIds ?? []);
  if (!selected.size) {
    return items ?? [];
  }

  const next = [...(items ?? [])];

  if (direction < 0) {
    for (let index = 1; index < next.length; index += 1) {
      if (selected.has(next[index].id) && !selected.has(next[index - 1].id)) {
        const swap = next[index - 1];
        next[index - 1] = next[index];
        next[index] = swap;
      }
    }
    return next;
  }

  for (let index = next.length - 2; index >= 0; index -= 1) {
    if (selected.has(next[index].id) && !selected.has(next[index + 1].id)) {
      const swap = next[index + 1];
      next[index + 1] = next[index];
      next[index] = swap;
    }
  }

  return next;
}

function getSelectionSummary(selectedIds, items) {
  const selected = clampSelectedIds(selectedIds, items);
  return {
    selectedCount: selected.size,
    totalCount: (items ?? []).length,
    hasSelection: selected.size > 0,
  };
}


function getRuntime(runtime) {
  if (!runtime?.React) {
    throw new Error("Related entity list UI requires a React runtime");
  }

  return runtime;
}

function renderRelatedEntityList(runtime, props) {
  const { React, Button } = getRuntime(runtime);
  const items = Array.isArray(props.items) ? props.items : [];

  if (!items.length) {
    return React.createElement(
      "div",
      { className: "stash-composables-related-entity-list__empty" },
      props.emptyMessage ?? "No related items."
    );
  }

  return React.createElement(
    "div",
    { className: "stash-composables-related-entity-list" },
    ...items.map((item, index) => {
      const key = props.getItemKey ? props.getItemKey(item) : item.id ?? index;
      const title = props.renderTitle(item);
      const meta = props.renderMeta ? props.renderMeta(item) : null;
      const leading = props.renderLeading ? props.renderLeading(item) : null;
      const actions = props.renderActions ? props.renderActions(item) : null;
      const removeButton =
        Button && props.onRemove
          ? React.createElement(
              Button,
              {
                onClick: () => props.onRemove(item),
                variant: "secondary",
              },
              props.removeLabel ?? "Remove"
            )
          : null;

      return React.createElement(
        "div",
        {
          className: "stash-composables-related-entity-list__row",
          key,
        },
        leading,
        React.createElement(
          "div",
          { className: "stash-composables-related-entity-list__body" },
          React.createElement(
            "div",
            { className: "stash-composables-related-entity-list__title" },
            title
          ),
          meta
            ? React.createElement(
                "div",
                { className: "stash-composables-related-entity-list__meta" },
                meta
              )
            : null
        ),
        removeButton || actions
          ? React.createElement(
              "div",
              { className: "stash-composables-related-entity-list__actions" },
              removeButton,
              actions
            )
          : null
      );
    })
  );
}



function renderSearchPickerSurface(runtime, props) {
  const { React, Form, Button, Icon } = runtime;
  const selectedItems = Array.isArray(props.selectedItems) ? props.selectedItems : [];
  const results = Array.isArray(props.results) ? props.results : [];
  const renderThumb = (className, value) =>
    value
      ? React.createElement("img", {
          alt: "",
          className,
          loading: "lazy",
          src: value,
        })
      : null;

  return React.createElement(
    "div",
    { className: "stash-composables-search-picker" },
    selectedItems.length
      ? renderRelatedEntityList(
          { React, Button },
          {
            getItemKey: props.getSelectedItemKey ?? props.getResultKey ?? ((item) => item.id),
            items: selectedItems,
            onRemove: props.onClearSelection ?? null,
            removeLabel: props.selectedRemoveLabel ?? "Remove",
            renderActions: props.selectedActionLabel && props.onClearSelection
              ? () =>
                  React.createElement(
                    Button,
                    {
                      onClick: props.onClearSelection,
                      variant: "secondary",
                    },
                    props.selectedActionLabel
                  )
              : null,
            renderLeading: props.renderSelectedThumb
              ? (item) =>
                  renderThumb(
                    "stash-composables-search-picker__thumb",
                    props.renderSelectedThumb(item)
                  )
              : null,
            renderMeta: props.renderSelectedMeta ?? (() => null),
            renderTitle: props.renderSelectedTitle ?? ((item) => item.title ?? item.name ?? item.id),
          }
        )
      : null,
    React.createElement(
      "div",
      { className: "stash-composables-search-picker__input" },
      React.createElement(Form.Control, {
        onChange: (event) => props.onQueryChange(event.currentTarget.value),
        placeholder: props.inputPlaceholder ?? "Search",
        value: props.query ?? "",
      }),
      Icon ? React.createElement(Icon, { icon: props.icon }) : null,
      props.action
        ? React.createElement(
            Button,
            {
              disabled: Boolean(props.action.disabled),
              onClick: props.action.onClick,
              variant: props.action.variant ?? "secondary",
            },
            props.action.label
          )
        : null
    ),
    props.loading
      ? React.createElement(
          "div",
          { className: "stash-composables-search-picker__empty" },
          props.loadingMessage ?? "Loading..."
        )
      : props.showResults
        ? results.length
          ? React.createElement(
              "div",
              { className: "stash-composables-search-picker__results" },
              results.map((item) =>
                React.createElement(
                  "button",
                  {
                    className: "stash-composables-search-picker__result",
                    key: String((props.getResultKey ?? ((entry) => entry.id))(item)),
                    onClick: () => props.onSelectResult(item),
                    type: "button",
                  },
                  props.renderResultThumb
                    ? renderThumb(
                        "stash-composables-search-picker__thumb",
                        props.renderResultThumb(item)
                      )
                    : null,
                  React.createElement(
                    "div",
                    { className: "stash-composables-search-picker__result-main" },
                    React.createElement(
                      "strong",
                      null,
                      (props.renderResultTitle ??
                        ((entry) => entry.title ?? entry.name ?? entry.id))(item)
                    ),
                    props.renderResultMeta
                      ? React.createElement(
                          "div",
                          { className: "stash-composables-search-picker__meta" },
                          props.renderResultMeta(item)
                        )
                      : null
                  )
                )
              )
            )
          : React.createElement(
              "div",
              { className: "stash-composables-search-picker__empty" },
              props.emptyResultsMessage ?? "No results found."
            )
        : props.idleMessage
          ? React.createElement(
              "div",
              { className: "stash-composables-search-picker__empty" },
              props.idleMessage
            )
          : null
  );
}


function getRuntime(runtime) {
  if (!runtime?.React) {
    throw new Error("Selection surface UI requires a React runtime");
  }

  return runtime;
}

function getLabels(labels = {}) {
  return {
    grid: labels.grid ?? "Grid",
    list: labels.list ?? "List",
    moveDown: labels.moveDown ?? "Move down",
    moveUp: labels.moveUp ?? "Move up",
    removeSelected: labels.removeSelected ?? "Remove selected",
    selectAll: labels.selectAll ?? "Select all",
    selectNone: labels.selectNone ?? "Select none",
  };
}

function renderSelectionToolbar(runtime, props) {
  const { React, Button, ButtonGroup } = getRuntime(runtime);
  const labels = getLabels(props.labels);
  const hasSelection = Boolean(props.hasSelection);

  return React.createElement(
    "div",
    { className: "stash-composables-selection-toolbar" },
    React.createElement(
      ButtonGroup,
      null,
      React.createElement(
        Button,
        {
          onClick: props.onSelectAll,
          variant: "secondary",
        },
        labels.selectAll
      ),
      React.createElement(
        Button,
        {
          onClick: props.onSelectNone,
          variant: "secondary",
        },
        labels.selectNone
      )
    ),
    React.createElement(
      ButtonGroup,
      null,
      React.createElement(
        Button,
        {
          disabled: !hasSelection,
          onClick: props.onMoveUp,
          variant: "secondary",
        },
        labels.moveUp
      ),
      React.createElement(
        Button,
        {
          disabled: !hasSelection,
          onClick: props.onMoveDown,
          variant: "secondary",
        },
        labels.moveDown
      ),
      React.createElement(
        Button,
        {
          disabled: !hasSelection,
          onClick: props.onRemoveSelected,
          variant: "danger",
        },
        labels.removeSelected
      )
    ),
    React.createElement(
      ButtonGroup,
      null,
      React.createElement(
        Button,
        {
          onClick: () => props.onSetDisplayMode("grid"),
          variant: props.displayMode === "grid" ? "primary" : "secondary",
        },
        labels.grid
      ),
      React.createElement(
        Button,
        {
          onClick: () => props.onSetDisplayMode("list"),
          variant: props.displayMode === "list" ? "primary" : "secondary",
        },
        labels.list
      )
    )
  );
}

function renderSelectableGrid(runtime, props) {
  const { React } = getRuntime(runtime);

  if (!props.items?.length) {
    return React.createElement(
      "div",
      { className: "stash-composables-selection-empty" },
      props.emptyMessage ?? "No items."
    );
  }

  return React.createElement(
    "div",
    { className: "stash-composables-selection-grid" },
    props.items.map((item) => {
      const id = props.getItemId(item);
      const selected = props.selectedIds?.has(id) ?? false;

      return React.createElement(
        "div",
        {
          className: `stash-composables-selection-card${selected ? " is-selected" : ""}`,
          key: id,
        },
        React.createElement(
          "label",
          { className: "stash-composables-selection-card__select" },
          React.createElement("input", {
            checked: selected,
            onChange: (event) =>
              props.onSelectChange(id, event.currentTarget.checked, false),
            type: "checkbox",
          })
        ),
        React.createElement(
          "div",
          { className: "stash-composables-selection-card__body" },
          props.renderItemBody(item)
        )
      );
    })
  );
}

function renderSelectableTable(runtime, props) {
  const { React } = getRuntime(runtime);

  if (!props.items?.length) {
    return React.createElement(
      "div",
      { className: "stash-composables-selection-empty" },
      props.emptyMessage ?? "No items."
    );
  }

  return React.createElement(
    "table",
    { className: "table stash-composables-selection-table" },
    React.createElement(
      "thead",
      null,
      React.createElement(
        "tr",
        null,
        React.createElement("th", null, ""),
        props.columns.map((column) =>
          React.createElement("th", { key: column.key }, column.header)
        )
      )
    ),
    React.createElement(
      "tbody",
      null,
      props.items.map((item) => {
        const id = props.getItemId(item);
        const selected = props.selectedIds?.has(id) ?? false;

        return React.createElement(
          "tr",
          {
            className: selected ? "is-selected" : "",
            key: id,
          },
          React.createElement(
            "td",
            null,
            React.createElement("input", {
              checked: selected,
              onChange: (event) =>
                props.onSelectChange(id, event.currentTarget.checked, false),
              type: "checkbox",
            })
          ),
          props.columns.map((column) =>
            React.createElement("td", { key: column.key }, column.renderCell(item))
          )
        );
      })
    )
  );
}


  global.StashPluginComponents = {
    clearPickerSelection,
    createEntityPresentation,
    createSceneTabController,
    createSceneTabRegistration,
    createSearchPickerState,
    getDefaultDisplayModes,
    renderCollectionSurface,
    filterRankedItems,
    paginateRankedItems,
    renderRankedCollectionSurface,
    clampSelectedIds,
    clearSelectedIds,
    getDisplayMode,
    getSceneTabActions,
    getSelectionSummary,
    listItemIds,
    moveSelectedItems,
    rejectSearchPickerResults,
    removeSelectedItems,
    renderRelatedEntityList,
    renderModeSurfaceItem,
    renderSearchPickerSurface,
    renderSelectableGrid,
    renderSelectableTable,
    renderSelectionToolbar,
    resolveModeRenderer,
    resolveRankedItems,
    resolveSearchPickerResults,
    selectAllIds,
    selectPickerRecord,
    setSearchPickerLoading,
    setSearchPickerQuery,
    shouldShowSearchPickerResults,
    toggleSelectedId
  };
})(typeof window !== "undefined" ? window : globalThis);

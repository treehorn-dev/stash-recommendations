# Recommendation Card Poster Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task.

**Goal:** Make local recommendation cards match native scene-card media behavior: a poster that links to the scene and previews on hover, a heatmap rail below the poster, contextual poster overlays, and semantic rating badge colors.

**Architecture:** Extend `stash-plugin-components` with presentation-only media slots: caller-provided poster source, preview source, link target, overlay content, and a below-poster rail. The recommendation plugin queries and maps Stash-specific fields into those slots, while the shared component owns media structure, hover preview behavior, and accessibility. Rating badges carry a semantic source and normalized score; CSS determines fill versus outline and its color scale.

**Tech Stack:** JavaScript, React runtime supplied by Stash, GraphQL, CSS, Node test runner.

### Task 1: Add generic linked preview media to entity cards

**Files:**
- Modify: `/Users/allenday/src/treehorn-dev/stash-plugin-components/src/entity-card-ui.js`
- Modify: `/Users/allenday/src/treehorn-dev/stash-plugin-components/src/stash-surface.css`
- Modify: `/Users/allenday/src/treehorn-dev/stash-plugin-components/test/entity-card-ui.test.js`

1. Write a failing test that passes `thumbnail.href`, `thumbnail.previewSrc`, `thumbnail.overlay`, and `mediaRail`, then asserts the poster is an anchor, the preview video is present, and the rail renders after media but before body.
2. Run `npm test -- entity-card-ui.test.js`; confirm the test fails because those media slots are not rendered.
3. Render the static poster and muted looping preview video in a generic media wrapper. Use CSS to swap their visibility on hover and focus-within, wrap media in an anchor only when `href` is supplied, and render caller-owned overlay and `mediaRail` nodes without Stash-specific assumptions.
4. Add CSS for fixed poster positioning, hidden-until-hover preview video, focus-visible link state, and a compact below-poster rail.
5. Run `npm test` and `npm run build`.
6. Commit the reusable component and rebuilt `dist/` files.

### Task 2: Query and map Stash scene-card media data

**Files:**
- Modify: `plugin/stashRecommendations/ui/recommendations.js`
- Modify: `tests/ui/recommendations.test.js`

1. Write failing tests for `describeLocalScene` that expect preview path, a formatted primary-file duration, studio name/link data, and interactive speed; write a FYP-card test that expects poster `href`, `previewSrc`, overlays, and a heatmap `mediaRail` instead of a thumbnail overlay.
2. Run `node --test tests/ui/recommendations.test.js`; confirm the expectations fail.
3. Extend `FIND_LOCAL_SCENES` with `paths.preview`, `interactive_speed`, `studio { id name }`, and `files { duration }`.
4. Add pure helpers to select the primary file duration and format it as `H:MM:SS` or `M:SS`; preserve absent values as no overlay.
5. Map studio, duration, and interactive speed to caller-owned poster overlay elements. Link studio to its local page, link poster/preview media to `/scenes/{id}`, and pass heatmap as `mediaRail`.
6. Run the focused test and `npm run test:ui`.

### Task 3: Add score badge semantics and native-inspired color scale

**Files:**
- Modify: `plugin/stashRecommendations/ui/recommendations.js`
- Modify: `plugin/stashRecommendations/ui/recommendations.css`
- Modify: `tests/ui/recommendations.test.js`
- Modify: `tests/ui/smoke.test.js`

1. Write a failing unit test that expects a local rating badge with `source: "local"`, normalized five-point score, and color-step class; add cases for future predicted/public score sources that use the same color step with outline semantics.
2. Run `node --test tests/ui/recommendations.test.js`; confirm the current fixed rating badge fails the expected shape.
3. Keep local scene ratings as the current source, add the source/score/class mapping, and retain the predicted/public branches as data-driven fallbacks without introducing server prediction work.
4. Add FYP-scoped CSS color steps from beige through yellow and orange to red. Local ratings are filled; predicted and public ratings are transparent with a colored border and text.
5. Add a CSS smoke test that asserts media rail positioning, hover preview selectors, poster overlay classes, and fill/outline score classes.
6. Run `npm run test:ui`, the Python plugin suite, and `git diff --check`.

### Task 4: Vendor, visual QA, and release

**Files:**
- Modify: `plugin/stashRecommendations/ui/stash-plugin-components.js`
- Modify: `plugin/stashRecommendations/ui/stash-plugin-components.css` if the generated CSS changes

1. Copy the rebuilt generic browser bundle and CSS from `stash-plugin-components/dist/` into the plugin assets.
2. Run all plugin verification commands again after vendoring.
3. Load the authenticated FYP and native `/scenes?sortby=o_counter&sortdir=desc` reference. Verify poster click navigation, hover/focus preview, below-poster heatmap, studio/duration/speed overlays, and color semantics at narrow and normal card widths.
4. Commit and open separate component and recommendations PRs. Do not release until both PRs are merged and browser QA is accepted.

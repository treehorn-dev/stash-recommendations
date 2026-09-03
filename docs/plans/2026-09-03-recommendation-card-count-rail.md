# Recommendation Card Count Rail Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use `superpowers:executing-plans` to implement this plan task-by-task.

**Goal:** Replace verbose scene metadata text on For You cards with compact native-style icon/count controls.

**Architecture:** Add a generic structured count-rail slot to `stash-plugin-components`. Each entry supplies a caller-owned icon and popover content; the reusable component owns the count trigger and hover/focus presentation, while the plugin owns Stash icons, labels, links, and data mapping.

**Tech Stack:** JavaScript, React runtime supplied by Stash, CSS, Node test runner.

### Task 1: Add the generic component count-rail contract

**Files:**
- Modify: `/Users/allenday/src/treehorn-dev/stash-plugin-components/src/entity-card-ui.js`
- Modify: `/Users/allenday/src/treehorn-dev/stash-plugin-components/src/stash-surface.css`
- Modify: `/Users/allenday/src/treehorn-dev/stash-plugin-components/test/entity-card-ui.test.js`

1. Write a failing test proving structured count-rail entries render after the body and before the existing footer, with an icon/count trigger and a caller-provided popover body.
2. Run `npm test -- entity-card-ui.test.js` and confirm the expected failure.
3. Add the minimal `countRail` slot and generic structural classes without Stash-specific icons or meanings.
4. Add compact, wrapping hover/focus styling so every count trigger reveals its caller-provided content and remains keyboard reachable.
5. Run `npm test` and `npm run build`.
6. Commit the component contract and rebuilt `dist/` files.

### Task 2: Map recommendation scenes to count entries

**Files:**
- Modify: `plugin/stashRecommendations/ui/recommendations.js`
- Modify: `tests/ui/recommendations.test.js`

1. Write a failing test for a local scene that expects performer, tag, group, play, O, and media-type count entries.
2. Run `node --test tests/ui/recommendations.test.js` and confirm the expected failure.
3. Implement the adapter mapping with caller-owned icon nodes, accessible labels, and popover content: linked performer/tag/group names and descriptive engagement/media details.
4. Remove the equivalent verbose text attributes from recommendation cards.
5. Run the focused test and `npm run test:ui`.

### Task 3: Style and verify the For You integration

**Files:**
- Modify: `plugin/stashRecommendations/ui/recommendations.css`
- Modify: `tests/ui/smoke.test.js`

1. Write a failing CSS smoke assertion for the compact recommendation count rail.
2. Run `node --test tests/ui/smoke.test.js` and confirm the expected failure.
3. Add only FYP-scoped spacing and visual styling needed to match the native card rail.
4. Run `npm run test:ui`, the Python plugin suite, and `git diff --check`.
5. Reload the authenticated FYP browser page and verify icon/count rails fit within the native-sized cards.
6. Commit and open a recommendation-plugin PR.

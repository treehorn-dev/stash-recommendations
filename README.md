# Stash Recommendations v0

`stash-recommendations` is a Stash plugin plus a Go/PostgreSQL service. The
plugin captures normalized ratings plus explicit play and `o` history, proxies
authenticated Stash-box metadata through the local Stash client, and surfaces
related plus For You recommendations inside Stash. The server stores hashed API
keys, immutable interaction events, source snapshots, and versioned
recommendation projections.

## Prerequisites

- Go 1.22+
- Python with `pytest`
- Node 20+
- Docker with Compose
- A Stash instance with this plugin installed

## Start PostgreSQL

Bring up the local database:

```bash
docker compose up -d postgres
```

Use this DSN for local work:

```bash
export DATABASE_URL='postgres://stash_recommendations:stash_recommendations@127.0.0.1:5432/stash_recommendations?sslmode=disable'
export POSTGRES_TEST_DSN="$DATABASE_URL"
```

## Provision an API Key

v0 does not yet ship a dedicated admin CLI. The operator bootstrap is a
one-off Go helper placed under `server/` so it can import the internal store
package.

```bash
helper_dir=$(mktemp -d server/bootstrap-key.XXXXXX)
cat >"$helper_dir/main.go" <<'EOF'
package main

import (
  "context"
  "encoding/json"
  "log"
  "os"

  "github.com/treehorn/stash-recommendations/server/internal/store"
)

func main() {
  ctx := context.Background()
  repository, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
  if err != nil {
    log.Fatal(err)
  }
  defer repository.Close(ctx)
  if err := repository.Migrate(ctx); err != nil {
    log.Fatal(err)
  }
  account, err := repository.CreateAccount(ctx)
  if err != nil {
    log.Fatal(err)
  }
  if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
    "account_id": account.ID,
    "api_key": account.PlaintextKey,
  }); err != nil {
    log.Fatal(err)
  }
}
EOF
DATABASE_URL="$DATABASE_URL" go run "./${helper_dir#./}"
rm -rf "$helper_dir"
```

Record the emitted `api_key`. The service stores only the Argon2id hash.

## Run the Service

Start the HTTP API:

```bash
HTTP_ADDR=127.0.0.1:8080 DATABASE_URL="$DATABASE_URL" go run ./server/cmd/recommendations
```

Build a fresh recommendation model with the same one-off helper pattern. This
keeps the operator path explicit in v0:

```bash
helper_dir=$(mktemp -d server/build-model.XXXXXX)
cat >"$helper_dir/main.go" <<'EOF'
package main

import (
  "context"
  "encoding/json"
  "log"
  "os"

  "github.com/treehorn/stash-recommendations/server/internal/model"
  "github.com/treehorn/stash-recommendations/server/internal/store"
)

func main() {
  ctx := context.Background()
  repository, err := store.Open(ctx, os.Getenv("DATABASE_URL"))
  if err != nil {
    log.Fatal(err)
  }
  defer repository.Close(ctx)
  if err := repository.Migrate(ctx); err != nil {
    log.Fatal(err)
  }
  version, err := model.NewBuilder(model.NewRepository(repository.Pool()), model.DefaultOWeight).BuildAndActivate(ctx)
  if err != nil {
    log.Fatal(err)
  }
  if err := json.NewEncoder(os.Stdout).Encode(map[string]string{
    "model_version": version,
  }); err != nil {
    log.Fatal(err)
  }
}
EOF
DATABASE_URL="$DATABASE_URL" go run "./${helper_dir#./}"
rm -rf "$helper_dir"
```

For development, the server also supports `BUILD_MODEL_ON_START=true` on boot.
`MODEL_O_WEIGHT` defaults to `1.5` and may be overridden with a positive
number.

## Configure the Plugin

In Stash plugin settings for `stashRecommendations`, configure:

- `service_url`: public HTTPS base URL for the service
- `api_key`: the issued recommendation API key
- `show_remote_results`: optional remote-only recommendation links

The raw plugin exposes these tasks:

- `sync-ratings`: preview and enqueue a full explicit rating sync
- `sync-engagement`: preview and enqueue recorded `play_history` and `o_history`
- `sync-metadata`: preview and enqueue Stash-box scene and performer snapshots
- `deliver-outbox`: send pending events and snapshots to the service
- `status`: show configuration and outbox state

The rating hook is `capture-rating` on `Scene.Update.Post`.

## Explicit Sync and Status

`sync-ratings`, `sync-engagement`, and `sync-metadata` all require confirmation
before enqueueing work. `deliver-outbox` is independent: hooks and sync steps
write SQLite only, and delivery happens later.

`status` reports:

- `configured`
- `settings.service_url`
- `settings.api_key_configured`
- `settings.show_remote_results`
- `outbox.pending`, `outbox.delivered`, `outbox.quarantined`
- `outbox.last_error`
- `outbox.paused.active` and `outbox.paused.reason`

## Privacy Boundary

The plugin uploads only:

- normalized external content keys `{ endpoint, stash_id }`
- normalized explicit ratings in `[0,1]`
- event kind plus UTC timestamp for play and `o` history
- Stash-box-schema scene, performer, tag, studio, group, and remote URL/image data

The plugin does not upload:

- local scene ids, titles, tags, or custom fields
- file paths, files, hashes, organization, or player settings
- local playback positions or local-only metadata
- Stash-box credentials

Source API keys remain client-side and are used only for plugin-to-source
GraphQL requests.

## Manual Smoke

Use this flow against a local Stash only when one is available:

1. Configure the plugin with a valid HTTPS `service_url`, issued `api_key`, and one authenticated Stash-box endpoint in Stash configuration.
2. Rate a scene that exposes two `stash_ids`; run `deliver-outbox`; confirm the plugin reports two delivered rating items after the queue drains.
3. Clear that rating; run `deliver-outbox`; confirm one `scene.rating.remove` is delivered and the queue drains again.
4. Run `sync-ratings` and confirm the preview count matches the currently rated local scenes before re-running with confirmation.
5. Run `sync-engagement` and confirm the preview count matches imported `play_history` plus `o_history` timestamps before re-running with confirmation.
6. Run `sync-metadata` with a configured Stash-box source; confirm snapshots are delivered and a source API key does not appear in `source_snapshots.raw_json`.
7. Run the one-off model build helper; open a related scene page and confirm related content resolves locally by default.
8. Enable `show_remote_results`; confirm remote-only items appear as canonical source links while local items still resolve through Stash.
9. Stop the service, run `deliver-outbox`, and confirm `status` shows pending items plus a retryable `last_error`; restart the service and re-run `deliver-outbox` to clear the queue.
10. Replace the configured API key with a non-issued key, run `deliver-outbox`, and confirm `status` pauses delivery with `service authentication failed`.

## Verification

Run the automated checks:

```bash
make test
make test-contract
make test-e2e
go vet ./server/...
git diff --check
```

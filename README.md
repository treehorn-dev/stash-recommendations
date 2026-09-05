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

The compose service uses `pgvector/pgvector:pg16`. Existing deployments on a
plain PostgreSQL image must be moved to that image before applying migration
`009_pgvector_recommendations`; take a database backup before replacing the
container.

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
For an explicit production rebuild without starting another HTTP listener, run
the service binary with `REBUILD_MODEL_ONCE=true`; it migrates, activates one
new model version, logs the version, and exits.
`MODEL_O_WEIGHT` defaults to `1.5` and may be overridden with a positive
number. A build stores one 256-dimensional hashed vector per source scene and
uses bounded pgvector cosine retrieval for related scenes and account profiles;
it does not materialize pairwise scene recommendations.

## Configure the Plugin

Install through Stash's **Settings > Plugins > Available Packages** using this
remote package-list URL:

`https://github.com/treehorn-dev/stash-recommendations/releases/latest/download/index.yml`

In Stash plugin settings for `stashRecommendations`, configure:

- `service_url`: public HTTPS base URL, or an opted-in private/Tailnet HTTP URL
- `api_key`: the issued recommendation API key
- `show_remote_results`: optional remote-only recommendation links
- `allow_private_http`: allow HTTP only for loopback, private, or Tailnet IP addresses

The raw plugin exposes these tasks:

- `sync-ratings`, `sync-engagement`, `sync-metadata`: preview the respective sync
- `sync-ratings-confirmed`, `sync-engagement-confirmed`, `sync-metadata-confirmed`: queue the respective sync
- `deliver-outbox`: send pending events and snapshots to the service
- `status`: show configuration and outbox state

The rating hook is `capture-rating` on `Scene.Update.Post`.

## Better Scene Card Integration

When [Better Scene Card](https://github.com/treehorn-dev/stash-better-scene-card)
is installed, Stash Recommendations publishes a page-local cached value
provider named `stash-recommendations.predicted-rating`. It supplies a
personal predicted rating on the same `0..5` scale as Stash only for locally
resolved recommendation scenes. It does not issue network requests from card
rendering and is absent for cold-start accounts or scenes without a prediction.

Paste this into **Better Scene Card > Chip Slots** to show local rating first,
then the recommendation prediction when no local rating exists, plus an O/play
score normalized to `0..99`:

```json
[
  {
    "label": { "type": "icon", "name": "star" },
    "value": {
      "type": "function",
      "body": "const local = Number(scene.rating100); return local > 0 ? local / 20 : helpers.value('stash-recommendations.predicted-rating', scene);"
    },
    "mode": {
      "type": "function",
      "body": "return Number(scene.rating100) > 0 ? 'filled' : 'border';"
    },
    "fill": { "color": "#000000", "alpha": 0.65 },
    "color": {
      "type": "scale",
      "min": { "value": 0, "color": "#b59a68" },
      "mid": { "value": 2.5, "color": "#ffcc00" },
      "max": { "value": 5, "color": "#ff0000" }
    }
  },
  {
    "label": { "type": "text", "value": "O/P" },
    "value": {
      "type": "function",
      "body": "const plays = Number(scene.play_count) || 0; const oCount = Number(scene.o_counter) || 0; return plays > 0 ? Math.min(99, Math.max(0, Math.round((oCount / plays) * 99))) : 0;"
    },
    "mode": "filled",
    "color": {
      "type": "scale",
      "min": { "value": 0, "color": "#000000" },
      "mid": { "value": 50, "color": "#800000" },
      "max": { "value": 99, "color": "#ff0000" }
    }
  }
]
```

## Explicit Sync and Status

The `*-confirmed` tasks enqueue work. `deliver-outbox` is independent: hooks
and sync steps write SQLite only, and delivery happens later.

`status` reports:

- `configured`
- `settings.service_url`
- `settings.api_key_configured`
- `settings.show_remote_results`
- `outbox.pending`, `outbox.delivered`, `outbox.quarantined`
- `outbox.last_error`
- `outbox.paused.active` and `outbox.paused.reason`
- `outbox.last_delivery_attempt`

Each delivery attempt is also appended as a redacted JSON line to
`recommendations.delivery.log` in the installed plugin directory. The log
records outcome, HTTP status, and error text, but never API keys or request
payloads.

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
6. Run `sync-metadata` with a configured Stash-box source; confirm snapshots are delivered and a source API key does not appear in the persisted `source_snapshots.snapshot` JSON payload.
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

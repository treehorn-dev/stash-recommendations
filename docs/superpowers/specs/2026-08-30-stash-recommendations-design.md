# Stash Recommendations v0 Design

## Purpose

Build a Stash plugin and operated recommendation service that collects explicit
scene ratings and recorded play/o engagement, enriches catalog entries from the
configured source Stash-box instances, and returns related/local
recommendations. The design supports a future split between the interaction
system of record and the recommendation service.

## Scope

V0 includes:

- A Stash plugin with a Python worker and JavaScript recommendation UI.
- A Go HTTP service backed by PostgreSQL.
- Explicit scene rating capture/removal plus historical and incremental play/o
  capture, durable retries, and client/server idempotency.
- Immutable engagement events and two rebuildable session projections.
- Client-proxied canonical metadata capture from configured Stash-box endpoints.
- PostgreSQL batch-built collaborative and content-based recommendations.
- Scene-page related recommendations and a personalized For You page.
- Local-scene resolution by default, with an opt-in remote-only presentation.

V0 excludes account registration, invitations, endpoint credential overrides,
direct endpoint crawling by the service, media downloading, player-level
instrumentation, and ANN indexing.

## Terms

- **StashID**: Stash's external identity pair `{ endpoint, stash_id }`.
- **Content key**: a normalized endpoint-qualified StashID.
- **Source**: a configured Stash-box endpoint that owns canonical metadata.
- **Engagement event**: an immutable `scene.played` or `scene.o` record derived
  from a timestamp already recorded by local Stash history.

## Architecture

The monorepo has three boundaries:

1. `plugin/`: Stash manifest, Python hook/task worker, JavaScript UI bundle,
   and a local SQLite outbox.
2. `server/`: Go JSON/HTTP API, PostgreSQL migrations, ingestion, catalog,
   recommendation reads, and batch model jobs.
3. `contracts/`: versioned JSON event/snapshot fixtures shared by plugin and
   server contract tests.

Stash is authoritative for local scenes. Each Stash-box endpoint is
authoritative for its public metadata. The recommendation service owns account
identity, interactions, metadata snapshots, and derived recommendation
projections.

The server is the v0 interaction system of record. Its domain code exposes
`InteractionSource`, `RecommendationStore`, and catalog interfaces so a later
interaction service can replace the storage source without changing the plugin
wire contract or recommendation HTTP API.

## Authentication and Privacy

The operated service issues recommendation API keys through an administrative
bootstrap process in v0. The plugin sends its configured API key as a bearer
credential. The server stores only a secure hash, account state, and key
metadata; it never stores a plaintext key.

The plugin is inert without an HTTPS service URL and valid API key. It uploads
no local scene identifiers, paths, files, file hashes, local tags, custom
fields, organization state, durations, resume positions, player configuration,
or source credentials. It uploads only normalized ratings and endpoint-qualified
play/o event kind and UTC timestamp. Each issued API key identifies an
independent account in v0; account registration, revocation, rotation, and
disassociation workflows are out of scope.

## Interaction Event Protocol

Events are versioned, append-only, and idempotent:

```json
{
  "schema_version": 1,
  "event_id": "uuid",
  "client_id": "uuid",
  "sequence": 1042,
  "kind": "scene.rating.set",
  "content": {
    "endpoint": "https://example.invalid/graphql",
    "stash_id": "source-scene-id"
  },
  "rating": 0.75,
  "occurred_at": "2026-08-30T12:34:56Z",
  "origin": "hook"
}
```

`scene.rating.remove` omits `rating`. The plugin converts Stash `rating100` to
`rating100 / 100`. Every rating fans out once per scene `stash_ids` value. The
client assigns a persistent `client_id`, monotonically increasing `sequence`,
and stable `event_id`; retries retain all three. The server deduplicates event
IDs and projects the newest event per account/content key into current
preferences.

`scene.played` and `scene.o` omit `rating`. The plugin imports every recorded
timestamp in local `play_history` and `o_history`, then incrementally imports
new timestamps. It emits one event per external StashID with only its content
key, UTC `occurred_at`, kind, and deterministic identity; deterministic IDs
allow historical sync reruns without duplication. The server retains engagement
events immutably and never derives them from player instrumentation.

## Sessionization

Model jobs rebuild two independent projections from an account's ordered
engagement events. Both include repeated scenes unless the same scene occurs
consecutively, in which case the run collapses to one session item.

- **Latency sessions** start a new session only when the gap between adjacent
  events is greater than two hours.
- **o-bounded sessions** end immediately at every `scene.o`; the o belongs to
  the closed session and a later play starts a new one even within two hours.
  An o without a preceding play implies a played one-scene closed session.

The projections intentionally overlap and may double count the same events.
They provide unordered co-occurrence and ordered scene-transition candidates.
The server configuration has an o-event weight, initially `1.5`, against a
play-event weight of `1.0`.

## Plugin Capture and Delivery

The worker listens to `Scene.Update.Post`. It performs rating work only when
`hookContext.inputFields` includes `rating100`; then it reads the current scene
from local Stash and emits a set/remove event for every external ID. The hook
only persists local work and returns promptly.

Initial rating and engagement sync are explicit user tasks. Rating sync
paginates rated local scenes; engagement sync imports all recorded history.
Both display counts before confirmation and queue events using the same
contract. A local SQLite outbox persists pending events and retries with
exponential backoff. A successful acknowledgement deletes an event.

Authentication errors pause delivery and surface a configuration error. Invalid
payloads are quarantined with visible status instead of retrying forever. Rate
limits honor server retry guidance. The plugin exposes pending, delivered,
quarantined, and last-error status.

## Source-Authoritative Metadata Capture

The plugin reads Stash's configured Stash-box endpoint/API-key pairs and never
uploads those credentials. For each content key whose endpoint has a matching
configured source, it directly queries the source Stash-box GraphQL API under
that source's configured request limit. The server never crawls third-party
endpoints.

The client maps source results into versioned snapshots limited to the
Stash-box schema. Scene snapshots include source scene fields and references;
performer, studio, tag, and relationship snapshots carry endpoint-qualified
identities and Stash-box-compatible fields. Remote image and URL references may
be retained, but media is never downloaded or proxied. Snapshots contain no
local Stash-only fields.

Unauthenticated or unconfigured sources still receive rating events but do not
produce metadata snapshots. Metadata capture runs during initial catalog sync,
opportunistically alongside rated-scene capture, and through a manual metadata
sync task. It deduplicates content keys and UPSERTs by endpoint-qualified ID,
preserving source update timestamps and snapshot version.

## Recommendations

V0 builds recommendations in PostgreSQL batch jobs, not request paths. Each
job reads current preferences and source snapshots, writes a new model version,
and atomically activates it after completion. Previous versions remain
available for rollback.

The first model combines:

- Item-to-item collaborative scores from overlapping, user-centered explicit
  ratings with shrinkage for small co-rating counts.
- Session co-occurrence and ordered transition scores from both session
  projections, weighted by event kind.
- Content-based candidate scores from source-authoritative scene, performer,
  studio, tag, group, and compatible Stash-box attributes.
- Per-user For You scores derived from ratings, engagement sessions, and all
  candidate sources.

Insufficient data returns an empty state. V0 has no global popularity fallback.
The candidate-generation boundary is replaceable: a later ANN/Annoy artifact
can generate candidates while PostgreSQL continues to own API filtering,
ranking, version metadata, and result projection.

## UI and Read APIs

The server exposes authenticated endpoints for related content and For You.
They return endpoint-qualified content keys, score/reason metadata, model
version, and a source canonical URL when known.

The scene page asks for related items using every StashID on the current scene.
The plugin resolves results through local Stash and displays only locally
available scenes by default. An opt-in setting includes remote-only results
that open the source canonical URL. The For You page follows the same local
default and remote-only option.

## Failure Handling

- Endpoint request failures do not block rating delivery; snapshot work retries
  independently and respects the source limit.
- Snapshot UPSERTs are idempotent by source identity and source update version.
- A model job failure leaves the previously active model version serving reads.
- The server rejects malformed, unsupported, or unauthenticated requests
  without accepting partial projections.
- Service unavailability affects only the local outbox, never Stash mutations.

## Verification

Plugin tests cover rating normalization/removal, all-ID fan-out, history import,
incremental play/o sync, session-event privacy, source selection, snapshot
mapping, source rate limits, outbox retry/quarantine, and local/remote result
resolution.

Server tests cover API-key hashing/authentication, idempotent interaction
ingestion, event ordering, current-preference projection, immutable engagement
storage, both session boundary algorithms, source snapshot UPSERTs, batch
version activation/rollback, ranking fixtures, and empty-state behavior.

Shared contract tests validate fixed JSON event and Stash-box-schema snapshot
fixtures against both client serialization and server validation. End-to-end
tests use a Stash fixture plus a mock Stash-box endpoint to prove that hooks,
source proxying, delivery, and UI resolution work together.

## Future Extensions

Account registration and invitation flows, account revocation/rotation and
disassociation, endpoint credential overrides, scheduled metadata sync,
player-level instrumentation (completion thresholds and autoplay/preview
filtering), and ANN candidate generation are future work.

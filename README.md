# ThorSync

ThorSync is a self-hosted game-save library and safety broker for an AYN Thor
and a Windows PC. Syncthing transports files; ThorSync identifies each game,
archives every observed revision, records which endpoint authored it, maps
RetroArch `.srm` files to standalone-emulator `.sav` files, and pauses instead
of guessing when progress diverges.

![ThorSync application icon](docs/assets/icon.svg)

## Supported v1 setup

| System | AYN Thor | Windows |
| --- | --- | --- |
| GBA | RetroArch mGBA (`.srm`) | VBA-M (`.sav`) |
| Nintendo DS | RetroArch melonDS DS (`.srm`) | melonDS (`.sav`) |

Only in-game/battery saves are supported. Savestates are deliberately excluded
because they are tied to emulator and core versions.

## What it does

- Provides a responsive visual library with game status, current source
  device, timestamps, immutable history, activity, conflicts, and diagnostics.
- Keeps the Thor and Windows Syncthing folders isolated and brokers only
  byte-preserving, profile-validated saves between them.
- Stores content-addressed SHA-256 blobs and separate observations, so the same
  bytes from two devices keep both provenance records without using twice the
  space.
- Automatically delivers only linear updates. A stale device, simultaneous
  offline play, Syncthing conflict file, invalid size, or unknown mapping
  creates a branch or quarantine state instead of overwriting progress.
- Restores an older revision by creating a new head; later history is never
  erased.
- Creates manual timeline snapshots without duplicating bytes or pretending an
  offline device received data it has not acknowledged.
- Imports RetroArch playlists or browser-computed ROM hashes. ROM bytes never
  leave the browser. Hash-to-title matching uses a generated, attributed
  Libretro Database subset.
- Validates Cloudflare Access application JWTs for one exact administrator.

## Architecture

```text
Thor RetroArch ⇄ BasicSync ⇄ hub folder: thor ┐
                                               ├─ ThorSync broker + archive
Windows emulators ⇄ Syncthing ⇄ hub folder: pc ┘
```

The Go service embeds the React application, exposes an internal JSON/SSE API,
uses SQLite in WAL/FULL mode, and talks to Syncthing only over the private
Compose network. The browser never receives the Syncthing API key.

## ZimaOS deployment

The included Compose stack runs ThorSync, a pinned Syncthing hub, and WUD. Code
merged to `main` is tested, published to public GHCR as `edge` plus an immutable
`sha-<commit>` tag, and picked up by WUD within five minutes. WUD watches only
the ThorSync container; Compose and infrastructure changes remain deliberate.

Start with [the ZimaOS deployment guide](docs/zimaos-deployment.md). Also read:

- [Updates and rollback](docs/updates-and-rollback.md)
- [Backup and restore](docs/backup-and-restore.md)
- [Security model](docs/security.md)

Persistent data defaults to `/DATA/AppData/thorsync`. The revision archive on
that same disk is protection from sync mistakes, not disaster recovery; back
up both `data/` and `archive/` to another system.

## Development

Requirements: Go 1.24+, Node.js 22+, and npm.

```sh
go test ./...
cd web
npm ci
npm test
npm run build
```

For a local unauthenticated development server:

```sh
THORSYNC_AUTH_MODE=disabled go run ./cmd/thorsync
```

On PowerShell, set the same environment value with
`$env:THORSYNC_AUTH_MODE='disabled'`. Local runtime directories are ignored by
Git. Production defaults to Cloudflare authentication and fails closed when
its issuer, audience, or administrator identity is missing.

The production Docker build compiles `web/dist` into the Go binary. CI runs Go
tests and vet, frontend tests and build, Compose validation, a container build,
and a liveness smoke test before publishing `edge`.

Regenerate the licensed hash catalogue with:

```sh
go run ./cmd/cataloggen
```

The generator is pinned to the upstream revision named in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md).

## Safety boundaries

- Close the game on device A, wait for ThorSync to show delivery, and only then
  open it on device B. No server can know that an emulator still holds stale
  SaveRAM in memory.
- “Last modified by” identifies the logical source when Syncthing can prove it.
  Reconciled or missed-event data is labelled inferred or unknown.
- Deletions are recorded as missing and never propagated automatically.
- Unsupported formats are archived but not deployed. ThorSync does not perform
  speculative save conversion.
- Every production mutation is same-origin, authenticated, idempotent where it
  can overwrite a live save, and recorded in the operation journal.

## Licensing

ThorSync source code is available under the [MIT License](LICENSE). The embedded
hash catalogue has separate CC BY-SA 4.0 attribution in
[THIRD_PARTY_NOTICES.md](THIRD_PARTY_NOTICES.md). No ROMs or third-party box art
are included.

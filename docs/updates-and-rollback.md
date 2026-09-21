# Updates and rollback

## Automatic application updates

Successful pushes to `main` publish two public GHCR tags:

- `edge`, the mutable deployment channel watched by WUD.
- `sha-<full commit SHA>`, an immutable rollback reference.

Publication occurs only after Go tests (including migration and archive integrity tests), frontend tests/build, Compose validation, a container build, and a liveness smoke test pass. After the first publish, set the GHCR package visibility to **Public** in the repository owner's package settings.

WUD checks every five minutes. Its watcher defaults to opt-out and only the `thorsync` container has `wud.watch=true`, digest watching, and the `docker.thorsync` update trigger. It follows the digest of the exact tag currently configured on that container. Syncthing and WUD use explicit versions and must be upgraded deliberately. WUD does not prune the previous ThorSync image.

WUD's Docker trigger can recreate the application container, but it does not provide a health-based rollback. Watch `/health/ready` and application logs after an update. Image updates preserve bind-mounted metadata, archive, and sync folders. A change to `compose.yaml`, `.env`, volumes, ports, secrets, or networks requires a deliberate ZimaOS redeploy with `docker compose up -d`.

## Pin a last-known-good image

1. Find the last-known-good full commit SHA in GHCR or GitHub Actions.
2. Edit `.env`:

   ```text
   THORSYNC_IMAGE=ghcr.io/YOUR_GITHUB_OWNER/thorsync:sha-FULL_COMMIT_SHA
   ```

3. Pull and recreate only ThorSync:

   ```sh
   docker compose pull thorsync
   docker compose up -d --no-deps thorsync
   curl --fail http://127.0.0.1:8080/health/live
   docker compose logs --tail=100 thorsync
   ```

An immutable `sha-...` tag never receives a new digest, so a SHA-pinned deployment stays pinned until `.env` is changed back to `edge`.

## Roll back a database migration

Before each schema or additive feature migration ThorSync writes an online
SQLite backup to:

```text
/DATA/AppData/thorsync/data/migration-backups/
  thorsync-before-v<TARGET>-<UTC_TIMESTAMP>.db
  thorsync-before-revision-payloads-<UTC_TIMESTAMP>.db
```

The standalone-mGBA payload migration is additive and deliberately keeps
SQLite `user_version=1`, so the immediately previous image can still open the
database. The current image checks and backfills component metadata on every
startup, including revisions made during a temporary rollback. The newest ten
migration backups are retained. Prefer an image-only rollback first; restore a
database backup only when the previous image cannot open the migrated database.

1. Stop ThorSync so no process has the SQLite database or WAL open:

   ```sh
   docker compose stop thorsync
   ```

2. Make a safety copy of the entire current `data` directory.
3. Move the current database, its `-wal`, and its `-shm` sidecars to a dated recovery directory. Do not delete them.
4. Copy the matching pre-migration `.db` backup into the database's normal filename, retaining the application UID/GID and restrictive permissions.
5. Pin the immediately previous `sha-...` image and start ThorSync.
6. Check liveness, readiness, logs, game count, revision history, and a sample archive blob before resuming sync activity.

Never replace the archive when rolling back metadata unless a separately verified backup proves the archive and database were captured as one recovery point. Do not enable automatic propagation while validating a rollback.

See WUD's [Docker trigger](https://getwud.app/docs/configuration/triggers/docker/) and [container-label](https://getwud.app/docs/configuration/watchers/labels/) references for the updater controls used by this stack.

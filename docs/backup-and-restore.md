# Backup and restore

ThorSync's content-addressed archive is protection against device overwrites and conflicts, not disaster recovery when it resides on the same ZimaOS disk. Back up both the metadata and archive to a different physical system or storage provider.

## What to protect

Required recovery set:

- `/DATA/AppData/thorsync/data` — SQLite metadata, WAL files, event cursor, and migration backups.
- `/DATA/AppData/thorsync/archive` — immutable save blobs.

For standalone mGBA, the archive contains the exact physical `.sav` occurrence
as well as deduplicated battery and optional 16-byte RTC components. The SQLite
metadata in `data/` is what associates those blobs into a logical revision, so
neither directory is a complete backup by itself.

Recommended additions:

- `/DATA/AppData/thorsync/syncthing` — hub identity and folder/device configuration.
- `/DATA/AppData/thorsync/secrets` — only into an encrypted backup with equally restrictive access.
- The deployed `compose.yaml` and `.env` (again, encrypt `.env` if it contains local infrastructure details).

The live endpoint folders under `sync/` are useful for faster recovery but are not a substitute for the database and archive. WUD state is disposable.

## Consistent offline backup

This is the simplest reliable procedure and causes a short interruption:

```sh
docker compose stop thorsync
# Run the configured ZimaOS backup/snapshot job for data and archive here.
docker compose start thorsync
curl --fail http://127.0.0.1:8080/health/live
```

Stopping only ThorSync is sufficient for the database and archive. Syncthing can continue receiving endpoint files; ThorSync will reconcile them after restart. A filesystem snapshot must capture `data` and `archive` in the same recovery point. If the backup tool cannot provide an atomic multi-directory snapshot, stop ThorSync for the full copy window.

Schedule the backup according to acceptable save-loss tolerance, verify completion off-host, and retain several dated generations. Periodically restore into a disposable directory and verify checksums rather than relying solely on a successful-job indicator.

## Restore drill

1. Disable the WUD container so it cannot change the application image during recovery.
2. Stop ThorSync.
3. Rename the current `data` and `archive` directories to dated recovery names; do not delete them.
4. Restore a matched `data` + `archive` recovery point into their normal paths.
5. Restore ownership to the configured `PUID:PGID` and directories to mode `0750`.
6. Pin the ThorSync image SHA that was in use at backup time, then start only ThorSync.
7. Verify `/health/live`, `/health/ready`, logs, revision counts, and several archived hashes.
8. Start WUD only after validation. Reconcile endpoint folders before re-enabling propagation.

If Syncthing identity/configuration is also restored, keep the same hub device identity. Restoring an old Syncthing database can cause a large rescan; review folder state before accepting unexpected changes.

## Recovery boundaries

- Device-side deletion is recorded as “missing on device” and is not propagated, so a healthy server archive can restore it through the UI.
- Quota or free-space protection can pause capture. A backup does not make it safe to ignore a paused state; free local capacity first.
- The revision archive keeps every accepted blob indefinitely. Backup retention policy may be finite, but it should provide enough generations to recover from unnoticed corruption or operator error.

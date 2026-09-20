# Security model and operations

ThorSync is a single-administrator application. Treat save data, game names, activity times, device identifiers, and cover uploads as private data.

## Request authentication

All browser and API requests except detail-free liveness/readiness endpoints are expected to arrive through Cloudflare Access. ThorSync validates the `Cf-Access-Jwt-Assertion` signature and timing, fixed issuer, fixed audience, application-token type, and the exact email in `/run/secrets/admin_email`. Invalid or unavailable authentication fails closed. CORS is disabled; state-changing requests must be same-origin.

Do not expose a second unauthenticated route around Cloudflare. The host port binds to loopback by default. Cloudflare Tunnel reaches `http://thorsync:8080` over the external Docker network without publishing the container directly to the LAN or internet.

The detail-free health endpoints indicate process/dependency state only. They must not disclose paths, game names, device IDs, email addresses, token claims, or configuration values.

## Secrets

These files are mounted read-only under `/run/secrets`:

- `syncthing_api_key`
- `cf_audience`
- `admin_email`

Keep host copies at mode `0600`, owned by the configured application UID/GID. Never put their values in Compose, `.env`, logs, screenshots, issue reports, container labels, or the repository. Rotate the Syncthing API key after suspected disclosure. Changing Cloudflare audience or administrator requires changing the secret and recreating ThorSync.

## Container boundaries

ThorSync runs as the configured non-root UID/GID with all Linux capabilities dropped, `no-new-privileges`, a read-only root filesystem, and only its metadata, archive, endpoint folders, and temporary directory writable. It does not mount the Docker socket.

Syncthing has write access only to its state and the two endpoint folders. Its GUI is loopback-only by default and must have its own strong username/password before a LAN binding is enabled. Expose sync/discovery ports only through the ZimaOS firewall; do not forward the GUI port from the router.

WUD is the deliberate exception: write access to `/var/run/docker.sock` is effectively host-root authority. Its HTTP server is disabled, it runs on a separate bridge, its image is pinned, it watches no container by default, and only ThorSync opts into its automatic Docker trigger. Anyone who can change the WUD container, its configuration, or its persisted data should be treated as a ZimaOS administrator. Remove or stop WUD if this risk is not acceptable and perform image updates manually.

## Supply chain and update policy

- CI publishes only `linux/amd64` images after tests, Compose validation, and a live-container smoke test.
- Each deployment has an immutable `sha-<commit>` tag and provenance/SBOM attestations in GHCR.
- `edge` is mutable and receives automatic updates; Syncthing and WUD are pinned and manually reviewed.
- WUD retains the old image but does not provide automatic health rollback. Keep a known-good SHA recorded outside the server.
- GHCR should be public for unauthenticated pulls. Never bake credentials or secret files into the image or build context.

## Operational checklist

- Keep ZimaOS and Docker security updates current.
- Limit SSH, ZimaOS administration, and Syncthing GUI access to trusted LAN/VPN clients.
- Review Cloudflare Access policy and administrator email after account changes.
- Review `docker compose logs` for authentication failures, quarantines, paused propagation, quota warnings, and updater actions.
- Back up database and archive off-host and test restores.
- Close emulators before restore/promotion and wait for delivered status before opening a game on the other device.
- Investigate any unexpected source-attribution change; ThorSync marks uncertain provenance as inferred or unknown rather than guessing from connection timing.

# Deploy ThorSync on ZimaOS

This stack runs ThorSync, a pinned Syncthing hub, and a pinned WUD updater. ThorSync is the only service WUD is allowed to update. The application port and Syncthing administration port bind to loopback by default; Cloudflare Tunnel reaches ThorSync through a dedicated external Docker network.

## Prerequisites

- An amd64 ZimaOS host with Docker Compose v2.
- A Cloudflare Tunnel and Access application for one administrator.
- An existing public GHCR package built from this repository.
- BasicSync on the AYN Thor and Syncthing on Windows.
- At least 1 GiB of free disk space beyond the planned archive size.

The host paths below are deliberately stable so ZimaOS upgrades and container recreation do not move the data.

## 1. Prepare storage and secrets

Copy `.env.example` to `.env`. Set `THORSYNC_IMAGE`, the Cloudflare team issuer, the ZimaOS UID/GID that will own the files, and the host's timezone. Then create the persistent directories:

```sh
sudo install -d -m 0750 -o 1000 -g 1000 \
  /DATA/AppData/thorsync/data \
  /DATA/AppData/thorsync/archive \
  /DATA/AppData/thorsync/sync/thor \
  /DATA/AppData/thorsync/sync/windows \
  /DATA/AppData/thorsync/syncthing \
  /DATA/AppData/thorsync/wud
sudo install -d -m 0700 -o 1000 -g 1000 /DATA/AppData/thorsync/secrets
```

Replace `1000:1000` if `.env` uses another `PUID:PGID`. Create these newline-free files and make them readable only by that account:

| File | Initial value |
| --- | --- |
| `secrets/cf_audience` | Cloudflare Access application AUD tag |
| `secrets/admin_email` | Exact administrator email permitted by Access |
| `secrets/syncthing_api_key` | A random 256-bit value, for example from `openssl rand -hex 32` |

```sh
sudo chmod 0600 /DATA/AppData/thorsync/secrets/*
sudo chown 1000:1000 /DATA/AppData/thorsync/secrets/*
```

Do not commit `.env` or any secret file. Avoid shell history when entering their contents; use a local editor with a restrictive umask.

The stack reads `syncthing_api_key` into Syncthing's supported
`STGUIAPIKEY` override and gives the same file to ThorSync. Do not replace it
from the Syncthing GUI unless you also update the secret and recreate both
containers.

## 2. Join the Cloudflare network

`compose.yaml` expects an external network named by `CLOUDFLARE_NETWORK` (default `thorsync-cloudflare`). Reuse the external network already attached to `cloudflared`, or create and attach one:

```sh
docker network create thorsync-cloudflare
docker network connect thorsync-cloudflare YOUR_CLOUDFLARED_CONTAINER
```

Point the tunnel's private service at `http://thorsync:8080`. Protect the hostname with a Cloudflare Access self-hosted application whose policy permits only the email in `admin_email`. Set its AUD tag in `cf_audience` and set `THORSYNC_CF_ISSUER` to `https://YOUR_TEAM.cloudflareaccess.com`.

ThorSync derives the JWKS URL from that issuer. Set `THORSYNC_CF_JWKS_URL` only when a nonstandard endpoint is genuinely required.

## 3. Start and secure Syncthing

Validate and start the stack from the directory containing `compose.yaml`:

```sh
docker compose config -q
docker compose up -d syncthing wud
```

The Syncthing GUI binds to `127.0.0.1:8384` by default. Reach it with an SSH tunnel:

```sh
ssh -L 8384:127.0.0.1:8384 YOUR_ZIMA_USER@YOUR_ZIMA_HOST
```

Alternatively, set `SYNCTHING_GUI_BIND_ADDRESS` to the ZimaOS host's LAN address. Never bind the GUI to a WAN-facing address. Immediately configure a GUI username and strong password. The API key is already supplied from the secret file.

Pair the AYN Thor and Windows PC with the hub, but do not share a common save
folder. During ThorSync onboarding, enter both device IDs; ThorSync will create
exactly two isolated folders with 90-day staggered Syncthing versioning:

| Folder ID | Container path | Share with |
| --- | --- | --- |
| `thorsync-thor` | `/var/syncthing/sync/thor` | AYN Thor only |
| `thorsync-windows` | `/var/syncthing/sync/windows` | Windows PC only |

The same folders can be created manually in the Syncthing UI if onboarding
cannot reach its configuration API. Do not share either endpoint folder
directly with the other endpoint. That isolation is what lets ThorSync
validate, archive, map `.srm`/`.sav`, and detect divergent histories before
propagation.

Docker bridge networking does not reliably forward Syncthing's LAN-discovery broadcasts. Global discovery still works, but for predictable local transfers configure the hub on each endpoint as `tcp://ZIMA_LAN_IP:22000` (and configure direct endpoint addresses on the hub when needed). Keep TCP and UDP 22000 allowed by the ZimaOS LAN firewall.

On the Thor, keep saves in ordinary shared storage such as `Emulation/Saves`; BasicSync cannot reliably access Android private application directories. Configure the RetroArch mGBA and melonDS DS profiles during ThorSync onboarding. On Windows, select the corresponding VBA-M and standalone melonDS save directories.

## 4. Start ThorSync

```sh
docker compose up -d
docker compose ps
curl --fail http://127.0.0.1:8080/health/live
curl --fail http://127.0.0.1:8080/health/ready
```

`/health/ready` may report a degraded dependency while Syncthing or Cloudflare JWKS is temporarily unavailable; that should not produce a restart loop. Open the Cloudflare-protected hostname to complete onboarding. Leave propagation disabled until initial saves have been inventoried and reconciled.

## ZimaOS custom-app import

The top-level `x-casaos` section identifies `thorsync` as the main service and advertises port 8080. ZimaOS can import `compose.yaml` as a custom app after `.env`, directories, secret files, and the external network exist. If ZimaOS copies the Compose file into its own application directory, keep that deployed copy under configuration management; Compose changes are not delivered by container image updates.

## Useful checks

```sh
docker compose ps
docker compose logs --tail=100 thorsync
docker compose logs --tail=100 syncthing
docker compose logs --tail=100 wud
docker inspect --format '{{.State.Health.Status}}' thorsync
```

The application image is published for `linux/amd64`. Syncthing TCP/QUIC port 22000 and local discovery UDP port 21027 are published. WUD's HTTP server is disabled and it has no published UI port.

## Reference documentation

- [ZimaOS Compose and `x-casaos`](https://www.zimaspace.com/docs/developer/app-store-compose-x-casaos)
- [Syncthing's official Docker guidance](https://github.com/syncthing/syncthing/blob/main/README-Docker.md)
- [Cloudflare Access JWT validation](https://developers.cloudflare.com/cloudflare-one/access-controls/applications/http-apps/authorization-cookie/validating-json/)
- [Android shared-storage restrictions](https://developer.android.com/about/versions/11/privacy/storage)

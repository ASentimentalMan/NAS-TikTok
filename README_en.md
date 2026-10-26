<h1 align="center">
  NAS-TikTok
</h1>

<p align="center">
 <a href="README.md">简体中文</a> | <a href="README_en.md">English</a>
</p>

<p align="center">
<a><img src="https://img.shields.io/badge/license-GPLv3-orange"/></a>
<a><img src="https://img.shields.io/badge/PRs-welcome-brightgreen"/></a>
<a><img src="https://img.shields.io/badge/Powered%20by-Go-blue"/></a>
<a><img src="https://img.shields.io/badge/Powered%20by-Vue.js-blue"/></a>

</p>

Are you overwhelmed by too many resources on your NAS and don't know what to watch? Now, `NAS-TikTok` is here to solve that problem! `NAS-TikTok` is a project featuring a frontend that mimics `TikTok` ，and a backend that randomly samples your media library. It provides a smooth and fluid experience for Browse your NAS resources, just like scrolling through TikTok!

## Technology Stack

### Backend Go

1. Provides a single-process API, login authentication, CAPTCHA, and media streaming.

2. Uses a persistent SQLite media index, falling back to disk scans while the index is unavailable.

3. Supports configurable HTTP or HTTPS; HTTPS requires your own certificates.

4. Supports IP allowlists and optional IPv6 listening.

### Frontend Vue.js

1. Supports random jump to video timestamp (even if you swipe to the same video, a different segment will play).

2. Supports 2x speed playback / video rotation.

3. Supports image viewing / magnification / rotation / autoplay.

4. Operation buttons are hidden by default and only shown on touch, keeping the view unobstructed.

5. Supports cache-free PWA: iOS devices can add it to the home screen from Safari (Android not tested) and use it as an app, with no data cached on the device.

## Online preview

[NAS-Tiktok Preview](https://dev.engraved.cn/nas-tiktok)

account：preview

password：preview

## Running

Note: This project is for learning and research purposes only and must not be used for commercial purposes.

### Development

---

The backend is written in Go. Node.js/npm are used for Vue frontend development and builds. You need Go 1.24 or later and Node.js 22.23.3 (the version used by the project CI).

Clone the project and create the media directory:

```bash
git clone https://github.com/ASentimentalMan/NAS-TikTok.git
cd NAS-TikTok
mkdir -p static
```

Put the videos and images you want to browse in `static/`. Open a terminal at the project root and start the Go backend. Keep this terminal running:

```bash
cd server
ACCOUNT=demo \
PASSWORD=change-me \
JWT_SECRET='local-development-only-secret-replace-before-sharing' \
APP_URL_PREFIX= \
API_BASE=/api \
LISTEN_PORT=3000 \
MEDIA_ROOT=../static \
go run ./cmd/nas-tiktok
```

For local runs, set `APP_ENV=development` to load `server/.env.development`; process environment variables take precedence over file values. Use `server/.env.example` as a starting point for new configuration.

Open a second terminal at the project root and start the frontend:

```bash
cd client
npm ci
npm run dev
```

Vite proxies `/api` requests to the backend. Open [http://127.0.0.1:5173](http://127.0.0.1:5173) and log in with account `demo` and password `change-me`. If your terminal does not start in the project root, first change to the cloned `NAS-TikTok` directory. The example secret is for local development only; replace it with a random secret of at least 32 bytes before sharing or deploying the app. Do not commit real credentials.

Common Go settings: `LISTEN_HOST` (default `0.0.0.0`), `LISTEN_PORT` (default `3000`, or falls back to `VITE_API_PORT`), `MEDIA_ROOT` (default `server/static`), `STATIC_ROOT` (default `client/dist`), optional `APP_URL_PREFIX`, `API_BASE` (optional frontend API base; defaults to same-origin `/api`), `SSL` (default `false`), `TLS_CERT_FILE` and `TLS_KEY_FILE` (default to `certs/fullchain.pem` and `certs/privkey.pem`), `ALLOWED_IPS` (unset or empty by default; this only controls the source-IP allowlist, empty means no source-IP restriction and does not disable account/password login or other security checks, and configured IP/CIDR entries restrict access to matching sources), `ALLOW_IPv6`, `ACCOUNT`, `PASSWORD`, `JWT_SECRET`, and `JWT_EXPIRE` (default 1200 seconds). The relative defaults for `MEDIA_ROOT` and `STATIC_ROOT` are resolved from the Go process's current working directory at startup. The local example runs from `server/`, so it explicitly sets the media directory to `../static`; if the Go service serves the frontend, the default static directory is `server/client/dist`. The Docker image sets both paths to absolute locations. `COOKIE_SAME_SITE` defaults to `lax`, and `COOKIE_SECURE` defaults to the value of `SSL`. For cross-site cookies, list the frontend origin in `CORS_ORIGINS` and set `COOKIE_SAME_SITE=none` and `COOKIE_SECURE=true`; browsers also require HTTPS for cross-site cookies. Forwarded client-IP headers are trusted only from proxy ranges configured in `TRUSTED_PROXIES`. `ACCOUNT`, `PASSWORD`, and `JWT_SECRET` are required at runtime. When HTTPS is enabled, the certificate and key must be readable. Root-path deployment needs no extra setting. For a subpath, set `APP_URL_PREFIX` at runtime and the matching `VITE_BASE_URL` at build time (for example, `/photos/`).

`LOG_LEVEL` defaults to `info` and records login results and index initialization, recovery, completion, or failure events. `debug` also records redacted request diagnostics and per-directory scan progress. `off` suppresses routine runtime logs while critical errors remain visible for diagnosis. Same-origin HTTPS behind a reverse proxy does not require a permissive `CORS_ORIGINS` default: Go must receive the external Host including its port. If the Go upstream uses HTTP, `TRUSTED_PROXIES` must match the direct peer address Go actually sees, and the proxy must forward the real external scheme. For a proxy chain, verify `X-Forwarded-Proto` against the actual TLS termination point.

`DB_PATH` defaults to `db/nas-tiktok.db`, relative to the Go process's current working directory. Media lists use SQLite sampling when the index is ready; the server scans the media directory while building or rebuilding an index. After adding or removing NAS files, an authenticated session can call `GET {APP_URL_PREFIX}/api/reinit` (or `/api/reinit` at the root) to start a background rebuild. The request needs no body and returns `202` when started, or `409` if another rebuild is already running.

### Deployment

---

#### Docker Internal Structure

```bash
/workspace
├── certs/ # Certificate folder
│   ├── fullchain.pem # Certificate
│   └── privkey.pem # Private key
├── db/ # Persistent SQLite media index
├── static/ # Read-only media directory
└── client/dist/ # Vue static files generated during the Docker build
/usr/local/bin/nas-tiktok # Single-process Go service
```

#### Local build and deployment

```bash
docker build -t nas-tiktok:local .
```

The `Dockerfile` builds Vue `client/dist` with Node.js, then builds the backend with Go 1.24. The final image runs one Go process to serve both the API and frontend files. Account credentials and the JWT signing key are supplied at runtime and are not copied into the final image. The local Docker image build and container startup with a temporary media library and persistent database directory were verified; real NAS mounts and production deployment were not tested.

Building requires BuildKit because the Dockerfile uses a read-only mount from another build stage; starting an image from an exported tar does not require BuildKit.

At deployment, SQLite data is stored in `$HOME/workspace/NAS-TikTok/db/` and mounted read-write at `/workspace/db`. The media and certificate directories remain read-only mounts.

On first startup, the new image's initializer reads Docker's actual UID:GID and adjusts ownership of the container-owned `/workspace` tree, including `db/` and `client/dist/`, while preserving existing permission modes and never using `1777`. If `/workspace` itself is a mountpoint, it leaves the host-backed tree untouched; media, certificates, and other mountpoints are skipped. A dedicated read-write `/workspace/db` volume is the exception: only that directory and the SQLite file named by `DB_PATH` plus its `-wal`, `-shm`, and `-journal` files are handled; read-only database volumes are left unchanged. `DB_PATH` does not expand initialization beyond `/workspace`, and paths outside `/workspace` are never chowned. Once ownership matches, later starts perform only an O(1) metadata check and do not repeat the traversal or chown.

The initializer uses the image's `CAP_CHOWN` file capability only during startup; before executing the Go server, it clears capabilities and sets `no_new_privs`. If the runtime enables `no-new-privileges` or removes `CHOWN`, prepare the directory and volume ownership to match the container UID:GID in advance.

`build.sh` builds from the Git worktree containing the script. It does not fetch or switch branches. The image records the source Commit; builds from a worktree with uncommitted changes are marked `dirty`. Before deployment, create a Docker `KEY=value` runtime environment file at `$HOME/workspace/NAS-TikTok/.env` with at least `ACCOUNT`, `PASSWORD`, and a `JWT_SECRET` of 32 bytes or more. For HTTPS, set `SSL=true` and place `fullchain.pem` and `privkey.pem` in the sibling `certs/` directory; put media in `static/`. Root-path deployment needs no extra setting. For a subpath, set `APP_URL_PREFIX=photos` in the runtime `.env`, then build the matching frontend with `VITE_BASE_URL=/photos/ bash ./build.sh`. Run `bash ./build.sh` and choose build, deploy, or build-and-deploy. Deployment prompts for `host` or `bridge` networking and the service port. If there is no old container and `ss` is available, the script makes a best-effort check for a local TCP listener; Docker may still report a port conflict when starting the container. It asks whether `.env` enables HTTPS; with the default certificate paths it checks that the certificate and key files exist, are nonempty, and are readable by the current user. Custom paths are checked by the container startup health check. Media and certificate directories are mounted read-only. The script creates a candidate container and waits up to 90 seconds for an in-container loopback health check. It removes the old container only after the candidate is healthy and attempts to restore the old container's previous running state if startup or health checking fails. The health check requests `/healthz` under `APP_URL_PREFIX`; HTTPS probes skip local certificate trust verification. Docker deployment has not been validated.

With `host` networking, the service listens directly on the NAS port. With `bridge` networking, the script publishes the selected port. Bridge networking usually exposes the Docker bridge-side source address to the app; configure trusted proxies for the actual topology if client IP identification through a reverse proxy is needed. The container does not restrict source IPs by default (`ALLOWED_IPS` is unset or empty); this does not disable login authentication or other security checks. To enable the allowlist, set comma-separated IP or CIDR entries in `.env`, based on the source addresses the app actually sees.

To start the image offline on another NAS, export it on the build machine with `docker save` and import it on the NAS with `docker load`. Image build and import have not been validated.

## Features and Suggestions

If you have any features or suggestions, please feel free to raise them in the `Issues`

If you are willing to contribute to this project, pull requests are welcome! Thank you very much for your support!

## Contact Me

You can contact me via email: <a href="mailto:ASentimentalMan@GMail.com">ASentimentalMan@GMail.com</a>

## License

[GPLv3](LICENSE)

# xray-pilot

Self-hosted Xray proxy management for small teams. Push configs over SSH, detect config drift, and generate multi-format subscriptions from one embedded Go binary.

[中文文档](./README.zh-CN.md)

## Features

- Node lifecycle management for Xray Reality deployments
- Config drift detection with scheduled SHA256 comparisons
- User and group management for subscription access control
- Subscription generation with health-aware node selection
- SSH-based sync with atomic uploads and remote service reloads
- Single-binary deployment with the frontend embedded via `go:embed`
- Runtime system settings managed from the web UI and persisted in the database

## Screenshots

Place screenshots in [`docs/screenshots/`](./docs/screenshots/) with the following naming convention:

- `01-login.png`
- `02-dashboard.png`
- `03-users.png`
- `04-profiles.png`
- `05-settings.png`
- `06-subscribe.png`

Recommended capture order:

1. Login screen
2. Dashboard overview
3. User management
4. Protocol configuration
5. System settings
6. Subscription portal

> The current admin UI is primarily Chinese. Keeping `README.md` in English is still recommended for project discovery, while the screenshots and the Chinese manual provide localized context.

### Login

![Login](./docs/screenshots/01-login.png)

Secure access to the control panel with a lightweight sign-in screen, theme toggle, GitHub link, and clear operator-facing branding.

### Dashboard

![Dashboard](./docs/screenshots/02-dashboard.png)

The dashboard highlights node health, active users, subscription delivery, recent operations, and the current control-plane status in a single view.

### User Management

![Users](./docs/screenshots/03-users.png)

Manage subscription users, copy subscription links, open QR codes, assign groups, control expiration, and toggle access with consistent confirmation flows.

### Protocol Configuration

![Profiles](./docs/screenshots/04-profiles.png)

Define protocol templates, attach node-specific key material, and keep shared settings separate from node overrides for clearer operations.

### System Settings

![Settings](./docs/screenshots/05-settings.png)

Inspect runtime diagnostics, review deployment hints, and manage database-backed system settings without editing config files manually.

### Subscription Portal

![Subscribe](./docs/screenshots/06-subscribe.png)

Render a browser-friendly subscription page with QR import, node-level copy actions, theme support, and smart fallback links while keeping client access on the same `/sub/{token}` endpoint.

## Tech Stack

| Layer | Technology |
| --- | --- |
| Backend | Go 1.26.1+, Gin, GORM, Zap |
| Database | SQLite by default, PostgreSQL optional |
| Auth | JWT bearer tokens, bcrypt |
| Crypto | AES-GCM for secrets at rest, SHA256 for drift checks |
| Frontend | React 19, Vite, TypeScript, Tailwind CSS v4 |

## Installation

### One-line install

Linux `amd64` and `arm64` hosts can install the latest release with:

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/install.sh | sudo bash
```

The installer will:

- detect `amd64` or `arm64`
- download the latest GitHub Release
- verify `checksums.txt`
- install `xray-pilot` to `/usr/local/bin/xray-pilot`
- create `/etc/xray-pilot/config.yaml`
- create `/etc/xray-pilot/ssh/` for service-managed SSH keys
- create and start the `xray-pilot.service` systemd unit
- generate a random admin password, JWT secret, and crypto master key

### Manual install

| Platform | Archive |
| --- | --- |
| Linux amd64 | `xray-pilot_<version>_linux_amd64.tar.gz` |
| Linux arm64 | `xray-pilot_<version>_linux_arm64.tar.gz` |
| macOS amd64 | `xray-pilot_<version>_darwin_amd64.tar.gz` |
| macOS arm64 | `xray-pilot_<version>_darwin_arm64.tar.gz` |

Manual installation steps:

1. Download the matching archive and `checksums.txt` from the latest [GitHub Release](https://github.com/imrui/xray-pilot/releases).
2. Verify the archive with `sha256sum -c checksums.txt --ignore-missing`.
3. Extract the binary and place it in your preferred location.
4. Copy [`config.yaml.example`](./config.yaml.example) to `config.yaml` and adjust the values.
5. Start the service with `./xray-pilot`.

## Upgrade

Existing Linux installations can be upgraded by re-running the installer:

```bash
curl -fsSL https://raw.githubusercontent.com/imrui/xray-pilot/main/install.sh | sudo bash
```

The upgrade process will:

- replace `/usr/local/bin/xray-pilot` with the latest release binary
- keep the existing `/etc/xray-pilot/config.yaml`
- keep the existing `/etc/xray-pilot/ssh/` key directory
- keep the existing SQLite database or external database settings
- reload and restart `xray-pilot.service`

After the upgrade, you can confirm the running version with:

```bash
journalctl -u xray-pilot -n 20 --no-pager
```

## Quick Start From Source

**Prerequisites:** Go 1.26.1+, Node.js 24+

```bash
git clone https://github.com/imrui/xray-pilot.git
cd xray-pilot

make build

cp config.yaml.example config.yaml
# edit jwt.secret and crypto.master_key before first production run

./xray-pilot
```

Open `http://localhost:2026` and sign in with the administrator account defined in `config.yaml`.

## Configuration

Copy [`config.yaml.example`](./config.yaml.example) to `config.yaml`.

```yaml
server:
  port: 2026
  mode: release

database:
  driver: sqlite
  dsn: xray-pilot.db

jwt:
  secret: "change-me-use-a-long-random-string"
  expire: 24

crypto:
  master_key: ""

ssh:
  default_port: 22
  default_user: "root"
  default_key_path: ""
  known_hosts_path: "/var/lib/xray-pilot/known_hosts"

admins:
  - username: admin
    password: "change-me-now"
```

For Linux service deployments, the recommended SSH key location is `/etc/xray-pilot/ssh/id_ed25519`. The service-owned known_hosts file is stored at `/var/lib/xray-pilot/known_hosts`.

Runtime settings such as scheduler intervals, SSH defaults, subscription formatting, and Xray log options are stored in the database-backed system settings table and managed from the web UI.

Environment variable override:

```bash
XRAY_PILOT_MASTER_KEY=<hex-key> ./xray-pilot
```

Priority order:

1. `XRAY_PILOT_MASTER_KEY`
2. `config.yaml -> crypto.master_key`
3. auto-generated key on first start

## Development

```bash
# backend
make dev-backend

# frontend
make dev-frontend
```

The frontend dev server proxies API requests to `http://localhost:2026`.

## Release Automation

Pushing a tag named `v*` triggers GitHub Actions to:

1. build the frontend with Node 24
2. compile release binaries with Go 1.26
3. package `tar.gz` archives for Linux and macOS on `amd64` and `arm64`
4. generate `checksums.txt`
5. publish a GitHub Release with all artifacts attached

## License

MIT. See [LICENSE](./LICENSE).

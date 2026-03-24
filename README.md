# xray-pilot

Self-hosted Xray proxy management for small teams. Push configs via SSH, auto-detect config drift, and generate multi-format subscriptions. Built with Go + React.

## Features

- **Node management** — add Xray Reality nodes, generate x25519 keypairs, push configs via SSH
- **Config drift detection** — scheduled SHA256 comparison between local and remote configs; auto-marks drifted nodes
- **User & group management** — assign users to groups, control per-user subscription access
- **Subscription links** — generate VLESS URI subscription URLs; health-first node selection with fallback
- **SSH sync** — upload config atomically (`tmp` → `mv`), reload Xray via `systemctl`/`service`
- **Health checks** — periodic TCP probe; tracks latency and last-check status per node
- **Single binary** — frontend embedded via `go:embed`, no separate static server needed
- **Configurable scheduler** — drift check and health check intervals adjustable at runtime via API

## Tech Stack

| Layer | Technology |
|-------|-----------|
| Backend | Go 1.26.1+, Gin, GORM, Zap |
| Database | SQLite (default) / PostgreSQL |
| Auth | JWT (Bearer token) + bcrypt |
| Crypto | AES-GCM (private key at rest), SHA256 (config hash) |
| Frontend | React 19, Vite, TypeScript, Tailwind CSS v4 |
| State | TanStack Query, Zustand |

## Quick Start

**Prerequisites:** Go 1.26.1+, Node.js 24+

```bash
# Clone
git clone https://github.com/imrui/xray-pilot.git
cd xray-pilot

# Build frontend + Go binary
make build

# Copy and edit config (required before first run)
cp config.yaml.example config.yaml
# Edit config.yaml: set jwt.secret and crypto.master_key

# Run
./xray-pilot
```

Open `http://localhost:8080` and call the setup endpoint to create the admin account:

```bash
curl -X POST http://localhost:8080/api/setup \
  -H "Content-Type: application/json" \
  -d '{"username":"admin","password":"your-password"}'
```

**Development mode (hot reload):**

```bash
# Terminal 1 — backend
make dev-backend

# Terminal 2 — frontend (proxies API to :8080)
make dev-frontend
```

## Configuration

Copy `config.yaml.example` to `config.yaml`. Key settings:

```yaml
server:
  port: 8080
  mode: release          # debug | release

database:
  driver: sqlite         # sqlite | postgres
  dsn: xray-pilot.db    # for postgres: "host=... dbname=xray_pilot ..."

jwt:
  secret: "change-me"   # Must be changed in production
  expire: 24            # Token TTL in hours

crypto:
  master_key: ""        # AES-GCM key for encrypting node private keys.
                        # Leave blank to auto-generate; save the generated
                        # key back here before restarting.

scheduler:
  drift_check_interval: 300   # Seconds between config drift checks (0 = off)
  health_check_interval: 120  # Seconds between TCP health checks (0 = off)
```

**Environment variable override:**

```bash
XRAY_PILOT_MASTER_KEY=<base64-key> ./xray-pilot
```

Priority: `env XRAY_PILOT_MASTER_KEY` > `config.yaml crypto.master_key` > auto-generate.

## Architecture

```
Browser / Subscription clients
        |
   [ Gin HTTP ]          ← single binary (frontend embedded via go:embed)
        |
   [ Handlers ]          ← request validation, response shaping
        |
   [ Services ]          ← business logic (sync, subscribe, drift, auth)
        |
   [ Repository ]        ← GORM data access (SQLite / PostgreSQL)
        |
   [ SQLite / PG ]

Background:
   [ Scheduler ] ──── drift check ──→ SSH read remote config → SHA256 compare
                 └─── health check ─→ TCP probe → update LastCheckOK / latency

Node communication:
   Service → pkg/ssh → SSH tunnel → upload config.json → systemctl restart xray
```

## License

MIT License — see [LICENSE](LICENSE)

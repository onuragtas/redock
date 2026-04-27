# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

**Redock DevStation** is an all-in-one local development environment manager written in Go with a Vue 3 frontend. It provides a single binary that runs as a system service, offering: API Gateway (HTTP/HTTPS reverse proxy), DNS server with filtering, Docker environment management, tunnel proxy, local proxy, SSH server, PHP Xdebug adapter, deployment manager, and email/VPN integrations — all managed through a web dashboard on port 6001.

## Commands

### Backend (Go)

```bash
# Build
go build -o redock
make build           # runs critic + security + lint + tests, outputs to ./build/

# Run (requires root)
sudo ./redock
SKIP_UPDATE_CHECK=1 sudo ./redock   # skip version check during dev

# Service management
sudo redock --action install
sudo redock --action start
sudo redock --action stop
sudo redock --action uninstall

# Test
go test -v -timeout 30s -coverprofile=cover.out -cover ./...
go tool cover -func=cover.out

# Lint / static analysis
golangci-lint run ./...
gocritic check -enableAll ./...
gosec ./...

# Makefile shortcuts
make test       # runs critic, security, lint, tests
make lint
make critic
make security
```

### Frontend (Vue 3 in `/web/`)

```bash
cd web
npm install
npm run dev      # dev server at :5173 (proxies API calls to :6001)
npm run build    # outputs to web/dist/ (embedded into Go binary)
npm run lint     # ESLint with auto-fix
npm run format   # Prettier
```

## Architecture

### Entry Points & Startup Sequence

| File | Role |
|---|---|
| `service.go` | `main()` — permission check, flag parsing, systemd/Windows service wrapper |
| `init.go` | `initialize()` — in order: self-update check → Docker manager → in-memory DB → entity registration → all service `Init()` calls |
| `app.go` | `app()` — Fiber setup, middleware, route registration, SSH server goroutine, serve embedded Vue `dist/` on `/` |

Services are initialized in `init.go` in this order: `devenv` → `tunnel_server` → `localproxy` → `php_debug_adapter` → `saved_commands` → `deployment` → `api_gateway` → `dns_server` → `vpn_server` → `cloudflare` → `email_server`.

### Core Subsystems

**API Gateway** (`/api_gateway/`)  
HTTP/HTTPS reverse proxy. Priority-based route matching (host + path + method + headers). Per-route rate limiting, auth (JWT/Basic/API-key), health checks, TLS via Let's Encrypt, route cache (2048 entries, 30s TTL), client auto-blocking. Observability exporters to Loki, OTLP, InfluxDB, ClickHouse, Graylog. Listens on port 80/443.

**DNS Server** (`/dns_server/`)  
UDP/TCP on port 53, DoT on 853, DoH on 5053. Hosts/adblock/regex blocklists with per-client overrides. Query caching, 24-hour statistics, custom upstream per client.

**In-Memory Database** (`/platform/memory/`)  
Generic JSON-backed in-memory store. All application state (routes, DNS config, users, JWT secrets, etc.) lives here. Entities are registered with `memory.Register[T](db, "table_name")` in `init.go`. A periodic goroutine flushes dirty tables to `$DOCKER_WORK_DIR/data/`. Optional SQL backends (MySQL/PostgreSQL) exist but are secondary.

**Docker Manager** (`/docker-manager/`)  
Singleton via `GetDockerManager()`. Wraps Docker API for environment lifecycle, virtual host management, port assignment, and environment variable persistence.

**Tunnel Proxy** (`/tunnel_proxy/`, `/tunnel_server/`)  
Client manages domain list/renewal/account. Server daemon handles domain registration and unused domain cleanup.

**Local Proxy** (`/local_proxy/`)  
TCP/HTTP bridge, singleton via `GetLocalProxyManager()`. Persisted to JSON, auto-starts on startup.

### HTTP Layer

- **Framework**: Fiber v2 (port 6001)
- **Auth**: JWT in `Authorization: Bearer` header; token stored in browser `localStorage` as `redock_jwt`
- **Routes**: Defined in `/pkg/routes/`; middleware in `/pkg/middleware/`
- **Controllers**: `/app/controllers/` (~19 controllers)
- **WebSocket**: `/ws` endpoint for real-time stats and log streaming

### Frontend Structure

```
web/src/
├── views/          # Page components (HomeView, ApiGateway, DNSServer, Deployment, …)
├── components/     # Reusable components
├── services/
│   └── ApiService.js   # Axios wrapper; injects JWT, handles token refresh
├── stores/main.js  # Pinia global state
└── router/index.js # Vue Router
```

API base: `/api/v1/`. Standard response envelope: `{ data, message, code }`.

## Adding New Features

**New service module**: create package at root, implement `Init(dm *dockermanager.DockerEnvironmentManager)`, call it from `init.go`, register any DB entities before the call.

**New API endpoint**: add controller in `/app/controllers/`, define models in `/app/models/`, register route in `/pkg/routes/`, protect with `middleware.JWTProtected()` for private routes.

**New frontend page**: add Vue component in `/web/src/views/`, add route in `/web/src/router/index.js`, use `ApiService` for HTTP calls.

## Environment Variables

| Variable | Default | Description |
|---|---|---|
| `SKIP_UPDATE_CHECK` | — | Set to `1` to disable auto-update checks during development |
| `REDOCK_HOST` | `0.0.0.0` | Listen address |
| `REDOCK_PORT` | `6001` | Dashboard/API port |
| `SERVER_READ_TIMEOUT` | `60` | Request timeout in seconds |

# Redock DevStation

`Redock DevStation` is your all-in-one local development environment manager. It's a lightweight service application that brings together container management, development tools, and productivity features in a single, easy-to-use platform.

---

## Feature Overview

| Area | Highlights |
|------|------------|
| **API Gateway** | HTTP/HTTPS routing, priority-based matching, per-route auth, rate limiting, health checks, observability exporters, automatic TLS, and hot reloads |
| **Client Protection** | Real-time client tracking, auto-blocking based on configurable thresholds, manual block list with persistence, and top-client analytics |
| **Container & Dev Environments** | Docker lifecycle manager, service templates, environment bootstrap scripts, and redeploy helpers for consistent developer machines |
| **Networking Toolkit** | Local HTTP proxy, tunnel proxy, embedded SSH server, and secure remote access helpers for on-prem or cloud workloads |
| **Developer Productivity** | Saved commands, script launcher, PHP Xdebug adapter, local proxy controller, and WebSocket tooling to streamline daily workflows |
| **Operations & Automation** | Service install/start/stop/uninstall commands, self-update hooks, deployment helpers, and systemd-friendly logging |
| **Observability** | Built-in stats dashboard, telemetry exporters (Loki, InfluxDB, Graylog, OTLP, ClickHouse), and configurable batch/flush policies |
| **Modern Web UI** | Vue 3 + Tailwind dashboard with route/service editors, certificate manager, client insights, and modal-driven workflows |

---

## Screenshots

<div align="center">

### Dashboard Overview

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_56_48.png" alt="Dashboard Overview" width="800"/>

### API Gateway Management

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_57_02.png" alt="API Gateway Management" width="800"/>

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_57_09.png" alt="API Gateway Configuration" width="800"/>

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_57_22.png" alt="API Gateway Routes" width="800"/>

### Client Management & Security

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_58_10.png" alt="Client Management" width="800"/>

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_58_37.png" alt="Client Security Settings" width="800"/>

### Deployment & Container Management

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_59_09.png" alt="Deployment Management" width="800"/>

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_59_24.png" alt="Container Management" width="800"/>

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_59_38.png" alt="Container Settings" width="800"/>

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-21_59_57.png" alt="Container Configuration" width="800"/>

### Advanced Features

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-22_00_06.png" alt="Advanced Features" width="800"/>

<img src="../images/screencapture-192-168-36-240-6001-2026-01-21-22_00_17.png" alt="Feature Configuration" width="800"/>

</div>

---

## Detailed Capabilities

### API Gateway & Traffic Management
- HTTP and HTTPS listeners with automatic TLS cert loading plus optional Let's Encrypt provisioning and renewal scheduler.
- Route engine with priority sorting, host/path matching, method filtering, header rules, path stripping, and per-route observability toggles.
- Upstream service registry with health checks, retries, protocol/timeout controls, and aggregated service stats.
- Global and per-route rate limiters, JWT/basic/API-key authentication hooks, and reverse proxy logging (request/response bodies with truncation safeguards).
- Route cache, client-aware logging (X-Forwarded-* headers), and on-demand validation/testing helpers.

### Client Security & Telemetry
- Configurable client tracking (up to 1000 entries) with last-path/status metadata and consecutive miss counting.
- Auto-block policies based on unmatched-route thresholds and block duration controls, plus manual block entries persisted to disk.
- Block list import/export, JSON persistence, and UI-driven unblock workflows.
- Observability exporters for Loki, InfluxDB, Graylog, OTLP, and ClickHouse with batching/flush intervals and credential fields.
- Request log streaming to telemetry exporters with per-route override switches.

### Container & Environment Automation
- Docker environment manager (`docker-manager/`) handling config parsing, virtual host mapping, and lifecycle orchestration.
- Deployment/devenv helpers for initializing project templates, migrations, and stack-specific tooling.
- Saved command catalog plus command execution controller to keep frequently used scripts in one place.

### Networking, Proxying & Remote Access
- Local HTTP proxy service for routing dev traffic between containers and host.
- Tunnel proxy plus companion SSH server for exposing local services securely.
- PHP Xdebug adapter with Fiber-based controller for seamless debugging sessions from the dashboard.
- Support for manual and auto-managed tunnels via WebSocket controllers.

### Developer Productivity Suite
- Terminal-like saved command runner, credentials helpers, JWT utilities, and password generators inside `pkg/utils`.
- Auth/token management APIs with Fiber middleware for JWT verification and role-based guards.
- Web UI sections for services, routes, clients, certificates, observability, and saved commands with responsive layout.

### Operations & Service Management
- Cross-platform binaries (macOS, Linux, Windows) with `--action install|start|stop|uninstall` flags for service control.
- Self-update module for fetching the latest release metadata and applying updates.
- Logging utilities with structured output, rotating blocklists, and Go-based CLI for headless environments.

---

## Requirements
- A compatible platform:
  - **macOS**
  - **Linux**
  - **Windows**
- Administrator/root access for managing services.
- Optional: **Go** (if building from source).

---

## Download and Run
### For macOS

<details>
<summary>Apple Silicon</summary>

```bash
wget https://github.com/onuragtas/redock/releases/latest/download/redock_Darwin_arm64 -O /usr/local/bin/redock
chmod +x /usr/local/bin/redock
redock
```

</details>

<details>
<summary>AMD64</summary>

```bash
wget https://github.com/onuragtas/redock/releases/latest/download/redock_Darwin_amd64 -O /usr/local/bin/redock
chmod +x /usr/local/bin/redock
redock
```

</details>

---

### For Linux

<details>
<summary>Download and Run</summary>

Download the latest release:

```bash
wget https://github.com/onuragtas/redock/releases/latest/download/redock_Linux_amd64 -O /usr/local/bin/redock
chmod +x /usr/local/bin/redock
```

Run the application:

```bash
redock
```

</details>

---

## Service Management

The application supports the following service management actions:

| Action      | Description                 |
|-------------|-----------------------------|
| `install`   | Installs the service.       |
| `start`     | Starts the service.         |
| `stop`      | Stops the running service.  |
| `uninstall` | Removes the installed service. |

### Command Syntax
```bash
redock --action [install|start|stop|uninstall]
```

### Example Commands
- Install the service:
  ```bash
  redock --action install
  ```
- Start the service:
  ```bash
  redock --action start
  ```
- Stop the service:
  ```bash
  redock --action stop
  ```
- Uninstall the service:
  ```bash
  redock --action uninstall
  ```

---

## Building from Source
To build the application locally:
1. Clone the repository:
   ```bash
   git clone https://github.com/onuragtas/redock.git
   cd redock
   ```

2. Install dependencies and build the web UI:
   ```bash
   cd web
   npm install && npm run build
   ```

3. Build the binary:
   ```bash
   cd ..
   go build -o redock
   ```

4. Move the binary to a location in your `PATH`, such as `/usr/local/bin`:
   ```bash
   mv redock /usr/local/bin/
   ```

---

## Logging
Service logs are printed to the console by default. For advanced logging, redirect output to a file:
```bash
redock > redock.log 2>&1
```

---

## Troubleshooting
- Ensure the application has proper permissions (e.g., run with `sudo` on Linux/macOS).
- Check service status:
  - **Linux/macOS**: `systemctl status redock`
  - **Windows**: Use the Services manager.

---

## API Documentation

For detailed API documentation, see:
- [API Documentation](API_DOCS.md)
- [Swagger JSON](swagger.json)
- [Swagger YAML](swagger.yaml)

---

## License
This project is licensed under the MIT License. See the `LICENSE` file for more details.

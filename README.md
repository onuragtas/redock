
# Redock DevStation

`Redock DevStation` is your all-in-one local development environment manager. It's a lightweight service application that brings together container management, development tools, and productivity features in a single, easy-to-use platform.

---

## Features
- **Container Management**: Docker container lifecycle management with easy-to-use interface
- **Development Environment**: Setup and manage multiple development environments
- **Terminal Integration**: Built-in terminal with multi-tab support and saved commands
- **Local Proxy & Tunneling**: HTTP proxy and SSH tunneling capabilities
- **PHP XDebug Integration**: Seamless debugging setup for PHP development
- **Cross-Platform**: Supports macOS, Linux, and Windows platforms
- **Service Management**: Install, start, stop, and uninstall via command-line flags
- **Modern Web UI**: Beautiful, responsive interface built with Vue.js

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
   git clone <repository-url>
   cd <repository-folder>
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

## Demo
![Demo](https://user-images.githubusercontent.com/10091460/151700639-d8af1fff-d88b-4e33-a9ae-3b6a4622d5ec.mov)

---

## License
This project is licensed under the MIT License. See the `LICENSE` file for more details.


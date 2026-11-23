package ssh_server

// WebTerminalEnabled is the kill switch for the panel's web terminal: the
// /ws/:containerID WebSocket route and the 127.0.0.1:2222 SSH server behind it.
// That SSH server accepts any local connection without authentication and opens
// a root shell, so while the switch is off it is not started at all.
//
// Disabled on 2026-11-23: in the 2026-11-21 incident the web terminal was the
// attacker's root shell on the gateway. Use real SSH (key only) instead.
const WebTerminalEnabled = false

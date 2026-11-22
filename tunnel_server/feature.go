package tunnel_server

// FeatureEnabled is the kill switch for the whole tunnel feature (server daemon,
// client auto-start, agents and every /api/v1/tunnel/* endpoint).
//
// Disabled on 2026-11-22: the tunnel account credential leaked in the GitHub
// account compromise. Re-enable only after tunnel users, tokens and agent
// secrets have been rotated.
const FeatureEnabled = false

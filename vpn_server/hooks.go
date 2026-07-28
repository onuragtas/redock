package vpn_server

import "net"

// NATEvent describes a VPN server's network being (re)configured, so other
// modules (e.g. traffic_inspector) can layer additional NAT/redirect rules
// on top of the base MASQUERADE/forwarding setup without vpn_server needing
// to import or know about them.
type NATEvent struct {
	ServerID        uint
	TunInterface    string
	UplinkInterface string
	ServerNet       *net.IPNet
}

// OnNATConfigured, if set, is invoked after a VPN server's NAT/forwarding
// has been successfully configured (server start or restart).
var OnNATConfigured func(NATEvent)

// OnNATCleanup, if set, is invoked when a VPN server's NAT/forwarding rules
// are torn down (server stop), so dependent redirect rules can be removed
// too.
var OnNATCleanup func(serverID uint, tunInterface string)

// GetInstanceNetworkInfo returns the TUN and uplink interface names for a
// running server instance, for modules that need to build their own
// redirect rules alongside the base NAT setup.
func (m *WireGuardManager) GetInstanceNetworkInfo(serverID uint) (tunIface, uplinkIface string, ok bool) {
	m.mutex.RLock()
	defer m.mutex.RUnlock()

	instance, exists := m.instances[serverID]
	if !exists {
		return "", "", false
	}
	return instance.Server.Interface, instance.UplinkInterface, true
}

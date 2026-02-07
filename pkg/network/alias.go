package network

import (
	"fmt"
	"net"
	"runtime"
	"strings"

	"golang.org/x/sys/unix"
	"github.com/vishvananda/netlink"
)

const maxAliases = 512 // En fazla tek seferde eklenebilecek IP sayısı

// InterfaceInfo sunucudaki ağ arayüzü bilgisi.
type InterfaceInfo struct {
	Name    string   `json:"name"`
	Up      bool     `json:"up"`
	IPs     []string `json:"ips,omitempty"`
	MAC     string   `json:"mac,omitempty"`
	MTU     int      `json:"mtu,omitempty"`
	Gateway string   `json:"gateway,omitempty"` // Bu arayüzü kullanan varsayılan route'un gateway'i
}

// ListInterfaces sunucudaki ağ arayüzlerini listeler (net.Interfaces kullanır).
func ListInterfaces() ([]InterfaceInfo, error) {
	if runtime.GOOS != "linux" {
		return listInterfacesStd()
	}
	return listInterfacesNetlink()
}

func listInterfacesStd() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, err
	}
	out := make([]InterfaceInfo, 0, len(ifaces))
	for _, iface := range ifaces {
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		info := InterfaceInfo{
			Name: iface.Name,
			Up:   iface.Flags&net.FlagUp != 0,
			MTU:  iface.MTU,
			MAC:  iface.HardwareAddr.String(),
		}
		addrs, _ := iface.Addrs()
		for _, a := range addrs {
			if ipNet, ok := a.(*net.IPNet); ok && ipNet.IP.To4() != nil {
				info.IPs = append(info.IPs, ipNet.String())
			}
		}
		out = append(out, info)
	}
	return out, nil
}

func listInterfacesNetlink() ([]InterfaceInfo, error) {
	links, err := netlink.LinkList()
	if err != nil {
		return nil, err
	}
	defaultGwByLinkIndex := getDefaultGatewaysByLinkIndex()
	out := make([]InterfaceInfo, 0, len(links))
	for _, link := range links {
		attrs := link.Attrs()
		if attrs.Flags&net.FlagLoopback != 0 {
			continue
		}
		info := InterfaceInfo{
			Name:    attrs.Name,
			Up:      attrs.Flags&net.FlagUp != 0,
			MTU:     attrs.MTU,
			MAC:     attrs.HardwareAddr.String(),
			Gateway: defaultGwByLinkIndex[attrs.Index],
		}
		addrs, err := netlink.AddrList(link, unix.AF_INET)
		if err == nil {
			for _, a := range addrs {
				info.IPs = append(info.IPs, a.IPNet.String())
			}
		}
		out = append(out, info)
	}
	return out, nil
}

// getDefaultGatewaysByLinkIndex returns map[linkIndex]gatewayIP for default route (0.0.0.0/0) per link.
func getDefaultGatewaysByLinkIndex() map[int]string {
	out := make(map[int]string)
	routes, err := netlink.RouteList(nil, unix.AF_INET)
	if err != nil {
		return out
	}
	for _, r := range routes {
		if r.LinkIndex == 0 {
			continue
		}
		isDefault := false
		if r.Dst == nil {
			isDefault = true
		} else if r.Dst.IP != nil && r.Dst.IP.To4() != nil {
			ones, bits := r.Dst.Mask.Size()
			if bits == 32 && ones == 0 && r.Dst.IP.Equal(net.IPv4zero) {
				isDefault = true
			}
		}
		if isDefault && r.Gw != nil && r.Gw.To4() != nil {
			out[r.LinkIndex] = r.Gw.String()
		}
	}
	return out
}

// ParseIPRange CIDR veya "başlangıç-bitiş" string'inden /32 IP listesi üretir. IPv4.
func ParseIPRange(cidrOrRange string) ([]*net.IPNet, error) {
	cidrOrRange = strings.TrimSpace(cidrOrRange)
	if cidrOrRange == "" {
		return nil, fmt.Errorf("empty range")
	}

	// CIDR (örn. 88.255.136.0/24)
	if strings.Contains(cidrOrRange, "/") {
		_, ipNet, err := net.ParseCIDR(cidrOrRange)
		if err != nil {
			return nil, fmt.Errorf("invalid CIDR: %w", err)
		}
		return expandCIDRTo32(ipNet)
	}

	// Başlangıç-bitiş (örn. 88.255.136.1-88.255.136.254)
	if strings.Contains(cidrOrRange, "-") {
		parts := strings.SplitN(cidrOrRange, "-", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid range format (start-end expected)")
		}
		start := net.ParseIP(strings.TrimSpace(parts[0]))
		end := net.ParseIP(strings.TrimSpace(parts[1]))
		if start == nil || end == nil || start.To4() == nil || end.To4() == nil {
			return nil, fmt.Errorf("invalid IP address")
		}
		return rangeToIPNets(start.To4(), end.To4())
	}

	// Tek IP
	ip := net.ParseIP(cidrOrRange)
	if ip == nil || ip.To4() == nil {
		return nil, fmt.Errorf("invalid IP or range")
	}
	_, ipNet, _ := net.ParseCIDR(ip.To4().String() + "/32")
	return []*net.IPNet{ipNet}, nil
}

func expandCIDRTo32(ipNet *net.IPNet) ([]*net.IPNet, error) {
	ones, bits := ipNet.Mask.Size()
	if bits != 32 {
		return nil, fmt.Errorf("only IPv4 is supported")
	}
	count := 1 << (32 - ones)
	if count > maxAliases {
		return nil, fmt.Errorf("at most %d IPs can be added (requested %d)", maxAliases, count)
	}
	out := make([]*net.IPNet, 0, count)
	ip := make(net.IP, 4)
	copy(ip, ipNet.IP.To4())
	for i := 0; i < count; i++ {
		_, n, _ := net.ParseCIDR(ip.String() + "/32")
		out = append(out, n)
		inc(ip)
	}
	return out, nil
}

func inc(ip net.IP) {
	for j := len(ip) - 1; j >= 0; j-- {
		ip[j]++
		if ip[j] != 0 {
			break
		}
	}
}

func rangeToIPNets(start, end net.IP) ([]*net.IPNet, error) {
	if compareIP(start, end) > 0 {
		start, end = end, start
	}
	out := make([]*net.IPNet, 0)
	p := make(net.IP, 4)
	copy(p, start)
	for compareIP(p, end) <= 0 {
		if len(out) >= maxAliases {
			return nil, fmt.Errorf("at most %d IPs can be added", maxAliases)
		}
		_, n, _ := net.ParseCIDR(p.String() + "/32")
		out = append(out, n)
		inc(p)
	}
	return out, nil
}

func compareIP(a, b net.IP) int {
	for i := range a {
		if a[i] < b[i] {
			return -1
		}
		if a[i] > b[i] {
			return 1
		}
	}
	return 0
}

// AddAliases verilen arayüze IP adreslerini ekler (netlink). Sadece Linux.
func AddAliases(ifaceName string, ipNets []*net.IPNet) (added int, err error) {
	if runtime.GOOS != "linux" {
		return 0, fmt.Errorf("IP alias is only supported on Linux")
	}
	if len(ipNets) > maxAliases {
		return 0, fmt.Errorf("en fazla %d IP eklenebilir", maxAliases)
	}
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return 0, fmt.Errorf("interface not found: %w", err)
	}
	for _, ipNet := range ipNets {
		addr := &netlink.Addr{IPNet: ipNet}
		if err := netlink.AddrAdd(link, addr); err != nil {
			if strings.Contains(err.Error(), "exists") {
				continue
			}
			return added, fmt.Errorf("adding %s: %w", ipNet.String(), err)
		}
		added++
	}
	return added, nil
}

// RemoveAliases verilen arayüzden IP adreslerini kaldırır. Sadece Linux.
func RemoveAliases(ifaceName string, ipNets []*net.IPNet) (removed int, err error) {
	if runtime.GOOS != "linux" {
		return 0, fmt.Errorf("IP alias is only supported on Linux")
	}
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return 0, fmt.Errorf("interface not found: %w", err)
	}
	for _, ipNet := range ipNets {
		addr := &netlink.Addr{IPNet: ipNet}
		if err := netlink.AddrDel(link, addr); err != nil {
			if strings.Contains(err.Error(), "not found") || strings.Contains(err.Error(), "No such file") {
				continue
			}
			return removed, fmt.Errorf("removing %s: %w", ipNet.String(), err)
		}
		removed++
	}
	return removed, nil
}

// ListAddresses arayüzdeki IPv4 adreslerini döner.
func ListAddresses(ifaceName string) ([]string, error) {
	if runtime.GOOS != "linux" {
		return listAddressesStd(ifaceName)
	}
	link, err := netlink.LinkByName(ifaceName)
	if err != nil {
		return nil, err
	}
	addrs, err := netlink.AddrList(link, unix.AF_INET)
	if err != nil {
		return nil, err
	}
	out := make([]string, 0, len(addrs))
	for _, a := range addrs {
		out = append(out, a.IPNet.String())
	}
	return out, nil
}

func listAddressesStd(ifaceName string) ([]string, error) {
	iface, err := net.InterfaceByName(ifaceName)
	if err != nil {
		return nil, err
	}
	addrs, err := iface.Addrs()
	if err != nil {
		return nil, err
	}
	out := make([]string, 0)
	for _, a := range addrs {
		if ipNet, ok := a.(*net.IPNet); ok && ipNet.IP.To4() != nil {
			out = append(out, ipNet.String())
		}
	}
	return out, nil
}

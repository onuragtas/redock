//go:build darwin

package traffic_inspector

import (
	"bufio"
	"fmt"
	"net"
	"os/exec"
	"strconv"
	"strings"
)

// OriginalDestination recovers the true destination of a TCP connection
// that arrived via a pf `rdr-to` redirect.
//
// This is implemented by shelling out to `pfctl -s state` and parsing its
// text output, rather than calling the DIOCNATLOOK ioctl directly. A first
// attempt at the ioctl (with a hand-reconstructed `pfioc_natlook` struct,
// since macOS doesn't ship <net/pfvar.h> in its public SDK) failed against
// live traffic under both PF_IN and PF_OUT directions — almost certainly a
// struct layout mismatch against Apple's actual (undocumented) ABI, which
// can't be resolved without the real kernel header. Parsing `pfctl`'s own
// output sidesteps that entirely: Apple's pfctl binary already knows the
// correct struct layout, we just read the human-readable result it prints.
//
// Verified state line format on macOS (confirmed against live output):
//
//	ALL tcp 127.0.0.1:20007 <- 157.240.234.61:443 <- 10.0.0.2:50004       TIME_WAIT:TIME_WAIT
//	         ^^^^^^^^^^^^^^    ^^^^^^^^^^^^^^^^^^    ^^^^^^^^^^^^^^
//	         our local addr    ORIGINAL destination   client addr
//
// i.e. "<local> <- <original-destination> <- <client>" — read right to
// left, this is the packet's journey: from the client, originally destined
// for the middle address, rewritten by our rdr rule to our local address.
func OriginalDestination(conn net.Conn) (string, int, error) {
	localAddr, ok := conn.LocalAddr().(*net.TCPAddr)
	if !ok {
		return "", 0, fmt.Errorf("not a TCP connection")
	}
	remoteAddr, ok := conn.RemoteAddr().(*net.TCPAddr)
	if !ok {
		return "", 0, fmt.Errorf("not a TCP connection")
	}

	return pfStateLookup("tcp", remoteAddr.String(), localAddr.String())
}

// OriginalDestinationUDP is the UDP counterpart of OriginalDestination.
func OriginalDestinationUDP(remoteAddr *net.UDPAddr, localPort int) (string, int, error) {
	localAddrStr := net.JoinHostPort("127.0.0.1", strconv.Itoa(localPort))
	return pfStateLookup("udp", remoteAddr.String(), localAddrStr)
}

// pfStateLookup shells out to `pfctl -s state` and finds the state entry
// matching (proto, client address, our locally-redirected-to address),
// returning the real destination pf rewrote it from. redock already runs
// as root, so this needs no sudo/password prompt at runtime.
func pfStateLookup(proto, clientAddr, localAddr string) (string, int, error) {
	out, err := exec.Command("pfctl", "-s", "state").CombinedOutput()
	if err != nil {
		return "", 0, fmt.Errorf("pfctl -s state failed: %w (output: %s)", err, string(out))
	}

	scanner := bufio.NewScanner(strings.NewReader(string(out)))
	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		// Expected: ["ALL", "<proto>", "<local>", "<-", "<dest>", "<-", "<client>", "<state>"]
		if len(fields) < 7 {
			continue
		}
		if !strings.EqualFold(fields[1], proto) {
			continue
		}
		if fields[2] != localAddr || fields[6] != clientAddr {
			continue
		}

		host, portStr, err := net.SplitHostPort(fields[4])
		if err != nil {
			continue
		}
		port, err := strconv.Atoi(portStr)
		if err != nil {
			continue
		}
		return host, port, nil
	}

	if err := scanner.Err(); err != nil {
		return "", 0, fmt.Errorf("scan pfctl output: %w", err)
	}

	return "", 0, fmt.Errorf("no matching pf state found for %s %s (local %s)", proto, clientAddr, localAddr)
}

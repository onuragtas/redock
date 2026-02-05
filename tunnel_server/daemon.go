package tunnel_server

import (
	"bufio"
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	maxAuthLineLen       = 4096
	authOKReply          = "AUTH_OK\n"
	authFailReply        = "AUTH_FAILED\n"
	maxFramePayload      = 2 * 1024 * 1024 // 2MB
	frameTypeControl     = 0
	frameTypeData        = 1
	protocolTCP          = 0
	protocolUDP          = 1
	backendBufSize       = 32 * 1024
	udpBackendPortOffset  = 10000 // Gateway 0.0.0.0:Port UDP -> daemon 127.0.0.1:(Port+10000)
	tcpBackendPortOffset  = 20000 // Gateway 0.0.0.0:Port TCP -> daemon 127.0.0.1:(Port+20000)
	httpBackendPortOffset = 30000 // HTTP backend daemon 127.0.0.1:(Port+30000); böylece 0.0.0.0:Port gateway'e kalır (PORTS.md)
)

// Kontrol komutları (client -> server): "BIND <subdomain|full_domain>\n", "PING\n", "CLOSE_STREAM <id>\n"
// Server yanıtları: "BIND_OK\n", "BIND_FAILED <reason>\n", "PONG\n", "NEW_STREAM <id> tcp\n", "CLOSE_STREAM <id>\n"

// Client represents a tunnel client connected to the daemon (one TCP connection, validated by OAuth2 token).
type Client struct {
	UserID      uint
	Conn        net.Conn
	ConnectedAt time.Time
}

// streamKey identifies a single TCP stream (client + stream_id).
type streamKey struct {
	client   *Client
	streamID uint32
}

// stream holds the backend connection for a tunneled TCP stream.
type stream struct {
	backend net.Conn
}

// udpStream holds the remote addr for a tunneled UDP stream (gateway tarafı; cevap bu adrese yazılır).
type udpStream struct {
	clientAddr *net.UDPAddr
	port       int
}

var (
	daemonMu              sync.Mutex
	daemonListener        net.Listener
	clientsMu             sync.RWMutex
	clients               map[net.Conn]*Client
	boundDomainsMu        sync.RWMutex
	boundDomains          map[string]*Client // fullDomain -> client that receives traffic for this domain
	daemonRunning         bool
	backendListenersMu       sync.Mutex
	backendListeners         map[int]net.Listener // port -> TCP listener (HTTP backend)
	backendTCPListenersMu    sync.Mutex
	backendTCPListeners     map[int]net.Listener // internalTcpPort -> raw TCP listener
	backendUDPConnsMu        sync.Mutex
	backendUDPConns       map[int]*net.UDPConn // port -> UDP conn
	streamsMu             sync.RWMutex
	streams               map[streamKey]*stream
	udpStreamsMu          sync.RWMutex
	udpStreams            map[streamKey]*udpStream
	udpStreamByAddr       map[string]streamKey // "port:addr" -> streamKey (aynı clientAddr için stream_id tekrar kullanılır)
	clientNextStreamIDMu  sync.Mutex
	clientNextStreamID    map[*Client]uint32
)

func init() {
	clients = make(map[net.Conn]*Client)
	boundDomains = make(map[string]*Client)
	backendListeners = make(map[int]net.Listener)
	backendTCPListeners = make(map[int]net.Listener)
	backendUDPConns = make(map[int]*net.UDPConn)
	streams = make(map[streamKey]*stream)
	udpStreams = make(map[streamKey]*udpStream)
	udpStreamByAddr = make(map[string]streamKey)
	clientNextStreamID = make(map[*Client]uint32)
}

// StartDaemon starts the tunnel daemon listener on TunnelListenAddr if enabled.
// Plain TCP for now; TLS can be added later.
func StartDaemon() {
	daemonMu.Lock()
	defer daemonMu.Unlock()
	if daemonRunning {
		return
	}
	cfg := GetConfig()
	if !cfg.Enabled {
		return
	}
	addr := cfg.TunnelListenAddr
	if addr == "" {
		addr = ":8443"
	}
	listener, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("tunnel_server: daemon listen %s: %v", addr, err)
		return
	}
	daemonListener = listener
	daemonRunning = true
	log.Printf("tunnel_server: daemon listening on %s", addr)
	go acceptLoop()
	startAllBackendListeners()
}

// StopDaemon stops the tunnel daemon and closes all client connections.
func StopDaemon() {
	daemonMu.Lock()
	if !daemonRunning {
		daemonMu.Unlock()
		return
	}
	daemonRunning = false
	if daemonListener != nil {
		_ = daemonListener.Close()
		daemonListener = nil
	}
	daemonMu.Unlock()

	stopAllBackendListeners()
	stopAllBackendTCPListeners()
	stopAllBackendUDPListeners()
	closeAllStreams()
	closeAllUDPStreams()

	clientsMu.Lock()
	for _, c := range clients {
		_ = c.Conn.Close()
	}
	clients = make(map[net.Conn]*Client)
	clientsMu.Unlock()
	log.Printf("tunnel_server: daemon stopped")
}

// IsDaemonRunning returns whether the daemon listener is running.
func IsDaemonRunning() bool {
	daemonMu.Lock()
	defer daemonMu.Unlock()
	return daemonRunning
}

func acceptLoop() {
	for {
		conn, err := daemonListener.Accept()
		if err != nil {
			if !daemonRunning {
				return
			}
			log.Printf("tunnel_server: daemon accept: %v", err)
			continue
		}
		go handleConnection(conn)
	}
}

func handleConnection(conn net.Conn) {
	defer conn.Close()

	// Auth: first line = JWT access token
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReaderSize(conn, maxAuthLineLen)
	line, err := br.ReadString('\n')
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		log.Printf("tunnel_server: daemon auth read: %v", err)
		return
	}
	token := strings.TrimSpace(line)
	if token == "" {
		log.Printf("tunnel_server: auth in: empty token, reject")
		_, _ = conn.Write([]byte(authFailReply))
		return
	}
	log.Printf("tunnel_server: auth in: token len=%d", len(token))
	userID, err := ValidateTunnelToken(token)
	if err != nil {
		log.Printf("tunnel_server: daemon auth invalid token: %v", err)
		_, _ = conn.Write([]byte(authFailReply))
		return
	}
	if _, err := conn.Write([]byte(authOKReply)); err != nil {
		return
	}
	log.Printf("tunnel_server: auth out: AUTH_OK userID=%d", userID)

	client := &Client{
		UserID:      userID,
		Conn:        conn,
		ConnectedAt: time.Now(),
	}
	registerClient(conn, client)
	defer unregisterClient(conn)
	log.Printf("tunnel_server: client connected userID=%d from %s", userID, conn.RemoteAddr())

	// Keep connection alive; read loop (framing will be added in 3.2)
	serveClient(client, br)
}

func registerClient(conn net.Conn, c *Client) {
	clientsMu.Lock()
	defer clientsMu.Unlock()
	clients[conn] = c
}

func unregisterClient(conn net.Conn) {
	clientsMu.Lock()
	c, ok := clients[conn]
	if ok {
		delete(clients, conn)
		log.Printf("tunnel_server: client disconnected userID=%d", c.UserID)
	}
	clientsMu.Unlock()
	if ok {
		closeStreamsForClient(c)
		unbindAllDomainsForClient(c)
	}
}

func unbindAllDomainsForClient(c *Client) {
	boundDomainsMu.Lock()
	defer boundDomainsMu.Unlock()
	for fullDomain, bound := range boundDomains {
		if bound == c {
			delete(boundDomains, fullDomain)
			log.Printf("tunnel_server: unbound domain %s (client disconnected)", fullDomain)
		}
	}
}

// serveClient runs the read loop: length-prefixed frames, control (BIND/PING) and data (3.3).
func serveClient(c *Client, br *bufio.Reader) {
	for {
		payload, err := readFrame(br)
		if err != nil {
			if err != io.EOF {
				log.Printf("tunnel_server: client read frame userID=%d: %v", c.UserID, err)
			}
			return
		}
		if len(payload) < 1 {
			continue
		}
		typ := payload[0]
		body := payload[1:]
		switch typ {
		case frameTypeControl:
			cmd := strings.TrimSpace(string(body))
			if idx := strings.Index(cmd, " "); idx > 0 {
				cmd = cmd[:idx]
			}
			log.Printf("tunnel_server: in userID=%d type=control cmd=%s len=%d", c.UserID, cmd, len(body))
			handleControlMessage(c, body)
		case frameTypeData:
			if len(body) >= 5 {
				sid := binary.BigEndian.Uint32(body[0:4])
				proto := "tcp"
				if body[4] == protocolUDP {
					proto = "udp"
				}
				log.Printf("tunnel_server: in userID=%d type=data streamID=%d proto=%s len=%d", c.UserID, sid, proto, len(body)-5)
			}
			// 3.3: stream_id (4) + protocol (1) + data; server may forward to backend
			handleDataFrame(c, body)
		default:
			log.Printf("tunnel_server: unknown frame type %d", typ)
		}
	}
}

// readFrame reads one frame: 4 byte big-endian length + payload. Returns payload only.
func readFrame(br *bufio.Reader) ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(br, lenBuf[:]); err != nil {
		return nil, err
	}
	length := binary.BigEndian.Uint32(lenBuf[:])
	if length > maxFramePayload {
		return nil, io.ErrShortBuffer
	}
	if length == 0 {
		return nil, nil
	}
	payload := make([]byte, length)
	if _, err := io.ReadFull(br, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

// writeControlFrame sends a control message (e.g. BIND_OK, BIND_FAILED reason, PONG).
func writeControlFrame(conn net.Conn, msg string) error {
	msgTrim := strings.TrimSpace(msg)
	if len(msgTrim) > 60 {
		msgTrim = msgTrim[:60] + "..."
	}
	log.Printf("tunnel_server: out type=control msg=%q", msgTrim)
	payload := make([]byte, 1+len(msg))
	payload[0] = frameTypeControl
	copy(payload[1:], msg)
	return writeFrame(conn, payload)
}

// writeDataFrame sends a data frame (stream_id, protocol, payload) to client (3.3 backend -> client).
func writeDataFrame(conn net.Conn, streamID uint32, protocol byte, data []byte) error {
	proto := "tcp"
	if protocol == protocolUDP {
		proto = "udp"
	}
	log.Printf("tunnel_server: out type=data streamID=%d proto=%s len=%d", streamID, proto, len(data))
	payload := make([]byte, 1+4+1+len(data))
	payload[0] = frameTypeData
	binary.BigEndian.PutUint32(payload[1:5], streamID)
	payload[5] = protocol
	copy(payload[6:], data)
	return writeFrame(conn, payload)
}

func writeFrame(conn net.Conn, payload []byte) error {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := conn.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := conn.Write(payload)
	return err
}

func handleControlMessage(c *Client, body []byte) {
	text := string(body)
	parts := strings.SplitN(text, " ", 2)
	cmd := strings.ToUpper(strings.TrimSpace(parts[0]))
	arg := ""
	if len(parts) > 1 {
		arg = parts[1]
		// BIND arg is "domain\thost_rewrite"; TrimSpace would remove the tab so we'd lose "empty host_rewrite" case.
		if cmd != "BIND" {
			arg = strings.TrimSpace(arg)
		} else {
			arg = strings.TrimSuffix(arg, "\n")
			arg = strings.TrimSuffix(arg, "\r")
		}
	}
	switch cmd {
	case "BIND":
		handleBind(c, arg)
	case "PING":
		_ = writeControlFrame(c.Conn, "PONG\n")
	case "CLOSE_STREAM":
		handleCloseStream(c, arg)
	default:
		log.Printf("tunnel_server: unknown control command %q", cmd)
	}
}

func handleBind(c *Client, domainArg string) {
	if domainArg == "" {
		_ = writeControlFrame(c.Conn, "BIND_FAILED domain required\n")
		return
	}
	// Optional host_rewrite: "domain\thost_rewrite" (tab-separated). If no tab, only domain.
	domainPart := domainArg
	var hostRewrite string
	if idx := strings.Index(domainArg, "\t"); idx >= 0 {
		domainPart = strings.TrimSpace(domainArg[:idx])
		if idx+1 < len(domainArg) {
			hostRewrite = strings.TrimSpace(domainArg[idx+1:])
		}
	}
	if domainPart == "" {
		_ = writeControlFrame(c.Conn, "BIND_FAILED domain required\n")
		return
	}
	var d *TunnelDomain
	if strings.Contains(domainPart, ".") {
		d = FindDomainByFullDomain(domainPart)
	} else {
		d = FindDomainBySubdomain(domainPart)
	}
	if d == nil {
		_ = writeControlFrame(c.Conn, "BIND_FAILED domain not found\n")
		return
	}
	// Sadece domain sahibi bind edebilir; UserID 0 (admin oluşturdu) ise herhangi bir client bind edebilir
	if d.UserID != 0 && d.UserID != c.UserID {
		_ = writeControlFrame(c.Conn, "BIND_FAILED forbidden\n")
		return
	}
	boundDomainsMu.Lock()
	prev := boundDomains[d.FullDomain]
	boundDomains[d.FullDomain] = c
	boundDomainsMu.Unlock()
	if prev != nil && prev != c {
		log.Printf("tunnel_server: domain %s rebound, closing previous client userID=%d", d.FullDomain, prev.UserID)
		_ = prev.Conn.Close()
	}
	log.Printf("tunnel_server: domain %s bound to client userID=%d", d.FullDomain, c.UserID)
	now := time.Now()
	d.LastUsedAt = &now
	_ = UpdateDomain(d)
	// Update route HostRewrite when client sent it (tab present): empty = clear, non-empty = override
	if idx := strings.Index(domainArg, "\t"); idx >= 0 {
		if err := SetTunnelRouteHostRewrite(d, hostRewrite); err != nil {
			log.Printf("tunnel_server: SetTunnelRouteHostRewrite %s: %v", d.FullDomain, err)
		}
	}
	_ = writeControlFrame(c.Conn, "BIND_OK\n")
}

// handleDataFrame processes a data frame from client (3.3 TCP, 3.4 UDP). Forward to backend.
func handleDataFrame(c *Client, body []byte) {
	if len(body) < 5 {
		return
	}
	streamID := binary.BigEndian.Uint32(body[0:4])
	protocol := body[4]
	data := body[5:]
	key := streamKey{client: c, streamID: streamID}
	switch protocol {
	case protocolTCP:
		streamsMu.RLock()
		st, ok := streams[key]
		streamsMu.RUnlock()
		if !ok || st == nil {
			return
		}
		if _, err := st.backend.Write(data); err != nil {
			closeStream(key)
			_ = writeControlFrame(c.Conn, fmt.Sprintf("CLOSE_STREAM %d\n", streamID))
		}
	case protocolUDP:
		udpStreamsMu.RLock()
		us, ok := udpStreams[key]
		udpStreamsMu.RUnlock()
		if !ok || us == nil {
			return
		}
		backendUDPConnsMu.Lock()
		conn := backendUDPConns[us.port]
		backendUDPConnsMu.Unlock()
		if conn != nil {
			_, _ = conn.WriteToUDP(data, us.clientAddr)
		}
	default:
		return
	}
}

func handleCloseStream(c *Client, arg string) {
	idStr := strings.TrimSpace(arg)
	if idStr == "" {
		return
	}
	id, err := strconv.ParseUint(idStr, 10, 32)
	if err != nil {
		return
	}
	key := streamKey{client: c, streamID: uint32(id)}
	closeStream(key)
}

// GetClientByUserID returns the first connected client for the given tunnel user ID.
// Used later to bind a domain to a client (3.2).
func GetClientByUserID(userID uint) *Client {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	for _, c := range clients {
		if c.UserID == userID {
			return c
		}
	}
	return nil
}

// ListClients returns a snapshot of connected clients (for debugging/admin).
func ListClients() []*Client {
	clientsMu.RLock()
	defer clientsMu.RUnlock()
	out := make([]*Client, 0, len(clients))
	for _, c := range clients {
		out = append(out, c)
	}
	return out
}

// GetClientByDomain returns the client bound to the given full domain, or nil.
// Used by backend listener (3.3) to forward incoming traffic to the right client.
func GetClientByDomain(fullDomain string) *Client {
	boundDomainsMu.RLock()
	defer boundDomainsMu.RUnlock()
	return boundDomains[fullDomain]
}

// GetDomainByPort returns the tunnel domain for the given public port.
func GetDomainByPort(port int) *TunnelDomain {
	all := AllDomains()
	for _, d := range all {
		if d.Port == port {
			return d
		}
	}
	return nil
}

// internalHttpPort returns the port the daemon listens on for HTTP backend (gateway 80/443 -> 127.0.0.1:this).
func internalHttpPort(domainPort int) int {
	return domainPort + httpBackendPortOffset
}

// GetDomainByInternalHttpPort returns the domain for the given daemon HTTP backend port.
func GetDomainByInternalHttpPort(internalPort int) *TunnelDomain {
	all := AllDomains()
	for _, d := range all {
		if needHTTPForDomain(d) && internalHttpPort(d.Port) == internalPort {
			return d
		}
	}
	return nil
}

// needHTTPForDomain returns true if domain uses HTTP/HTTPS backend.
func needHTTPForDomain(d *TunnelDomain) bool {
	return d.Protocol == "http" || d.Protocol == "https" || d.Protocol == "all"
}

// internalUDPPort returns the port the daemon listens on for UDP backend (gateway forwards to this).
func internalUDPPort(domainPort int) int {
	return domainPort + udpBackendPortOffset
}

// GetDomainByInternalUDPPort returns the domain for the given daemon UDP backend port (Port+offset).
func GetDomainByInternalUDPPort(internalPort int) *TunnelDomain {
	all := AllDomains()
	for _, d := range all {
		if needUDPForDomain(d) && internalUDPPort(d.Port) == internalPort {
			return d
		}
	}
	return nil
}

// internalTcpPort returns the port the daemon listens on for raw TCP backend (gateway forwards to this).
func internalTcpPort(domainPort int) int {
	return domainPort + tcpBackendPortOffset
}

// GetDomainByInternalTCPPort returns the domain for the given daemon raw TCP backend port (Port+offset).
func GetDomainByInternalTCPPort(internalPort int) *TunnelDomain {
	all := AllDomains()
	for _, d := range all {
		if needTCPForDomain(d) && internalTcpPort(d.Port) == internalPort {
			return d
		}
	}
	return nil
}

func needTCPForDomain(d *TunnelDomain) bool {
	return d.Protocol == "tcp" || d.Protocol == "tcp+udp" || d.Protocol == "all"
}

// --- Backend TCP listeners (3.3: gateway proxies to 127.0.0.1:port, we accept and forward to client) ---

func startAllBackendListeners() {
	for _, d := range AllDomains() {
		if needHTTPForDomain(d) {
			StartBackendListener(d.Port)
		}
		if needTCPForDomain(d) {
			StartBackendTCPListener(internalTcpPort(d.Port))
		}
		if needUDPForDomain(d) {
			StartBackendUDPListener(internalUDPPort(d.Port))
		}
	}
}

func needUDPForDomain(d *TunnelDomain) bool {
	return d.Protocol == "udp" || d.Protocol == "tcp+udp" || d.Protocol == "all"
}

// StartBackendListener starts listening on 127.0.0.1:(port+30000) for HTTP backend so 0.0.0.0:port stays free for gateway TCP/UDP.
func StartBackendListener(port int) {
	backendListenersMu.Lock()
	defer backendListenersMu.Unlock()
	if _, ok := backendListeners[port]; ok {
		return
	}
	internalPort := internalHttpPort(port)
	addr := fmt.Sprintf("127.0.0.1:%d", internalPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("tunnel_server: backend listen %s: %v", addr, err)
		return
	}
	backendListeners[port] = ln
	log.Printf("tunnel_server: backend listening on %s (domain port %d)", addr, port)
	go backendAcceptLoop(internalPort, ln)
}

// StopBackendListener stops the backend listener for the given port. Call when domain is deleted.
func StopBackendListener(port int) {
	backendListenersMu.Lock()
	ln, ok := backendListeners[port]
	if ok {
		delete(backendListeners, port)
	}
	backendListenersMu.Unlock()
	if ok && ln != nil {
		_ = ln.Close()
		log.Printf("tunnel_server: backend stopped port %d", port)
	}
}

func stopAllBackendListeners() {
	backendListenersMu.Lock()
	list := make(map[int]net.Listener)
	for p, ln := range backendListeners {
		list[p] = ln
	}
	backendListeners = make(map[int]net.Listener)
	backendListenersMu.Unlock()
	for _, ln := range list {
		if ln != nil {
			_ = ln.Close()
		}
	}
}

func backendAcceptLoop(port int, ln net.Listener) {
	for {
		backendConn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleBackendConnection(port, backendConn)
	}
}

func handleBackendConnection(internalPort int, backendConn net.Conn) {
	defer backendConn.Close()
	d := GetDomainByInternalHttpPort(internalPort)
	if d == nil {
		return
	}
	handleBackendTCPStream(d, backendConn)
}

// handleBackendTCPStream finds client by domain and forwards backendConn bidirectionally to the tunnel client.
func handleBackendTCPStream(d *TunnelDomain, backendConn net.Conn) {
	log.Printf("tunnel_server: backend tcp new connection domain=%s from=%s", d.FullDomain, backendConn.RemoteAddr())
	client := GetClientByDomain(d.FullDomain)
	if client == nil {
		return
	}
	streamID := allocateStreamID(client)
	key := streamKey{client: client, streamID: streamID}
	streamsMu.Lock()
	streams[key] = &stream{backend: backendConn}
	streamsMu.Unlock()
	defer func() {
		closeStream(key)
		_ = writeControlFrame(client.Conn, fmt.Sprintf("CLOSE_STREAM %d\n", streamID))
	}()

	_ = writeControlFrame(client.Conn, fmt.Sprintf("NEW_STREAM %d tcp\n", streamID))

	buf := make([]byte, backendBufSize)
	for {
		n, err := backendConn.Read(buf)
		if err != nil {
			return
		}
		if n == 0 {
			continue
		}
		if err := writeDataFrame(client.Conn, streamID, protocolTCP, buf[:n]); err != nil {
			return
		}
	}
}

// StartBackendTCPListener starts listening on 127.0.0.1:internalPort for raw TCP tunnel backend (gateway TCPRoute forwards here).
func StartBackendTCPListener(internalPort int) {
	backendTCPListenersMu.Lock()
	defer backendTCPListenersMu.Unlock()
	if _, ok := backendTCPListeners[internalPort]; ok {
		return
	}
	addr := fmt.Sprintf("127.0.0.1:%d", internalPort)
	ln, err := net.Listen("tcp", addr)
	if err != nil {
		log.Printf("tunnel_server: backend TCP listen %s: %v", addr, err)
		return
	}
	backendTCPListeners[internalPort] = ln
	log.Printf("tunnel_server: backend TCP listening on %s", addr)
	go backendTCPAcceptLoop(internalPort, ln)
}

// StopBackendTCPListener stops the raw TCP backend listener for the given internal port.
func StopBackendTCPListener(internalPort int) {
	backendTCPListenersMu.Lock()
	ln, ok := backendTCPListeners[internalPort]
	if ok {
		delete(backendTCPListeners, internalPort)
	}
	backendTCPListenersMu.Unlock()
	if ok && ln != nil {
		_ = ln.Close()
		log.Printf("tunnel_server: backend TCP stopped port %d", internalPort)
	}
}

func backendTCPAcceptLoop(internalPort int, ln net.Listener) {
	for {
		backendConn, err := ln.Accept()
		if err != nil {
			return
		}
		go handleBackendTCPConnection(internalPort, backendConn)
	}
}

func handleBackendTCPConnection(internalPort int, backendConn net.Conn) {
	defer backendConn.Close()
	d := GetDomainByInternalTCPPort(internalPort)
	if d == nil {
		return
	}
	handleBackendTCPStream(d, backendConn)
}

func stopAllBackendTCPListeners() {
	backendTCPListenersMu.Lock()
	list := make(map[int]net.Listener)
	for p, ln := range backendTCPListeners {
		list[p] = ln
	}
	backendTCPListeners = make(map[int]net.Listener)
	backendTCPListenersMu.Unlock()
	for _, ln := range list {
		if ln != nil {
			_ = ln.Close()
		}
	}
}

func allocateStreamID(c *Client) uint32 {
	clientNextStreamIDMu.Lock()
	defer clientNextStreamIDMu.Unlock()
	id := clientNextStreamID[c]
	clientNextStreamID[c] = id + 1
	return id
}

func closeStream(key streamKey) {
	streamsMu.Lock()
	st, ok := streams[key]
	if ok {
		delete(streams, key)
		if st.backend != nil {
			_ = st.backend.Close()
		}
	}
	streamsMu.Unlock()
}

func closeStreamsForClient(c *Client) {
	streamsMu.Lock()
	var toClose []streamKey
	for k := range streams {
		if k.client == c {
			toClose = append(toClose, k)
		}
	}
	for _, k := range toClose {
		if st := streams[k]; st != nil && st.backend != nil {
			_ = st.backend.Close()
		}
		delete(streams, k)
	}
	streamsMu.Unlock()
	udpStreamsMu.Lock()
	for k := range udpStreams {
		if k.client == c {
			delete(udpStreams, k)
		}
	}
	for addr, k := range udpStreamByAddr {
		if k.client == c {
			delete(udpStreamByAddr, addr)
		}
	}
	udpStreamsMu.Unlock()
	clientNextStreamIDMu.Lock()
	delete(clientNextStreamID, c)
	clientNextStreamIDMu.Unlock()
}

func closeAllStreams() {
	streamsMu.Lock()
	for _, st := range streams {
		if st != nil && st.backend != nil {
			_ = st.backend.Close()
		}
	}
	streams = make(map[streamKey]*stream)
	streamsMu.Unlock()
	closeAllUDPStreams()
	clientNextStreamIDMu.Lock()
	clientNextStreamID = make(map[*Client]uint32)
	clientNextStreamIDMu.Unlock()
}

func closeAllUDPStreams() {
	udpStreamsMu.Lock()
	udpStreams = make(map[streamKey]*udpStream)
	udpStreamByAddr = make(map[string]streamKey)
	udpStreamsMu.Unlock()
}

// --- Backend UDP listeners (3.4: gateway UDP route -> 127.0.0.1:port, we accept and forward to client) ---

const udpBackendBufSize = 64 * 1024

// StartBackendUDPListener starts listening on UDP 127.0.0.1:port for tunnel backend (gateway forwards UDP here).
func StartBackendUDPListener(port int) {
	backendUDPConnsMu.Lock()
	defer backendUDPConnsMu.Unlock()
	if _, ok := backendUDPConns[port]; ok {
		return
	}
	addr := &net.UDPAddr{IP: net.IPv4(127, 0, 0, 1), Port: port}
	conn, err := net.ListenUDP("udp", addr)
	if err != nil {
		log.Printf("tunnel_server: backend UDP listen %s: %v", addr, err)
		return
	}
	backendUDPConns[port] = conn
	log.Printf("tunnel_server: backend UDP listening on %s", addr)
	go backendUDPReadLoop(port, conn)
}

// StopBackendUDPListener stops the UDP backend for the given port.
func StopBackendUDPListener(port int) {
	backendUDPConnsMu.Lock()
	conn, ok := backendUDPConns[port]
	if ok {
		delete(backendUDPConns, port)
	}
	backendUDPConnsMu.Unlock()
	if ok && conn != nil {
		_ = conn.Close()
		log.Printf("tunnel_server: backend UDP stopped port %d", port)
	}
}

func stopAllBackendUDPListeners() {
	backendUDPConnsMu.Lock()
	list := make(map[int]*net.UDPConn)
	for p, c := range backendUDPConns {
		list[p] = c
	}
	backendUDPConns = make(map[int]*net.UDPConn)
	backendUDPConnsMu.Unlock()
	for _, c := range list {
		if c != nil {
			_ = c.Close()
		}
	}
}

func backendUDPReadLoop(port int, conn *net.UDPConn) {
	buf := make([]byte, udpBackendBufSize)
	for {
		n, clientAddr, err := conn.ReadFromUDP(buf)
		if err != nil {
			return
		}
		if n == 0 {
			continue
		}
		packet := make([]byte, n)
		copy(packet, buf[:n])
		go handleBackendUDPPacket(port, clientAddr, packet)
	}
}

func handleBackendUDPPacket(internalPort int, clientAddr *net.UDPAddr, packet []byte) {
	log.Printf("tunnel_server: backend udp in port=%d from=%s len=%d", internalPort, clientAddr.String(), len(packet))
	d := GetDomainByInternalUDPPort(internalPort)
	if d == nil {
		return
	}
	client := GetClientByDomain(d.FullDomain)
	if client == nil {
		return
	}
	addrKey := fmt.Sprintf("%d:%s", internalPort, clientAddr.String())
	udpStreamsMu.Lock()
	sk, exists := udpStreamByAddr[addrKey]
	if !exists {
		streamID := allocateStreamID(client)
		sk = streamKey{client: client, streamID: streamID}
		udpStreams[sk] = &udpStream{clientAddr: clientAddr, port: internalPort}
		udpStreamByAddr[addrKey] = sk
	}
	udpStreamsMu.Unlock()
	if err := writeDataFrame(client.Conn, sk.streamID, protocolUDP, packet); err != nil {
		return
	}
}

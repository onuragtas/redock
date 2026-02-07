package client

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
	maxAuthLineLen  = 4096
	authOKReply     = "AUTH_OK\n"
	authFailReply   = "AUTH_FAILED\n"
	maxFramePayload = 2 * 1024 * 1024 // 2MB
	frameTypeControl = 0
	frameTypeData    = 1
	protocolTCP      = 0
	protocolUDP      = 1
	backendBufSize   = 32 * 1024
)

// Config holds tunnel client configuration.
type Config struct {
	// ServerAddr is the daemon address (e.g. "host:8443").
	ServerAddr string
	// Token is the JWT access token for auth.
	Token string
	// Domain is subdomain or full domain to bind (e.g. "myapp" or "myapp.tnpx.org").
	Domain string
	// LocalHttpAddr is the destination for HTTP/HTTPS tunneled traffic (e.g. "127.0.0.1:8080"). Empty to disable or fall back to LocalTCPAddr.
	LocalHttpAddr string
	// LocalTCPAddr is the destination for raw TCP tunneled traffic (e.g. "127.0.0.1:9000"). Empty to disable TCP.
	LocalTCPAddr string
	// LocalUDPAddr is the destination address for UDP forwarding (e.g. "127.0.0.1:53"). Empty to disable UDP.
	LocalUDPAddr string
	// SourceBindIP is the local IP to bind when connecting to LocalTCPAddr/LocalUDPAddr (outbound source). Empty = system default.
	SourceBindIP string
	// HostRewrite is sent to the daemon on BIND to set/clear the route's Host header override (HTTP/HTTPS only). Empty clears.
	HostRewrite string
}

// Client is a tunnel client connected to the daemon.
type Client struct {
	cfg    Config
	conn   net.Conn
	br     *bufio.Reader
	closed chan struct{}
	once   sync.Once
	// tcpStreams: streamID -> local backend connection
	tcpStreams   map[uint32]net.Conn
	tcpStreamsMu sync.RWMutex
	// udpSockets: streamID -> local UDP conn used to talk to LocalUDPAddr (so we know which stream_id a reply belongs to)
	udpSockets   map[uint32]*net.UDPConn
	udpSocketsMu sync.RWMutex
}

// ConnectOnce connects to the daemon, performs auth and BIND, and returns the client.
// The caller should call Run() (e.g. in a goroutine) and Close() when done.
func ConnectOnce(cfg Config) (*Client, error) {
	if cfg.ServerAddr == "" || cfg.Token == "" || cfg.Domain == "" {
		return nil, fmt.Errorf("client: ServerAddr, Token and Domain are required")
	}
	conn, err := net.DialTimeout("tcp", cfg.ServerAddr, 15*time.Second)
	if err != nil {
		return nil, fmt.Errorf("client: dial: %w", err)
	}
	conn.SetReadDeadline(time.Now().Add(30 * time.Second))
	br := bufio.NewReaderSize(conn, maxAuthLineLen)
	log.Printf("tunnel_client: out auth token len=%d", len(cfg.Token))
	if _, err := conn.Write([]byte(cfg.Token + "\n")); err != nil {
		conn.Close()
		return nil, fmt.Errorf("client: write token: %w", err)
	}
	line, err := br.ReadString('\n')
	conn.SetReadDeadline(time.Time{})
	if err != nil {
		conn.Close()
		return nil, fmt.Errorf("client: read auth reply: %w", err)
	}
	line = strings.TrimSpace(line)
	log.Printf("tunnel_client: in auth reply=%q", line)
	if line != "AUTH_OK" {
		conn.Close()
		return nil, fmt.Errorf("client: auth failed: %s", line)
	}
	c := &Client{
		cfg:        cfg,
		conn:       conn,
		br:         br,
		closed:     make(chan struct{}),
		tcpStreams: make(map[uint32]net.Conn),
		udpSockets: make(map[uint32]*net.UDPConn),
	}
	// Always send host_rewrite (tab-separated) so server can set or clear the route's Host override
	if err := c.sendControl("BIND " + cfg.Domain + "\t" + cfg.HostRewrite + "\n"); err != nil {
		c.Close()
		return nil, fmt.Errorf("client: send BIND: %w", err)
	}
	ctrl, err := c.readControl()
	if err != nil {
		c.Close()
		return nil, fmt.Errorf("client: read BIND reply: %w", err)
	}
	log.Printf("tunnel_client: in control %s", strings.TrimSpace(ctrl))
	if !strings.HasPrefix(ctrl, "BIND_OK") {
		c.Close()
		return nil, fmt.Errorf("client: bind failed: %s", ctrl)
	}
	log.Printf("tunnel_client: bound domain %s", cfg.Domain)
	return c, nil
}

// Connect connects to the daemon, performs auth and BIND, then runs the frame loop.
// It blocks until the connection is closed or an error occurs.
func Connect(cfg Config) error {
	cl, err := ConnectOnce(cfg)
	if err != nil {
		return err
	}
	return cl.Run()
}

// Run runs the frame loop. It blocks until the connection is closed or an error occurs.
func (c *Client) Run() error {
	return c.run()
}

func (c *Client) run() error {
	for {
		payload, err := c.readFrame()
		if err != nil {
			if err != io.EOF {
				log.Printf("tunnel_client: read frame: %v", err)
			}
			return err
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
			log.Printf("tunnel_client: in type=control cmd=%s len=%d", cmd, len(body))
			c.handleControl(body)
		case frameTypeData:
			if len(body) >= 5 {
				sid := binary.BigEndian.Uint32(body[0:4])
				proto := "tcp"
				if body[4] == protocolUDP {
					proto = "udp"
				}
				log.Printf("tunnel_client: in type=data streamID=%d proto=%s len=%d", sid, proto, len(body)-5)
			}
			c.handleDataFrame(body)
		default:
			log.Printf("tunnel_client: unknown frame type %d", typ)
		}
	}
}

func (c *Client) readFrame() ([]byte, error) {
	var lenBuf [4]byte
	if _, err := io.ReadFull(c.br, lenBuf[:]); err != nil {
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
	if _, err := io.ReadFull(c.br, payload); err != nil {
		return nil, err
	}
	return payload, nil
}

func (c *Client) readControl() (string, error) {
	payload, err := c.readFrame()
	if err != nil {
		return "", err
	}
	if len(payload) < 1 || payload[0] != frameTypeControl {
		return "", fmt.Errorf("expected control frame")
	}
	return strings.TrimSpace(string(payload[1:])), nil
}

func (c *Client) sendControl(msg string) error {
	msgTrim := strings.TrimSpace(msg)
	if len(msgTrim) > 50 {
		msgTrim = msgTrim[:50] + "..."
	}
	log.Printf("tunnel_client: out type=control msg=%q", msgTrim)
	payload := make([]byte, 1+len(msg))
	payload[0] = frameTypeControl
	copy(payload[1:], msg)
	return c.writeFrame(payload)
}

func (c *Client) writeFrame(payload []byte) error {
	var lenBuf [4]byte
	binary.BigEndian.PutUint32(lenBuf[:], uint32(len(payload)))
	if _, err := c.conn.Write(lenBuf[:]); err != nil {
		return err
	}
	_, err := c.conn.Write(payload)
	return err
}

func (c *Client) writeDataFrame(streamID uint32, protocol byte, data []byte) error {
	proto := "tcp"
	if protocol == protocolUDP {
		proto = "udp"
	}
	log.Printf("tunnel_client: out type=data streamID=%d proto=%s len=%d", streamID, proto, len(data))
	payload := make([]byte, 1+4+1+len(data))
	payload[0] = frameTypeData
	binary.BigEndian.PutUint32(payload[1:5], streamID)
	payload[5] = protocol
	copy(payload[6:], data)
	return c.writeFrame(payload)
}

func (c *Client) handleControl(body []byte) {
	text := strings.TrimSpace(string(body))
	parts := strings.SplitN(text, " ", 3)
	cmd := strings.ToUpper(strings.TrimSpace(parts[0]))
	switch cmd {
	case "BIND_OK":
		// already handled in Connect
	case "BIND_FAILED":
		log.Printf("tunnel_client: %s", text)
	case "PONG":
		// keepalive reply
	case "NEW_STREAM":
		if len(parts) < 3 {
			log.Printf("tunnel_client: NEW_STREAM missing parts (got %d)", len(parts))
			return
		}
		idStr := strings.TrimSpace(parts[1])
		proto := strings.ToLower(strings.TrimSpace(strings.TrimRight(parts[2], "\r\n")))
		if proto != "tcp" && proto != "http" {
			log.Printf("tunnel_client: NEW_STREAM unknown proto %q", proto)
			return
		}
		streamID, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			return
		}
		c.handleNewStream(uint32(streamID), proto)
	case "CLOSE_STREAM":
		if len(parts) < 2 {
			return
		}
		idStr := strings.TrimSpace(parts[1])
		streamID, err := strconv.ParseUint(idStr, 10, 32)
		if err != nil {
			return
		}
		c.closeStream(uint32(streamID))
	default:
		log.Printf("tunnel_client: unknown control %q", cmd)
	}
}

func (c *Client) handleNewStream(streamID uint32, streamType string) {
	var addr string
	switch streamType {
	case "http":
		addr = c.cfg.LocalHttpAddr
		if addr == "" {
			addr = c.cfg.LocalTCPAddr // fallback for backward compat
		}
	case "tcp":
		addr = c.cfg.LocalTCPAddr
	default:
		addr = c.cfg.LocalTCPAddr
	}
	if addr == "" {
		log.Printf("tunnel_client: NEW_STREAM %d %s no backend address (LocalHttp=%q LocalTCP=%q)", streamID, streamType, c.cfg.LocalHttpAddr, c.cfg.LocalTCPAddr)
		_ = c.sendControl(fmt.Sprintf("CLOSE_STREAM %d\n", streamID))
		return
	}
	log.Printf("tunnel_client: NEW_STREAM %d %s -> dial %s", streamID, streamType, addr)
	dialer := &net.Dialer{Timeout: 10 * time.Second}
	if c.cfg.SourceBindIP != "" {
		dialer.LocalAddr = &net.TCPAddr{IP: net.ParseIP(c.cfg.SourceBindIP)}
	}
	backend, err := dialer.Dial("tcp", addr)
	if err != nil {
		log.Printf("tunnel_client: dial %s %s (source %s): %v", streamType, addr, c.cfg.SourceBindIP, err)
		_ = c.sendControl(fmt.Sprintf("CLOSE_STREAM %d\n", streamID))
		return
	}
	c.tcpStreamsMu.Lock()
	c.tcpStreams[streamID] = backend
	c.tcpStreamsMu.Unlock()
	go c.tcpBackendToTunnel(streamID, backend)
}

func (c *Client) tcpBackendToTunnel(streamID uint32, backend net.Conn) {
	defer func() {
		backend.Close()
		c.closeStream(streamID)
	}()
	buf := make([]byte, backendBufSize)
	for {
		n, err := backend.Read(buf)
		if err != nil {
			return
		}
		if n == 0 {
			continue
		}
		if err := c.writeDataFrame(streamID, protocolTCP, buf[:n]); err != nil {
			return
		}
	}
}

func (c *Client) handleDataFrame(body []byte) {
	if len(body) < 5 {
		return
	}
	streamID := binary.BigEndian.Uint32(body[0:4])
	protocol := body[4]
	data := body[5:]
	switch protocol {
	case protocolTCP:
		c.tcpStreamsMu.RLock()
		backend := c.tcpStreams[streamID]
		c.tcpStreamsMu.RUnlock()
		if backend != nil {
			if _, err := backend.Write(data); err != nil {
				c.closeStream(streamID)
			}
		}
	case protocolUDP:
		c.forwardUDPToLocal(streamID, data)
	default:
		// ignore
	}
}

// forwardUDPToLocal sends data to local UDP. We use one UDP socket per stream_id so that
// when the local service replies, we know which stream_id to use for the reply.
func (c *Client) forwardUDPToLocal(streamID uint32, data []byte) {
	if c.cfg.LocalUDPAddr == "" {
		return
	}
	raddr, err := net.ResolveUDPAddr("udp", c.cfg.LocalUDPAddr)
	if err != nil {
		log.Printf("tunnel_client: resolve %s: %v", c.cfg.LocalUDPAddr, err)
		return
	}
	c.udpSocketsMu.Lock()
	uc, ok := c.udpSockets[streamID]
	if !ok {
		var localAddr *net.UDPAddr
		if c.cfg.SourceBindIP != "" {
			localAddr = &net.UDPAddr{IP: net.ParseIP(c.cfg.SourceBindIP), Port: 0}
		}
		uc, err = net.ListenUDP("udp", localAddr)
		if err != nil {
			c.udpSocketsMu.Unlock()
			log.Printf("tunnel_client: listen udp (source %s): %v", c.cfg.SourceBindIP, err)
			return
		}
		c.udpSockets[streamID] = uc
		go c.udpLocalToTunnel(streamID, uc)
	}
	uc = c.udpSockets[streamID]
	c.udpSocketsMu.Unlock()
	if uc != nil {
		_, _ = uc.WriteToUDP(data, raddr)
	}
}

// udpLocalToTunnel reads replies from the local UDP socket and sends them back to the daemon.
func (c *Client) udpLocalToTunnel(streamID uint32, uc *net.UDPConn) {
	buf := make([]byte, 64*1024)
	for {
		n, _, err := uc.ReadFromUDP(buf)
		if err != nil {
			c.udpSocketsMu.Lock()
			if c.udpSockets[streamID] == uc {
				delete(c.udpSockets, streamID)
			}
			c.udpSocketsMu.Unlock()
			return
		}
		if n == 0 {
			continue
		}
		payload := make([]byte, n)
		copy(payload, buf[:n])
		if err := c.writeDataFrame(streamID, protocolUDP, payload); err != nil {
			return
		}
	}
}

func (c *Client) closeStream(streamID uint32) {
	// TCP
	c.tcpStreamsMu.Lock()
	if backend := c.tcpStreams[streamID]; backend != nil {
		backend.Close()
		delete(c.tcpStreams, streamID)
	}
	c.tcpStreamsMu.Unlock()
	// UDP
	c.udpSocketsMu.Lock()
	if uc := c.udpSockets[streamID]; uc != nil {
		uc.Close()
		delete(c.udpSockets, streamID)
	}
	c.udpSocketsMu.Unlock()
	_ = c.sendControl(fmt.Sprintf("CLOSE_STREAM %d\n", streamID))
}

// Close closes the client connection.
func (c *Client) Close() error {
	var err error
	c.once.Do(func() {
		c.tcpStreamsMu.Lock()
		for _, backend := range c.tcpStreams {
			backend.Close()
		}
		c.tcpStreams = make(map[uint32]net.Conn)
		c.tcpStreamsMu.Unlock()
		c.udpSocketsMu.Lock()
		for _, uc := range c.udpSockets {
			uc.Close()
		}
		c.udpSockets = make(map[uint32]*net.UDPConn)
		c.udpSocketsMu.Unlock()
		if c.conn != nil {
			err = c.conn.Close()
		}
		close(c.closed)
	})
	return err
}

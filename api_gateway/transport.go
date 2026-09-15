package api_gateway

import (
	"net"
	"net/http"
	"time"
)

const (
	upstreamMaxIdleConns        = 100
	upstreamMaxIdleConnsPerHost = 64
	// HTTP/2 keepalive pings detect dead upstream connections that would
	// otherwise hang long-lived gRPC streams.
	upstreamH2SendPingTimeout = 30 * time.Second
	upstreamH2PingTimeout     = 15 * time.Second
)

// upstreamTransport returns the shared transport for a service protocol so
// upstream connections are pooled across requests. gRPC traffic always goes
// over HTTP/2 (h2c for cleartext, h2 for TLS), even when the service is
// declared as plain http/https, because gRPC cannot run over HTTP/1.1.
func (g *Gateway) upstreamTransport(protocol string, grpcReq bool) *http.Transport {
	key := upstreamScheme(protocol)
	if grpcReq || isGRPCProtocol(protocol) {
		if key == "https" {
			key = ProtocolGRPCS
		} else {
			key = ProtocolGRPC
		}
	}

	g.transportMu.Lock()
	defer g.transportMu.Unlock()
	if t := g.transports[key]; t != nil {
		return t
	}

	to := g.resolveTimeouts()
	t := &http.Transport{
		DialContext: (&net.Dialer{
			Timeout:   to.upstreamDial,
			KeepAlive: to.upstreamKeepAlive,
		}).DialContext,
		MaxIdleConns:          upstreamMaxIdleConns,
		MaxIdleConnsPerHost:   upstreamMaxIdleConnsPerHost,
		IdleConnTimeout:       to.upstreamIdleConn,
		TLSHandshakeTimeout:   to.tlsHandshake,
		ExpectContinueTimeout: to.expectContinue,
	}
	switch key {
	case ProtocolGRPC:
		var p http.Protocols
		p.SetUnencryptedHTTP2(true)
		t.Protocols = &p
		t.HTTP2 = upstreamHTTP2Config()
	case ProtocolGRPCS:
		var p http.Protocols
		p.SetHTTP2(true)
		t.Protocols = &p
		t.HTTP2 = upstreamHTTP2Config()
	}

	if g.transports == nil {
		g.transports = make(map[string]*http.Transport)
	}
	g.transports[key] = t
	return t
}

func upstreamHTTP2Config() *http.HTTP2Config {
	return &http.HTTP2Config{
		SendPingTimeout: upstreamH2SendPingTimeout,
		PingTimeout:     upstreamH2PingTimeout,
	}
}

// resetTransports drops the pooled transports so the next request rebuilds
// them with the current timeout settings. In-flight requests keep using the
// old transport; only its idle connections are closed.
func (g *Gateway) resetTransports() {
	g.transportMu.Lock()
	defer g.transportMu.Unlock()
	for _, t := range g.transports {
		t.CloseIdleConnections()
	}
	g.transports = nil
}

// routeRateLimiter returns the persistent limiter for a route, recreating it
// when the route's limit settings change.
func (g *Gateway) routeRateLimiter(route *Route) *rateLimiter {
	window := time.Duration(route.RateLimitWindow) * time.Second

	g.routeLimiterMu.Lock()
	defer g.routeLimiterMu.Unlock()
	if l := g.routeLimiters[route.ID]; l != nil && l.requests == route.RateLimitRequests && l.window == window {
		return l
	}
	if g.routeLimiters == nil {
		g.routeLimiters = make(map[string]*rateLimiter)
	}
	l := &rateLimiter{
		clients:  make(map[string]*clientRateLimit),
		requests: route.RateLimitRequests,
		window:   window,
	}
	g.routeLimiters[route.ID] = l
	return l
}

func (g *Gateway) resetRouteRateLimiters() {
	g.routeLimiterMu.Lock()
	defer g.routeLimiterMu.Unlock()
	g.routeLimiters = nil
}

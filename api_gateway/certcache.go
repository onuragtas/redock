package api_gateway

import (
	"crypto/tls"
	"os"
	"sync"
	"time"
)

// certCache keeps the parsed TLS certificate in memory. GetCertificate runs
// on every TLS handshake; reading and parsing the PEM files each time costs
// disk I/O plus key parsing per new connection. The files are re-read only
// when their path or modification time changes (e.g. after a renewal), which
// a cheap stat — throttled to once per second — detects.
type certCache struct {
	mu        sync.Mutex
	certFile  string
	keyFile   string
	certMod   time.Time
	keyMod    time.Time
	checkedAt time.Time
	cert      *tls.Certificate
}

const certRecheckInterval = time.Second

func (c *certCache) get(certFile, keyFile string) (*tls.Certificate, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	now := time.Now()
	samePaths := c.cert != nil && c.certFile == certFile && c.keyFile == keyFile
	if samePaths && now.Sub(c.checkedAt) < certRecheckInterval {
		return c.cert, nil
	}

	certInfo, err := os.Stat(certFile)
	if err != nil {
		if samePaths {
			return c.cert, nil // keep serving the loaded cert during a rename/rewrite
		}
		return nil, err
	}
	keyInfo, err := os.Stat(keyFile)
	if err != nil {
		if samePaths {
			return c.cert, nil
		}
		return nil, err
	}
	c.checkedAt = now
	if samePaths && certInfo.ModTime().Equal(c.certMod) && keyInfo.ModTime().Equal(c.keyMod) {
		return c.cert, nil
	}

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		if samePaths {
			// A half-written renewal: keep the previous cert, retry next second.
			return c.cert, nil
		}
		return nil, err
	}
	c.cert = &cert
	c.certFile, c.keyFile = certFile, keyFile
	c.certMod, c.keyMod = certInfo.ModTime(), keyInfo.ModTime()
	return c.cert, nil
}

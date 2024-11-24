package ssh_server

import (
	"encoding/binary"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/exec"
	"sync"
	"syscall"
	"unsafe"

	"github.com/kr/pty"
	"golang.org/x/crypto/ssh"
)

const privateKey = `-----BEGIN OPENSSH PRIVATE KEY-----
b3BlbnNzaC1rZXktdjEAAAAABG5vbmUAAAAEbm9uZQAAAAAAAAABAAABFwAAAAdzc2gtcn
NhAAAAAwEAAQAAAQEA0xSBmXhcXj8Lk88xh/kXraG2GqE4jCemPY4CmI+I7TwBmHhW00c+
9znwYPW3bjXME/XkyG4Dj6yhIEwXp586N0cF40JROH8e7K9elF07hFaDchwQaRLppTk2d0
yMcyMAM8gzm3Z3/UHVqVbS71Xu9tt6yjU9wY6kFrNXB0tjC9jMmtXpQhKQ+KjazVbA1eYj
E5JP6uRXNKRxW48lsTxphiwgBVcUosb+WN7D9EKcex7pbDrIMC0/PAsqhGQY13fiPjQ1V/
fK8HdMcerVQJWN8adw0jNDPOc7a794dNBuYp5xiAiwP3JT06rXtd/c2qHPtRpEpibDxcet
uqogWZkryQAAA9hbW9I8W1vSPAAAAAdzc2gtcnNhAAABAQDTFIGZeFxePwuTzzGH+Retob
YaoTiMJ6Y9jgKYj4jtPAGYeFbTRz73OfBg9bduNcwT9eTIbgOPrKEgTBennzo3RwXjQlE4
fx7sr16UXTuEVoNyHBBpEumlOTZ3TIxzIwAzyDObdnf9QdWpVtLvVe7223rKNT3BjqQWs1
cHS2ML2Mya1elCEpD4qNrNVsDV5iMTkk/q5Fc0pHFbjyWxPGmGLCAFVxSixv5Y3sP0Qpx7
HulsOsgwLT88CyqEZBjXd+I+NDVX98rwd0xx6tVAlY3xp3DSM0M85ztrv3h00G5innGICL
A/clPTqte139zaoc+1GkSmJsPFx626qiBZmSvJAAAAAwEAAQAAAQBaIv2c3csD7AQzoFzU
Zch4uv+aq5IMN7pDuurc3x5nwCImS+0318rJpBJENWmZRJvbQjvqYyBeMCe2NQg86j/f7x
JSk7U/XPmFtPW8gXuy7YbAKb/QPuVLSv05QJURbbbeZfWzw4lFuuFUqOD2l0muXNc4ljfC
+fiUQQ0+7jBjk/BbDwL/V00Mgoofnz7hk2wFhZEaB3sAimLE3DSV6Siz+nP2rxZwBa1ndA
gtPT6XY4Ax3qongmA+TTXFoo4E0A3MFz08AoYsHzULiteEdLCNPriXTPiso/gcX08wwHKx
nrzsJC+u1RKo68rPDNHxAgFBIqMcwSNHbsh3FnRAJ2RJAAAAgEsWdCw6zSBd4OsLAIn58S
SiVwoRETwb09KYIjFbiCXZnmQnV83rNf40Z6d9pU1OFmdjyTsv+CdTtES5gwqxabTSmEwc
R+Pf8jkF7MkTJTL+n4zUK/Ob6DRfXellnx9Xl5tldpps0zQoBkZY0QhgxFeramIv79VGr1
0HwdIEPg1aAAAAgQDvrpLKJSF+9IYc7nObrCrkOqjNM2ecY0mm5o/MI964fNaZGBjvKTA4
L+27J/WYa9ShZx/OZRPhan3fITCjIyXWCa7w8cdwBVukgE5rHRGNcfQubW8bV8pjsWSg3Y
Ym29e6mFC8/sZFUMNoe3vqp2wMAO8uxJyzr6aeMDg64Za7cwAAAIEA4XNuL+eN9qrE4hx9
6kOrt4L3dsFvLp/DTCNNTy2O4X04seQV8TukOd1Vg1yrv4ooWSGjMvaUxIBdRRFrhLZCU6
nfZNMVBV2fCf9VSvPRrDcKKVRXaVIpllt29yF5fDcNa1WAQyMQaJ0UcTuZ7NdaHSxr4jQe
ugD/fZXoSdjdpNMAAAAhb251cmFndGFzQE9udXJzLU1hY0Jvb2stUHJvLmxvY2FsAQI=
-----END OPENSSH PRIVATE KEY-----
`

type SSHClient struct{}

var sshClient SSHClient

func NewSSHClient() *SSHClient {
	sshClient = SSHClient{}
	return &sshClient
}

func (s *SSHClient) GetClient() *SSHClient {
	return &sshClient
}

func (t *SSHClient) Start() {
	config := &ssh.ServerConfig{
		NoClientAuth: true,
		PasswordCallback: func(c ssh.ConnMetadata, pass []byte) (*ssh.Permissions, error) {
			if c.User() == "foo" && string(pass) == "bar" {
				return nil, nil
			}
			return nil, fmt.Errorf("password rejected for %q", c.User())
		},
	}

	private, err := ssh.ParsePrivateKey([]byte(privateKey))
	if err != nil {
		log.Fatal("Failed to parse private key")
	}

	config.AddHostKey(private)

	listener, err := net.Listen("tcp4", "0.0.0.0:2222")
	if err != nil {
		log.Fatalf("Failed to listen on 2222 (%s)", err)
	}

	log.Print("Listening on 2222...")
	for {
		tcpConn, err := listener.Accept()
		if err != nil {
			log.Printf("Failed to accept incoming connection (%s)", err)
			continue
		}

		clientIP, _, _ := net.SplitHostPort(tcpConn.RemoteAddr().String())
		if !isPrivateIP(clientIP) {
			log.Printf("Connection rejected from %s", clientIP)
			tcpConn.Close()
			continue
		}

		sshConn, chans, reqs, err := ssh.NewServerConn(tcpConn, config)
		if err != nil {
			log.Printf("Failed to handshake (%s)", err)
			continue
		}

		log.Printf("New SSH connection from %s (%s)", sshConn.RemoteAddr(), sshConn.ClientVersion())
		go ssh.DiscardRequests(reqs)
		go handleChannels(chans)
	}
}

func handleChannels(chans <-chan ssh.NewChannel) {
	for newChannel := range chans {
		go handleChannel(newChannel)
	}
}

func handleChannel(newChannel ssh.NewChannel) {
	if t := newChannel.ChannelType(); t != "session" {
		newChannel.Reject(ssh.UnknownChannelType, fmt.Sprintf("unknown channel type: %s", t))
		return
	}

	connection, requests, err := newChannel.Accept()
	if err != nil {
		log.Printf("Could not accept channel (%s)", err)
		return
	}

	bash := exec.Command("bash")

	close := func() {
		connection.Close()
		_, err := bash.Process.Wait()
		if err != nil {
			log.Printf("Failed to exit bash (%s)", err)
		}
		log.Printf("Session closed")
	}

	log.Print("Creating pty...")
	bashf, err := pty.Start(bash)
	if err != nil {
		log.Printf("Could not start pty (%s)", err)
		close()
		return
	}

	// Varsayılan TERM değişkenini ayarla
	os.Setenv("TERM", "xterm-256color")

	var once sync.Once
	go func() {
		io.Copy(connection, bashf)
		once.Do(close)
	}()
	go func() {
		io.Copy(bashf, connection)
		once.Do(close)
	}()

	go func() {
		for req := range requests {
			switch req.Type {
			case "shell":
				if len(req.Payload) == 0 {
					req.Reply(true, nil)
				}
			case "pty-req":
				termLen := req.Payload[3]
				term := string(req.Payload[4 : 4+termLen])
				log.Printf("Client requested TERM: %s", term)

				// Gelen TERM değişkenini ayarla
				os.Setenv("TERM", term)

				w, h := parseDims(req.Payload[termLen+4:])
				SetWinsize(bashf.Fd(), w, h)
				req.Reply(true, nil)
			case "window-change":
				w, h := parseDims(req.Payload)
				SetWinsize(bashf.Fd(), w, h)
			case "env":
				// Çevre değişkenlerini işleme
				key, value := parseEnv(req.Payload)
				if key == "TERM" {
					log.Printf("Setting TERM to: %s", value)
					os.Setenv(key, value)
				}
			}
		}
	}()
}

func parseDims(b []byte) (uint32, uint32) {
	w := binary.BigEndian.Uint32(b)
	h := binary.BigEndian.Uint32(b[4:])
	return w, h
}

func parseEnv(payload []byte) (string, string) {
	nullIndex := 0
	for i, b := range payload {
		if b == 0 {
			nullIndex = i
			break
		}
	}
	key := string(payload[:nullIndex])
	value := string(payload[nullIndex+1:])
	return key, value
}

type Winsize struct {
	Height uint16
	Width  uint16
	x      uint16
	y      uint16
}

func SetWinsize(fd uintptr, w, h uint32) {
	ws := &Winsize{Width: uint16(w), Height: uint16(h)}
	syscall.Syscall(syscall.SYS_IOCTL, fd, uintptr(syscall.TIOCSWINSZ), uintptr(unsafe.Pointer(ws)))
}

func isPrivateIP(ip string) bool {
	privateBlocks := []string{
		"127.0.0.1/32",
		"10.0.0.0/8",
		"172.16.0.0/12",
		"192.168.0.0/16",
	}

	for _, block := range privateBlocks {
		_, cidr, _ := net.ParseCIDR(block)
		parsedIP := net.ParseIP(ip)
		if cidr.Contains(parsedIP) {
			return true
		}
	}
	return false
}

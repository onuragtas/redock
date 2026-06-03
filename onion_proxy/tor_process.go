package onion_proxy

import (
	"bytes"
	"log"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// torLogWriter Tor'un stderr/log çıktısını "tor:" prefix'iyle redock log'una
// yönlendirir. bine'in DebugWriter'ı tüm protokol trafiğini de yazar — biraz
// gürültülü ama hata teşhisinde paha biçilmez.
type torLogWriter struct{}

func (torLogWriter) Write(p []byte) (int, error) {
	line := strings.TrimRight(string(p), "\n")
	if line != "" {
		log.Printf("tor: %s", line)
	}
	return len(p), nil
}

// killStaleTor önceki redock run'undan kalmış ve bizim data dir'imizi
// kilitleyen bir Tor process'i varsa onu sonlandırır. pgrep darwin+linux'ta
// mevcut; windows release pipeline'da kapalı.
func killStaleTor(dataDir string) {
	out, err := exec.Command("pgrep", "-f", "tor.*--DataDirectory "+dataDir).Output()
	if err != nil {
		// pgrep eşleşme bulamazsa exit 1 verir — bu normal, log'lama.
		return
	}
	for _, line := range bytes.Split(bytes.TrimSpace(out), []byte("\n")) {
		pidStr := strings.TrimSpace(string(line))
		if pidStr == "" {
			continue
		}
		pid, err := strconv.Atoi(pidStr)
		if err != nil {
			continue
		}
		log.Printf("onion_proxy: önceki run'dan kalan tor process'i bulundu (pid=%d), sonlandırılıyor", pid)
		_ = syscall.Kill(pid, syscall.SIGTERM)
		// 2sn bekle, hâlâ ayaktaysa SIGKILL
		time.Sleep(2 * time.Second)
		if syscall.Kill(pid, 0) == nil {
			_ = syscall.Kill(pid, syscall.SIGKILL)
		}
	}
}

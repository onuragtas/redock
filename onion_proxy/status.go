package onion_proxy

import (
	"os/exec"
	"runtime"
	"strings"
)

// InstallHint platform-spesifik kurulum komutu için frontend'in göstereceği DTO.
type InstallHint struct {
	OS      string `json:"os"`      // runtime.GOOS
	Arch    string `json:"arch"`    // runtime.GOARCH
	Manager string `json:"manager"` // brew, apt, dnf, pacman, choco, …
	Command string `json:"command"` // kullanıcıya gösterilecek tek satırlık komut
	URL     string `json:"url"`     // dokümantasyon / indirme bağlantısı
}

// Status hidden service yönetimi için Tor'un durumunu raporlar.
type Status struct {
	Installed              bool         `json:"installed"`               // PATH'te tor bulundu mu
	BinaryPath             string       `json:"binary_path"`             // bulunduysa nerede
	Version                string       `json:"version"`                 // `tor --version` çıktısı (kısaltılmış)
	TorRunning             bool         `json:"tor_running"`             // manager Tor sürecini başlatmış mı
	OnionCount             int          `json:"onion_count"`             // kayıtlı hidden service sayısı
	SystemServiceConflict  bool         `json:"system_service_conflict"` // sistem `tor` servisi aktif → redock'un Tor'uyla çakışır
	InstallHint            *InstallHint `json:"install_hint,omitempty"`
}

// Status manager'ın durumunu döner. Bu çağrı Tor sürecini başlatmaz —
// yalnızca PATH kontrolü ve mevcut state'i raporlar.
func (m *Manager) Status() Status {
	st := Status{}

	path, err := exec.LookPath("tor")
	if err == nil {
		st.Installed = true
		st.BinaryPath = path
		if out, vErr := exec.Command(path, "--version").Output(); vErr == nil {
			line := strings.TrimSpace(strings.SplitN(string(out), "\n", 2)[0])
			st.Version = line
		}
		if SystemTorServiceRunning() {
			st.SystemServiceConflict = true
		}
	} else {
		st.InstallHint = installHintForCurrent()
	}

	m.mu.Lock()
	st.TorRunning = m.torReady && m.tor != nil
	m.mu.Unlock()

	st.OnionCount = len(m.List())
	return st
}

// SystemTorServiceRunning systemd üzerindeki "tor" servisinin aktif olup
// olmadığını döner. Linux dışı sistemlerde / systemctl yokluğunda false.
// Aktifse redock'un kendi Tor'u büyük olasılıkla çakışır.
func SystemTorServiceRunning() bool {
	if _, err := exec.LookPath("systemctl"); err != nil {
		return false
	}
	// `is-active` aktif servis için exit 0, değilse exit 3. Output yine
	// "active" / "inactive" döner; biz exit code'una bakıyoruz.
	cmd := exec.Command("systemctl", "is-active", "--quiet", "tor")
	return cmd.Run() == nil
}

// installHintForCurrent runtime.GOOS/GOARCH'a göre kullanıcıya gösterilecek
// kurulum talimatını döner. Resmi olarak desteklenmeyen platformlar için
// genel bir yönlendirme verilir.
func installHintForCurrent() *InstallHint {
	h := &InstallHint{OS: runtime.GOOS, Arch: runtime.GOARCH}
	switch runtime.GOOS {
	case "darwin":
		h.Manager = "brew"
		h.Command = "brew install tor"
		h.URL = "https://formulae.brew.sh/formula/tor"
	case "linux":
		// Hangi dağıtım olduğunu bilmediğimiz için en yaygın 3'ünü öneriyoruz.
		// İKİNCİ KOMUT ŞART: paketler otomatik systemd service başlatır;
		// redock kendi Tor instance'ını yönetir, sistem servisi açıkken
		// iki Tor çakışır ve bootstrap fail eder.
		h.Manager = "apt|dnf|pacman"
		h.Command = "sudo apt install tor   # Debian/Ubuntu\n" +
			"sudo dnf install tor   # Fedora/RHEL\n" +
			"sudo pacman -S tor     # Arch\n" +
			"\n" +
			"# ÖNEMLİ: redock kendi Tor instance'ını yönetir, sistem servisini kapatın:\n" +
			"sudo systemctl disable --now tor"
		h.URL = "https://community.torproject.org/onion-services/setup/install/"
	case "windows":
		h.Manager = "choco"
		h.Command = "choco install tor"
		h.URL = "https://www.torproject.org/download/tor/"
	default:
		h.Manager = ""
		h.Command = ""
		h.URL = "https://www.torproject.org/download/tor/"
	}
	return h
}

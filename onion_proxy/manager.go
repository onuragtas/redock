package onion_proxy

import (
	"context"
	"encoding/base64"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"time"

	"redock/api_gateway"
	dockermanager "redock/docker-manager"
	"redock/platform/database"
	"redock/platform/memory"

	"github.com/cretz/bine/control"
	"github.com/cretz/bine/tor"
)

// NOTE: Tor C ile yazıldığı için tek binary'ye saf-Go gömülemiyor; CGO ile
// gömmek ise mevcut CGO_ENABLED=0 release pipeline'ını kırardı. Bu yüzden
// onion_proxy sistemde kurulu `tor` binary'sine bağımlıdır. Yoksa frontend'e
// `Status().InstallHint` üzerinden kurulum talimatı gösterilir.

const (
	torStartTimeout = 90 * time.Second
)

type Manager struct {
	mu sync.Mutex

	dockerManager *dockermanager.DockerEnvironmentManager
	dataDir       string

	// torCtx Tor sürecinin parent context'i; manager ömrü boyunca yaşar.
	// tor.Start'a request ctx vermek tehlikelidir — bine processi o ctx'e
	// bağlar (tor.go:293) ve request bitince Tor da SIGKILL alır.
	torCtx    context.Context
	torCancel context.CancelFunc

	tor       *tor.Tor
	torErr    error
	torReady  bool
	starting  bool
	startOnce chan struct{} // beklemek isteyenler için (lazy start)
}

var (
	manager     *Manager
	managerLock sync.Mutex
)

func Init(dm *dockermanager.DockerEnvironmentManager) {
	managerLock.Lock()
	defer managerLock.Unlock()

	dataDir := filepath.Join(dm.GetWorkDir(), "data", "tor")
	if err := os.MkdirAll(dataDir, 0700); err != nil {
		log.Printf("onion_proxy: data dir oluşturulamadı: %v", err)
	}

	torCtx, torCancel := context.WithCancel(context.Background())
	manager = &Manager{
		dockerManager: dm,
		dataDir:       dataDir,
		torCtx:        torCtx,
		torCancel:     torCancel,
	}

	// Kalıcı kayıtları geri yükle (Tor lazy-start; ilk publish'te bootstrap olur).
	go manager.restoreAll()
}

func GetManager() *Manager {
	managerLock.Lock()
	defer managerLock.Unlock()
	return manager
}

// ensureTor lazy olarak Tor sürecini başlatır. Birden çok eşzamanlı çağrıda
// tek bir bootstrap çalışır; sonraki çağrılar mevcut başlatma bitince döner.
func (m *Manager) ensureTor(ctx context.Context) (*tor.Tor, error) {
	m.mu.Lock()
	if m.torReady && m.tor != nil {
		t := m.tor
		m.mu.Unlock()
		return t, nil
	}
	if m.starting {
		ch := m.startOnce
		m.mu.Unlock()
		select {
		case <-ch:
		case <-ctx.Done():
			return nil, ctx.Err()
		}
		m.mu.Lock()
		defer m.mu.Unlock()
		if m.torErr != nil {
			return nil, m.torErr
		}
		return m.tor, nil
	}

	m.starting = true
	m.startOnce = make(chan struct{})
	startCh := m.startOnce
	m.mu.Unlock()

	// Bootstrap'i goroutine'de çalıştır. Tor process'inin parent'ı m.torCtx
	// (background), request ctx'i değil — aksi halde request bitince Tor SIGKILL alır.
	// Caller aşağıdaki select ile request ctx'i altında bekler.
	go func() {
		// Önceki redock run'undan leak etmiş Tor process'i bizim data dir'i
		// kilit altında tutuyorsa yeni Tor exit 1 verir ("Invalid port format").
		// Başlamadan önce temizle.
		killStaleTor(m.dataDir)

		log.Println("onion_proxy: Tor başlatılıyor (bootstrap 10-60sn sürebilir)...")
		t, err := tor.Start(m.torCtx, &tor.StartConf{
			DataDir:         m.dataDir,
			EnableNetwork:   true,
			NoAutoSocksPort: true,
			DebugWriter:     torLogWriter{},
		})

		m.mu.Lock()
		m.starting = false
		if err != nil {
			m.torErr = fmt.Errorf("tor start: %w", err)
		} else {
			m.tor = t
			m.torReady = true
			m.torErr = nil
			log.Println("onion_proxy: Tor hazır.")
		}
		close(startCh)
		m.mu.Unlock()
	}()

	select {
	case <-startCh:
	case <-ctx.Done():
		// Caller vazgeçti ama Tor arkada bootstrap'a devam ediyor.
		return nil, ctx.Err()
	}

	m.mu.Lock()
	defer m.mu.Unlock()
	if m.torErr != nil {
		return nil, m.torErr
	}
	return m.tor, nil
}

// gatewayHTTPPort api_gateway HTTP portunu döner (.onion → gateway forward
// hedefi). Gateway hazır değilse 80 varsayılır.
func gatewayHTTPPort() int {
	g := api_gateway.GetGateway()
	if g == nil {
		return 80
	}
	cfg := g.GetConfig()
	if cfg == nil || cfg.HTTPPort == 0 {
		return 80
	}
	return cfg.HTTPPort
}

// publishService Tor'a tek bir hidden service kaydeder ve onion adresini döner.
func (m *Manager) publishService(ctx context.Context, e *OnionServiceEntity) (string, error) {
	t, err := m.ensureTor(ctx)
	if err != nil {
		return "", err
	}

	targetHost, targetPort := e.TargetHost, e.TargetPort
	if e.RouteID != "" || targetHost == "" || targetPort == 0 {
		// Gateway entegrasyon modu: tüm trafik api_gateway HTTP listener'ına gider.
		targetHost = "127.0.0.1"
		targetPort = gatewayHTTPPort()
	}

	virtualPort := e.VirtualPort
	if virtualPort == 0 {
		virtualPort = 80
	}

	req := &control.AddOnionRequest{
		Ports: []*control.KeyVal{{
			Key: strconv.Itoa(virtualPort),
			Val: net_JoinHostPort(targetHost, targetPort),
		}},
	}

	if e.PrivateKey != "" {
		k, kErr := control.ED25519KeyFromBlob(e.PrivateKey)
		if kErr != nil {
			return "", fmt.Errorf("private_key parse: %w", kErr)
		}
		req.Key = k
	} else {
		req.Key = control.GenKey(control.KeyAlgoED25519V3)
	}

	resp, err := t.Control.AddOnion(req)
	if err != nil {
		return "", fmt.Errorf("ADD_ONION: %w", err)
	}

	// İlk publish: üretilmiş key'i geri yaz
	if e.PrivateKey == "" && resp.Key != nil {
		if ek, ok := resp.Key.(*control.ED25519Key); ok {
			e.PrivateKey = base64.StdEncoding.EncodeToString(ek.PrivateKey())
		}
	}
	return resp.ServiceID + ".onion", nil
}

// net.JoinHostPort sarmalayıcısı; küçük bir indirection (Bash'te grep'lenirken
// "net." import'una karışmaması için).
func net_JoinHostPort(host string, port int) string {
	return host + ":" + strconv.Itoa(port)
}

// restoreAll süreç başlangıcında kayıtlı hidden service'leri Tor'a tekrar
// register eder. Hata olursa kayıt enabled=false olarak işaretlenir.
func (m *Manager) restoreAll() {
	db := database.GetMemoryDB()
	list := memory.FindAll[*OnionServiceEntity](db, TableOnionServices)
	enabled := 0
	for _, e := range list {
		if !e.Enabled {
			continue
		}
		enabled++
	}
	if enabled == 0 {
		return
	}
	if _, err := exec.LookPath("tor"); err != nil {
		log.Printf("onion_proxy: %d kayıt var ama tor PATH'te yok; restore atlandı", enabled)
		return
	}

	ctx := context.Background()
	for _, e := range list {
		if !e.Enabled {
			continue
		}
		addr, err := m.publishService(ctx, e)
		if err != nil {
			log.Printf("onion_proxy: %q restore başarısız: %v", e.Name, err)
			continue
		}
		// Adres beklenenden farklıysa güncelle (genelde aynı olur).
		if addr != e.OnionAddress {
			e.OnionAddress = addr
			_ = memory.Update(db, TableOnionServices, e)
		}
		log.Printf("onion_proxy: %q restored → %s", e.Name, e.OnionAddress)
	}
}

// CreateInput dış API için DTO.
type CreateInput struct {
	Name        string `json:"name"`
	RouteID     string `json:"route_id,omitempty"`
	TargetHost  string `json:"target_host,omitempty"`
	TargetPort  int    `json:"target_port,omitempty"`
	VirtualPort int    `json:"virtual_port,omitempty"`
}

// Create yeni bir hidden service oluşturur, Tor'da publish eder ve gerekli
// ise seçili api_gateway route'una alias olarak ekler.
func (m *Manager) Create(in CreateInput) (*OnionServiceEntity, error) {
	if in.Name == "" {
		return nil, fmt.Errorf("name zorunlu")
	}
	if in.RouteID == "" && (in.TargetHost == "" || in.TargetPort == 0) {
		return nil, fmt.Errorf("route_id ya da target_host+target_port verilmeli")
	}
	if _, err := exec.LookPath("tor"); err != nil {
		return nil, fmt.Errorf("tor binary PATH'te bulunamadı; /api/v1/onion/status ile kurulum talimatına bakın")
	}

	e := &OnionServiceEntity{
		Name:        in.Name,
		RouteID:     in.RouteID,
		TargetHost:  in.TargetHost,
		TargetPort:  in.TargetPort,
		VirtualPort: in.VirtualPort,
		Enabled:     true,
		CreatedAt:   time.Now(),
	}
	// Default virtual port:
	//   - gateway alias modu → 80 (browser default; Host header'la route eşleşir)
	//   - direct target modu → target_port'u aynala (SSH gibi non-HTTP servisler doğal görünür)
	if e.VirtualPort == 0 {
		if e.RouteID == "" && e.TargetPort > 0 {
			e.VirtualPort = e.TargetPort
		} else {
			e.VirtualPort = 80
		}
	}

	ctx, cancel := context.WithTimeout(context.Background(), torStartTimeout)
	defer cancel()

	addr, err := m.publishService(ctx, e)
	if err != nil {
		return nil, err
	}
	e.OnionAddress = addr

	db := database.GetMemoryDB()
	if err := memory.Create(db, TableOnionServices, e); err != nil {
		// Tor'da publish edildi ama kalıcı saklayamadık → DELONION ile geri al.
		_ = m.tor.Control.DelOnion(serviceIDFromAddress(addr))
		return nil, fmt.Errorf("persist: %w", err)
	}

	if e.RouteID != "" {
		if err := attachOnionToRoute(e.RouteID, addr); err != nil {
			log.Printf("onion_proxy: route alias eklenemedi (%s): %v", e.RouteID, err)
		}
	}
	return e, nil
}

// EditInput Update için DTO. Pointer'lar "alan dokunulmadı" semantiği taşır:
// bir alan nil ise mevcut değer korunur. Bool için ayrı pointer şart, yoksa
// false/"unset" karışır.
type EditInput struct {
	Name    *string `json:"name,omitempty"`
	RouteID *string `json:"route_id,omitempty"`
	Enabled *bool   `json:"enabled,omitempty"`
}

// Update mevcut bir hidden service'i günceller. .onion adresi her durumda
// korunur (republish aynı private key ile yapılır).
func (m *Manager) Update(id uint, in EditInput) (*OnionServiceEntity, error) {
	db := database.GetMemoryDB()
	list := memory.FindAll[*OnionServiceEntity](db, TableOnionServices)
	var target *OnionServiceEntity
	for _, e := range list {
		if e.GetID() == id {
			target = e
			break
		}
	}
	if target == nil {
		return nil, fmt.Errorf("onion service %d bulunamadı", id)
	}

	oldRouteID := target.RouteID
	oldEnabled := target.Enabled

	// 1) DB-only alanlar
	if in.Name != nil {
		target.Name = *in.Name
	}

	// 2) Route değişimi (alias modu): eski route'tan host'u çıkar, yenisine ekle.
	// Sadece kayıt enabled iken gateway tarafına dokunulur — disable'da alias zaten yok.
	routeChanged := in.RouteID != nil && *in.RouteID != oldRouteID
	if routeChanged {
		target.RouteID = *in.RouteID
	}

	// 3) Enabled toggle / target değişimi → Tor'da republish
	willBeEnabled := oldEnabled
	if in.Enabled != nil {
		willBeEnabled = *in.Enabled
	}
	target.Enabled = willBeEnabled

	tor := m.currentTor()

	// Disabled → unpublish (varsa) ve alias kaldır
	if oldEnabled && !willBeEnabled {
		if tor != nil && target.OnionAddress != "" {
			_ = tor.Control.DelOnion(serviceIDFromAddress(target.OnionAddress))
		}
		if oldRouteID != "" && target.OnionAddress != "" {
			if err := detachOnionFromRoute(oldRouteID, target.OnionAddress); err != nil {
				log.Printf("onion_proxy: update sırasında alias kaldırılamadı: %v", err)
			}
		}
	}

	// Enabled-kalacak ve route değişti → eski route'tan alias'ı çıkar
	if oldEnabled && willBeEnabled && routeChanged && oldRouteID != "" && target.OnionAddress != "" {
		if err := detachOnionFromRoute(oldRouteID, target.OnionAddress); err != nil {
			log.Printf("onion_proxy: update eski route detach: %v", err)
		}
	}

	// Disabled → Enabled, ya da enabled iken route/target değişimi → republish
	needRepublish := !oldEnabled && willBeEnabled
	if oldEnabled && willBeEnabled && routeChanged {
		// route değişimi target'ı (gateway port'unu) etkilemiyor (her ikisi de gateway),
		// ama yine de Hosts attach mantığı için republish'e gerek yok — sadece
		// gateway route alias değişiyor. Tor tarafı dokunulmaz.
	}
	if needRepublish {
		ctx, cancel := context.WithTimeout(context.Background(), torStartTimeout)
		defer cancel()
		addr, err := m.publishService(ctx, target)
		if err != nil {
			return nil, fmt.Errorf("republish: %w", err)
		}
		// Aynı private key ile publish olduğu için adres değişmez; yine de defensive update.
		target.OnionAddress = addr
	}

	// Enabled durumda route eklenmesi gereken senaryolar
	if willBeEnabled && target.RouteID != "" && target.OnionAddress != "" {
		if routeChanged || needRepublish {
			if err := attachOnionToRoute(target.RouteID, target.OnionAddress); err != nil {
				log.Printf("onion_proxy: update sonrası route attach: %v", err)
			}
		}
	}

	if err := memory.Update(db, TableOnionServices, target); err != nil {
		return nil, fmt.Errorf("persist: %w", err)
	}
	return target, nil
}

// currentTor lock alarak güvenle tor pointer'ı döner; ready değilse nil.
func (m *Manager) currentTor() *tor.Tor {
	m.mu.Lock()
	defer m.mu.Unlock()
	if !m.torReady {
		return nil
	}
	return m.tor
}

// Delete kaydı kaldırır, Tor'dan hidden service'i siler ve gateway route'undan
// alias'ı çıkarır.
func (m *Manager) Delete(id uint) error {
	db := database.GetMemoryDB()
	list := memory.FindAll[*OnionServiceEntity](db, TableOnionServices)
	var target *OnionServiceEntity
	for _, e := range list {
		if e.GetID() == id {
			target = e
			break
		}
	}
	if target == nil {
		return fmt.Errorf("onion service %d bulunamadı", id)
	}

	if m.tor != nil && target.OnionAddress != "" {
		_ = m.tor.Control.DelOnion(serviceIDFromAddress(target.OnionAddress))
	}
	if target.RouteID != "" && target.OnionAddress != "" {
		if err := detachOnionFromRoute(target.RouteID, target.OnionAddress); err != nil {
			log.Printf("onion_proxy: route alias kaldırılamadı: %v", err)
		}
	}
	return memory.Delete[*OnionServiceEntity](db, TableOnionServices, id)
}

// List kayıtlı tüm hidden service'leri döner.
func (m *Manager) List() []*OnionServiceEntity {
	db := database.GetMemoryDB()
	return memory.FindAll[*OnionServiceEntity](db, TableOnionServices)
}

// Shutdown Tor sürecini kapatır (graceful shutdown için).
func (m *Manager) Shutdown() {
	m.mu.Lock()
	t := m.tor
	cancel := m.torCancel
	m.tor = nil
	m.torReady = false
	m.mu.Unlock()
	if t != nil {
		_ = t.Close()
	}
	if cancel != nil {
		cancel()
	}
}

func serviceIDFromAddress(addr string) string {
	if len(addr) > len(".onion") && addr[len(addr)-len(".onion"):] == ".onion" {
		return addr[:len(addr)-len(".onion")]
	}
	return addr
}

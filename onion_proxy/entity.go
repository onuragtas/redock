package onion_proxy

import (
	"redock/platform/memory"
	"time"
)

const TableOnionServices = "onion_services"

// OnionServiceEntity, kullanıcı tarafından oluşturulmuş bir Tor hidden
// service'in kalıcı kaydıdır. Private key burada saklanır; redock yeniden
// başlatıldığında aynı .onion adresi geri yüklenir.
//
// İki entegrasyon modu desteklenir:
//   - RouteID dolu ise: oluşturulan .onion adresi seçilen api_gateway
//     route'unun Hosts dizisine alias olarak eklenir. İstek normal gateway
//     pipeline'ından geçer (rate limit, auth, observability dahil).
//   - RouteID boş, TargetHost/TargetPort dolu ise: hidden service direkt
//     verilen TCP adresine forward eder, gateway'i bypass eder.
type OnionServiceEntity struct {
	memory.BaseEntity
	Name         string    `json:"name"`
	OnionAddress string    `json:"onion_address"`     // xxxxxxxx.onion (boş = henüz publish edilmemiş)
	PrivateKey   string    `json:"private_key"`       // base64; ED25519-V3
	VirtualPort  int       `json:"virtual_port"`      // .onion:VirtualPort (default 80)
	RouteID      string    `json:"route_id,omitempty"` // api_gateway entegrasyon modu
	TargetHost   string    `json:"target_host,omitempty"`
	TargetPort   int       `json:"target_port,omitempty"`
	Enabled      bool      `json:"enabled"`
	CreatedAt    time.Time `json:"created_at"`
}

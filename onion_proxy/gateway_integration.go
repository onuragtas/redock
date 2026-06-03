package onion_proxy

import (
	"fmt"

	"redock/api_gateway"
)

// attachOnionToRoute verilen route'un Hosts dizisine onion adresini ekler
// (zaten varsa atlar). Gateway entegrasyon modunda Tor üzerinden gelen
// istekler bu sayede normal api_gateway route eşleştirmesinden geçer.
//
// UpdateRoute yerine UpdateConfig kullanıyoruz: kullanıcının mevcut
// config'inde upstream_id'siz "legacy" route'lar olabiliyor (UI'dan bulk
// kayıt yolu validation çalıştırmıyor) ve UpdateRoute bunları reddediyor.
// Biz sadece Hosts ekliyoruz — toptan config yazımı zararsız.
func attachOnionToRoute(routeID, onionAddr string) error {
	g := api_gateway.GetGateway()
	if g == nil {
		return fmt.Errorf("api_gateway henüz başlatılmadı")
	}
	cfg := g.GetConfigCopy()
	if cfg == nil {
		return fmt.Errorf("gateway config okunamadı")
	}
	for i := range cfg.Routes {
		if cfg.Routes[i].ID != routeID {
			continue
		}
		for _, h := range cfg.Routes[i].Hosts {
			if h == onionAddr {
				return nil
			}
		}
		cfg.Routes[i].Hosts = append(cfg.Routes[i].Hosts, onionAddr)
		return g.UpdateConfig(cfg)
	}
	return fmt.Errorf("route %s bulunamadı", routeID)
}

// detachOnionFromRoute Hosts dizisinden onion adresini çıkarır.
func detachOnionFromRoute(routeID, onionAddr string) error {
	g := api_gateway.GetGateway()
	if g == nil {
		return nil
	}
	cfg := g.GetConfigCopy()
	if cfg == nil {
		return nil
	}
	for i := range cfg.Routes {
		if cfg.Routes[i].ID != routeID {
			continue
		}
		filtered := make([]string, 0, len(cfg.Routes[i].Hosts))
		changed := false
		for _, h := range cfg.Routes[i].Hosts {
			if h == onionAddr {
				changed = true
				continue
			}
			filtered = append(filtered, h)
		}
		if !changed {
			return nil
		}
		cfg.Routes[i].Hosts = filtered
		return g.UpdateConfig(cfg)
	}
	return nil
}

# Memory DB Kullanmayan – Doğrudan JSON Dosya Okuyan/Yazan Yerler

Bu belge, projede **platform/memory** (memory DB) yerine **doğrudan JSON read/write** veya **dosya tabanlı persistence** kullanan alanları **menü/özellik bazında** gruplar.

*(platform/memory, migration status, TLS/key dosyaları, HTTP body parse, WebSocket mesajı gibi altyapı/network kullanımları hariç.)*

---

## 1. Dev Environment (DevEnv) — memory DB’ye taşındı

- **Eski dosya:** `{workdir}/devenv.json` (migration ile `dev_envs` tablosuna aktarıldı, dosya `devenv.json.bak` yapılır)
- **Yeni:** `platform/memory` tablosu `dev_envs`, entity `devenv.DevEnvEntity`
- **Kod:** `devenv/init.go` (memory CRUD), `devenv/entity.go`, `app/controllers/docker_controller.go` (GetDevEnv memory’den okur)

---

## 2. Deployment

- **Dosya:** `{workdir}/data/deployment.json`
- **Ne:** Deployment config (username, token, settings, projects)
- **Nerede:** `deployment/init.go`: LoadConfigUnsafe, LoadConfig, ve tüm proje ekleme/silme/güncelleme sonrası WriteFile

---

## 3. Container Settings / Docker Manager

### 3.1 Service settings

- **Dosya:** `{workdir}/service-settings.json`
- **Ne:** Servis override’ları (custom name, portlar)
- **Nerede:** `docker-manager/service_settings.go`: loadServiceSettings (ReadFile + Unmarshal), SaveServiceSettings (Marshal + WriteFile)

### 3.2 Virtual Hosts – yıldızlı liste (starred)

- **Dosya:** `{workdir}/data/starred_vhosts.json`
- **Ne:** Yıldızlı vhost path listesi
- **Nerede:** `docker-manager/virtualhost.go`: GetStarredVHosts, StarVHost, UnstarVHost, saveStarredVHosts (ReadFile/WriteFile + json Marshal/Unmarshal)

*Not: Vhost içeriği (nginx/httpd .conf) ve .env dosyaları bilinçli “config dosyası” yazımı; memory DB alternatifi değil.*

---

## 4. API Gateway

- **Dosyalar:**
  - `{workdir}/data/api_gateway.json` – ana konfig (portlar, servisler, route’lar, güvenlik)
  - Block list dosyası – engellenen IP’ler (blockListFilePath())
- **Ne:** Gateway konfigürasyonu ve kalıcı block list
- **Nerede:** `api_gateway/gateway.go`: loadConfig, saveConfigLocked (api_gateway.json); loadBlockList, writeBlockList (block list JSON)

---

## 5. PHP XDebug Adapter

- **Dosya:** `{workdir}/data/settings.json`
- **Ne:** Listen adresi ve path mapping listesi
- **Nerede:** `php_debug_adapter/php_debug_adapter.go`: getList (ReadFile + Unmarshal), save (Marshal + WriteFile); Add/Del/Settings bu yapıyı kullanıyor

---

## 6. Platform – Generic JSON storage

- **Ne:** Genel amaçlı JSON dosya okuma/yazma (Load, Save, AppendJSONL, ReadJSONL, RotateJSONL, CleanupOldJSONL)
- **Nerede:** `platform/storage/json_storage.go`
- **Kullanım:** Şu an proje içinde bu paketi import eden başka kod **yok**; kullanılmıyorsa “kullanılmayan altyapı” olarak değerlendirilebilir.

---

## Özet tablo

| Menü / Özellik        | Dosya(lar)                    | Konum(lar)                          |
|-----------------------|-------------------------------|-------------------------------------|
| Dev Environment       | *(memory DB: `dev_envs`)*     | devenv/init.go, entity.go, docker_controller |
| Deployment            | `data/deployment.json`        | deployment/init.go                  |
| Container Settings    | `service-settings.json`       | docker-manager/service_settings.go |
| Virtual Hosts        | `data/starred_vhosts.json`   | docker-manager/virtualhost.go       |
| API Gateway           | `data/api_gateway.json` + block list | api_gateway/gateway.go        |
| PHP XDebug Adapter    | `data/settings.json`         | php_debug_adapter/php_debug_adapter.go |
| (Generic)             | –                             | platform/storage/json_storage.go   |

Bu alanlar memory DB’ye taşınmak istenirse, her biri için ayrı tablo/entity ve `memory.Create/Update/FindAll/Where/Delete` kullanımı planlanabilir.

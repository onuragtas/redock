# Plan: Native Go Docker Orkestrasyonu (docker-compose + bash + harici repo → SDK)

## GÜNCELLEME — Repository modeli (embed'den vazgeçildi)

Stack artık binary'e gömülmüyor. Bunun yerine **paket-yöneticisi benzeri bir repository sistemi**:

- **Repository = docker-compose URL veya local klasör.** compose docker-compose **uyumlu**; biz parse edip `ServiceSpec`'e çeviriyoruz. compose-URL repolarında build context'leri (Dockerfile + COPY ettiği dosyalar) ve bind-mount config dosyaları (`./etc/redis.conf`, `./php74/php.ini` …) compose URL'inin dizinine göre HTTP ile çekilip `<workdir>/repositories/<ad>/` altına cache'lenir.
- **Default repository** = GitHub Pages'te host edilen statik bir compose URL (tıpkı `update.json` gibi). `DefaultRepoComposeURL` sabiti; ilk ihtiyaçta çekilir, binary'e hiçbir şey gömülmez. Ayardan değiştirilebilir.
- **Tekil servis** = repository olmadan, doğrudan Docker Hub imajı + kullanıcının kendi port/env/volume ayarı (`Registry.AddCustomService`, `Build` yasak).
- **Registry** = default repo + kullanıcı repoları + tekil servisler birleşik katalog (öncelik: tekil > sonraki repo > önceki repo).
- Motor (`engine.go`) aynen kalır; build context'i artık `os.DirFS(<repo cache>)`'ten okur (embed yerine).

Yeni dosyalar: `compose.go` (runtime compose→ServiceSpec parser), `repository.go` (Repository/Registry/fetcher). Kaldırılanlar: `embed.go`, `catalog_gen.go`, `_generator/`, vendor'lanmış `stack/`.

---

## Amaç
`onuragtas/docker` reposunun çalışma-zamanı klonlanmasını, `docker-compose` ve bash scriptlerini tamamen kaldırıp; ~58 servisi **Docker Go SDK** (`github.com/docker/docker v27.4.1`, zaten bağımlı) ile redock içinden **native** yönetmek. Sonuçta redock tek başına yeterli olacak: harici repo yok, `docker-compose` binary'si yok, `install.sh`/`serviceip.sh`/`add_virtualhost.sh` yok.

## Mevcut durum (çıkarılan sözleşme)
- `docker-manager/` (manager.go 643, virtualhost.go 347, service_settings.go 349 satır) compose YAML'ı parse edip `docker-compose`/`docker`/`bash`/`git` komutlarını shell'den çağırıyor.
- Ağ: tek bridge `net`, subnet `172.28.0.0/16`, servislere statik IP (`ipv4_address`).
- Build context'li servisler (26 Dockerfile: global, nginx, httpd, rabbitmq, elasticsearch, php56..php74 + xdebug, keycloak, supervisor, redis-commander…) + pull edilenler (php81/84_xdebug = hakanbaysal/*, mongo, postgres, redis, kafka…).
- `.env.example`/`.env` tüm port/IP/şifreleri tutuyor; specs içinde `${VAR}` ile referanslanıyor.
- amd64/arm64 farkları: `platform` pinleme, arm64'te `postgres11`+`pgbouncer` yok, kafka/zookeeper image/isim farkları.
- "Aktif servis" durumu üretilen `docker-compose.yml`'den çıkarılıyor.
- VirtualHost: nginx/httpd conf dosyaları `etc/nginx/` ve `httpd/sites-enabled/`'a yazılıp container restart.
- XDebug: conf dosyalarında `php{x}` ↔ `php{x}_xdebug` regex değişimi + `docker cp xdebug.ini` + restart; ayrıca lokal IP değişince 5 sn'de bir yeniden üretim döngüsü.
- DevEnv: `bash serviceip.sh` ile kişisel SSH container'ları + `docker rm`.
- PHP XDebug Adapter: TCP proxy (bağımsız, dokunmuyoruz).

## Hedef mimari (yeni paket: `docker-manager/native/`)

### 1. `spec` — Servis kataloğu (Go struct)
```go
type ServiceSpec struct {
    Name, ContainerName string
    Image     string        // pull edilecekse
    Build     *BuildSpec    // embed'li build context
    Ports     []PortMapping // host:container (env-ref çözülür)
    Env       map[string]string
    Volumes   []VolumeMount // bind veya named
    StaticIP  string        // net üzerinde ipv4
    Aliases   []string      // links yerine network alias
    DependsOn []string
    Command, Entrypoint []string
    Restart   string
    TTY       bool
    Hostname  string
    Ulimits   []Ulimit
    Platform  string        // "linux/amd64" vb.
    Category  string
}
func Catalog(arch string, env EnvModel) []ServiceSpec
```
Katalog, mevcut `docker-compose.yml.dist`'ten **tek seferlik bir üretici** ile Go'ya dökülür; sonra elle bakım. arch'a göre platform/exclude koşulları.

### 2. `engine` — Docker SDK runtime (`client.Client` sarmalayıcı)
- `EnsureNetwork()` — yoksa `net` bridge'i 172.28.0.0/16 ile oluştur.
- `EnsureVolume(name)`.
- `BuildImage(spec)` — `embed.FS`'ten tar context üretip `ImageBuild`.
- `PullImage(image, platform)` — `ImagePull`.
- `CreateAndStart(spec, env)` — `ContainerCreate` (`container.Config` + `HostConfig`: binds, portbindings, restart, ulimits, tty + `NetworkingConfig`: statik IP `IPAMConfig.IPv4Address`, alias'lar).
- `Stop/Remove/Restart(name)`, `Logs(name)` (stream), `Exec(name, cmd, tty)`, `Status(name)`, `CopyToContainer` (xdebug.ini için).
- Bağımlılık sırası: DependsOn+alias üzerinden topolojik sıralama.

### 3. `envmodel` — `.env` parse + `${VAR}` interpolasyonu
Şimdilik `.env` dosyası UI'dan düzenlenmeye devam; interpolasyon spec'lere uygulanır. (Sonradan memory DB'ye taşınabilir.)

### 4. `contexts` — `go:embed` build context'leri
Dockerfile + küçük destek dosyaları (crontab, php.ini, getip, setup.sh…) redock reposuna **vendor**lanıp gömülür. Build girdileri binary ile gelmek zorunda (clone kalkıyor). Veri dizinleri (data/, logs/, mysql/…) gömülmez — runtime'da oluşturulur.

## Entegrasyon / migrasyon stratejisi (kırmadan)
- Mevcut `DockerEnvironmentManager` (compose+bash) **dokunulmadan** kalır; native motor bir **bayrak/ayar** arkasında (`USE_NATIVE_ORCHESTRATION` veya memory-DB ayarı) eklenir.
- Controller sözleşmesi native motora karşı yeniden yazılır: Up/Down/Restart/Logs/Exec/Status/Add/Remove/UpdateImages/AddVirtualHost/XDebug/DevEnv.
- "Aktif servis" durumu üretilen compose yerine **memory DB** (`active_services` entity)'de tutulur.
- `links` → modern bridge DNS + network **alias** (container_name + service adı).
- VirtualHost conf'ları redock-yönetilen dizine yazılır, nginx/httpd'ye bind-mount edilir; xdebug regex mantığı aynı, restart SDK ile.
- DevEnv: `serviceip.sh` yerine SDK ile `hakanbaysal/devenv` create/remove.
- `git clone` + `install.sh` + `*.sh` tamamen kaldırılır.

## Fazlar
- **Faz 0 — Temel:** `engine` primitive'leri (network/volume/build/pull/create/start/stop/remove/logs/exec/status) + `ServiceSpec` modeli + env interpolasyonu + embed iskeleti. Canlı yola dokunmaz, test edilebilir.
- **Faz 1 — Katalog:** compose'tan tek-seferlik üretici → tam katalog; nginx, php74_xdebug, db(mysql), redis ile build+run doğrulaması.
- **Faz 2 — Lifecycle paritesi:** native Up/Down/Restart/Add/Remove/Status + bağımlılık sırası + aktif-servis durumu memory DB.
- **Faz 3 — Web + XDebug:** virtualhost config dizini + nginx/httpd bind + xdebug add/remove/regenerate (SDK CopyToContainer).
- **Faz 4 — DevEnv:** kişisel container'lar SDK ile.
- **Faz 5 — Cutover:** bayrağı aç, compose/bash/clone yollarını kaldır, frontend metinlerini güncelle.

## Riskler / kararlar
- Statik IP: SDK `EndpointSettings.IPAMConfig.IPv4Address` destekliyor ✓.
- Build context'ler gömülünce repo büyür (sadece Dockerfile + küçük dosyalar; veri yok) — kabul.
- amd64/arm64 + bazı image'larda `platform` pinleme spec'te `Platform` ile.
- `hunspell` servisi `elastic/hunspell` context'ine bakıyor ama dizin yok (yetim tanım) — kataloğa alınmaz.

## Doğrulama
- Her fazda `go build -o redock` + ilgili paket testleri.
- Faz 1–2: gerçek Docker daemon'da seçili servislerin build/up/down/restart/logs/exec paritesi.
- Cutover öncesi: compose yolu ile native yol yan yana aynı sonucu vermeli.

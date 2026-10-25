---
layout: default
title: Redock Meet — WebRTC Entegrasyonu
description: Redock'un görüntülü görüşme sunucusunu (SFU) ve TURN/STUN'unu kendi platformunuza bağlama rehberi
---

# Redock Meet entegrasyonu

Redock; görüntülü görüşme sunucusu (SFU), kendi TURN/STUN sunucusu ve hazır bir katılma ekranıyla gelir. Bu rehber, platformunuzun kendi kullanıcıları için oda açmasını, davet üretmesini ve görüşmeyi kendi uygulamasına yerleştirmesini anlatır.

Örneklerdeki adresler:

- `redock.example.com`: Redock paneli ve API
- `rtc.example.com`: görüşme alan adı
- `turn.example.com`: TURN/STUN adresi

## İçindekiler

1. [Genel akış](#genel-akış)
2. [Başlarken](#başlarken)
3. [API referansı](#api-referansı)
4. [Roller](#roller)
5. [Görüşmeye katılma](#görüşmeye-katılma)
6. [Sadece TURN/STUN](#sadece-turnstun-kendi-p2p-uygulamanız)
7. [Kayıt ve gizlilik](#kayıt-ve-gizlilik)
8. [Güvenlik](#güvenlik)
9. [Hatalar](#hatalar)
10. [Açılması gereken portlar](#açılması-gereken-portlar)

---

## Genel akış

Odalar şifreyle değil, Redock'un imzaladığı kısa ömürlü **token**larla açılır. Token'ı her zaman sizin backend'iniz ister; API anahtarı tarayıcıya ya da mobil uygulamaya hiç gitmez.

1. **Oda oluşturun:** backend'iniz `/rtc/ext/rooms` ile odayı açar (varsa günceller).
2. **Token isteyin:** her kullanıcı için `/rtc/ext/tokens` çağrılır; rol ve süre burada belirlenir.
3. **Kullanıcıyı gönderin:** dönen `join_url` açılır ya da token kendi istemcinize verilir.
4. **Görüşme:** medya SFU üzerinden, gerekiyorsa Redock TURN ile akar.

## Başlarken

Redock yöneticisinin panelde bir kez yapması gerekenler:

1. **Menüyü açın.** WebRTC menüsü varsayılanda gizlidir. Sol menünün altındaki **Menüyü Özelleştir**'den **WebRTC**'yi açın.
2. **WebRTC → Görüntülü görüşme (SFU)** sekmesinde sunucuyu açın ve **Alan adı** alanına görüşme alan adını (ör. `rtc.example.com`) girin. Redock bu alan adı için TLS'li bir API Gateway route'u açar ve Let's Encrypt sertifikası bu alan adını da kapsar. Kamera ve mikrofon yalnızca HTTPS'te çalışır. Alan adının DNS'te bu sunucuya işaret eden bir A/AAAA kaydı olmalıdır; yoksa sertifika alınamaz.
3. **NAT geçişi (TURN)** seçeneğini **Redock TURN** yapın ve **TURN / STUN** sekmesinden TURN sunucusunu açın. NAT ya da güvenlik duvarı arkasındaki kullanıcılar ancak böyle bağlanabilir.
4. Aynı sekmedeki **API anahtarları** bölümünden platformunuz için bir anahtar oluşturun. Anahtar (`rtk_…`) yalnızca bir kez gösterilir; backend'inizin gizli ayarlarına kaydedin.
5. Panelin gösterdiği [portları](#açılması-gereken-portlar) açın.

Tüm API çağrıları `X-API-Key` başlığıyla Redock'un API adresine yapılır:

```bash
curl https://redock.example.com/api/v1/rtc/ext/rooms/demo \
  -H 'X-API-Key: rtk_…'
```

## API referansı

Başarılı yanıtlar `{"error": false, "msg": null, "data": …}`, hatalar `{"error": true, "msg": "…"}` biçimindedir. İstek gövdeleri JSON'dur (`Content-Type: application/json`).

### `POST /api/v1/rtc/ext/rooms`

Odayı oluşturur; aynı adla tekrar çağrılırsa ayarlarını günceller. Her görüşmeden önce güvenle çağırabilirsiniz.

| Alan | Tür | Açıklama |
|------|-----|----------|
| `name` | string, zorunlu | Oda kimliği. Küçük harf, rakam, `-` ve `_`; en fazla 64 karakter. Linklerde görünür; tahmin edilebilir kişisel veri koymayın. `redock-test` ayrılmıştır. |
| `display_name` | string | Katılımcılara gösterilen ad. |
| `description` | string | Serbest açıklama. |
| `max_clients` | int | Eşzamanlı katılımcı sınırı. `0` = sınırsız. |
| `allow_recording` | bool | Moderatörlerin kayıt başlatabilmesi. Varsayılan `false`. |

```bash
curl -X POST https://redock.example.com/api/v1/rtc/ext/rooms \
  -H 'X-API-Key: rtk_…' -H 'Content-Type: application/json' \
  -d '{"name":"order-1234","display_name":"Sipariş #1234","max_clients":4}'
```

Yanıt (`data`):

```json
{
  "name": "order-1234",
  "display_name": "Sipariş #1234",
  "description": "",
  "max_clients": 4,
  "allow_recording": false,
  "created_at": "2026-10-24T10:00:00Z",
  "participants": 0,
  "locked": false,
  "recording": false
}
```

### `POST /api/v1/rtc/ext/tokens`

Bir kullanıcı için odaya giriş token'ı ve hazır katılma linki üretir.

| Alan | Tür | Açıklama |
|------|-----|----------|
| `room` | string, zorunlu | Var olan bir oda. |
| `username` | string | Görüşmede görünecek ad; token'a sabitlenir ve kullanıcı değiştiremez. Boş bırakılırsa linki açan kişi kendi adını yazar. En fazla 64 karakter; `/`, `\` ve `..` kabul edilmez. |
| `role` | string | `presenter` (varsayılan), `viewer`, `observer` ya da `moderator`. Bkz. [Roller](#roller). |
| `ttl_hours` | number | Geçerlilik süresi. Varsayılan 24, en fazla 720 (30 gün). Kesirli olabilir (`0.5` = 30 dk). |

```bash
curl -X POST https://redock.example.com/api/v1/rtc/ext/tokens \
  -H 'X-API-Key: rtk_…' -H 'Content-Type: application/json' \
  -d '{"room":"order-1234","username":"Ayşe Yılmaz","role":"presenter","ttl_hours":2}'
```

Yanıt (`data`):

```json
{
  "room": "order-1234",
  "role": "presenter",
  "username": "Ayşe Yılmaz",
  "token": "eyJhbGciOiJIUzI1NiIs…",
  "join_url": "https://rtc.example.com/redock/join.html#room=order-1234&token=eyJ…",
  "expires_at": "2026-10-24T12:00:00Z"
}
```

Token adresin `#` kısmında taşınır; sunucu loglarına ve `Referer` başlığına düşmez, katılma sayfası açılınca adres çubuğundan da silinir.

### `GET /api/v1/rtc/ext/rooms/:name`

Odanın anlık durumunu döner (`participants`, `locked`, `recording` dahil; biçim yukarıdakiyle aynı). Oda yoksa `404`.

### Node.js örneği

```javascript
const REDOCK = 'https://redock.example.com/api/v1/rtc/ext'
const headers = { 'X-API-Key': process.env.REDOCK_RTC_KEY, 'Content-Type': 'application/json' }

async function call(path, body) {
  const res = await fetch(REDOCK + path, { method: 'POST', headers, body: JSON.stringify(body) })
  const json = await res.json()
  if (json.error) throw new Error(json.msg)
  return json.data
}

// Oturum açmış kullanıcınız için katılma linki
export async function meetingLink(orderId, user) {
  const room = `order-${orderId}`
  await call('/rooms', { name: room, display_name: `Sipariş #${orderId}`, max_clients: 4 })
  const invite = await call('/tokens', {
    room,
    username: user.displayName,
    role: user.isAgent ? 'moderator' : 'presenter',
    ttl_hours: 2,
  })
  return invite.join_url
}
```

## Roller

| Rol | Kamera / mikrofon / ekran | Sohbet | Yönetim |
|-----|---------------------------|---------|---------|
| `presenter` | Evet | Evet | — |
| `viewer` | Hayır, izler | Evet | — |
| `observer` | Hayır, izler | Hayır | — |
| `moderator` | Evet | Evet | Katılımcıyı çıkarma, mikrofonunu kapattırma, odayı kilitleme; odada kayıt izni varsa kaydı başlatma |

Moderatörler odadaki herkese "Moderatör" etiketiyle görünür. Gizli katılım yoktur.

## Görüşmeye katılma

### 1. Hazır katılma sayfası (önerilen)

En kolay yol, `join_url`'i kullanıcıya açmaktır: yeni sekmede, yönlendirmeyle ya da e-postadaki bir linkle. Sayfa kamera önizlemesi, ad girişi, video ızgarası, ekran paylaşımı, sohbet, katılımcı listesi ve moderatör kontrolleriyle gelir; tarayıcı diline göre Türkçe ya da İngilizce açılır.

Sayfayı kendi sitenize gömmek isterseniz iframe'e kamera izinlerini verin:

```html
<iframe src="{{ join_url }}"
        allow="camera; microphone; display-capture; autoplay"
        style="width:100%;height:640px;border:0"></iframe>
```

### 2. Mobil uygulama içinde (WebView)

`join_url`'i bir WebView'da açın ve WebView'ın kamera/mikrofon iznini uygulamanın iznine bağlayın:

- **Android:** `WebChromeClient.onPermissionRequest` içinde `request.grant(request.resources)`; manifest'te `CAMERA` ve `RECORD_AUDIO`; `mediaPlaybackRequiresUserGesture = false`.
- **iOS (WKWebView):** `allowsInlineMediaPlayback = true`, `mediaTypesRequiringUserActionForPlayback = []`; Info.plist'te `NSCameraUsageDescription` ve `NSMicrophoneUsageDescription`.
- **Flutter / React Native:** `webview_flutter` ya da `react-native-webview` ile aynı izinler.

### 3. Kendi web arayüzünüz

Arayüzü tamamen kendiniz çizmek isterseniz, görüşme alan adındaki `protocol.js` kütüphanesini kullanın. Sayfanız görüşme alan adıyla **aynı origin**'de çalışmalıdır (ör. `https://rtc.example.com` altında barındırma ya da reverse proxy).

```html
<script src="https://rtc.example.com/protocol.js"></script>
<script>
async function joinRoom(room, token, name) {
  // 1) Odanın WebSocket adresi
  const status = await (await fetch(`/group/${room}/.status`)).json()

  const sc = new ServerConnection()
  sc.onconnected = () => sc.join(room, name, { type: 'token', token })
  sc.onjoined = async (kind, group, perms) => {
    if (kind === 'fail') return console.error('katılamadı')
    if (kind !== 'join') return
    sc.request({ '': ['audio', 'video'] })          // diğerlerinin yayınlarını al
    if (perms.includes('present')) {                 // kendi kameranı gönder
      const media = await navigator.mediaDevices.getUserMedia({ audio: true, video: true })
      const up = sc.newUpStream()
      up.label = 'camera'
      media.getTracks().forEach(t => up.pc.addTransceiver(t, { direction: 'sendonly', streams: [media] }))
      up.setStream(media)
    }
  }
  sc.ondownstream = (c) => {                         // başka bir katılımcının yayını
    c.ondowntrack = () => showVideo(c.id, c.username, c.stream)
    c.onclose = () => removeVideo(c.id)
  }
  sc.onchat = (id, src, dest, user, time, priv, history, kind, text) => addChat(user, text)
  await sc.connect(status.endpoint)
  return sc  // sc.chat('', '', 'merhaba'), sc.close()
}
</script>
```

Token'da ad sabitlenmişse `name` yok sayılır. TURN sunucusu adresleri bağlantı sırasında sunucudan otomatik gelir; ayrıca ayar gerekmez.

### 4. Native istemci

Herhangi bir WebRTC kütüphanesi (Android/iOS resmi kütüphaneleri, `flutter_webrtc`, `react-native-webrtc`) görüşme sunucusunun WebSocket protokolüyle konuşabilir: `wss://rtc.example.com/ws`, JSON mesajları, `join` mesajında `token`. Protokolün tamamı Galène'in belgesinde: [galene-protocol.md](https://github.com/jech/galene/blob/master/galene-protocol.md). Redock token'ları bu protokolün kriptografik token'larıdır.

## Sadece TURN/STUN (kendi P2P uygulamanız)

Kendi WebRTC uygulamanız eşler arası (P2P) çalışıyorsa, Redock'u yalnızca relay olarak kullanabilirsiniz. Yönetici **WebRTC → TURN / STUN** sekmesinde sizin için bir **uygulama** oluşturur ve size uygulama adını ve secret'ını verir. Kimlik bilgilerini backend'inizde imzalarsınız (TURN REST API standardı):

- **Kullanıcı adı:** `<bitiş unix zamanı>:<uygulama>[:<kullanıcı>]`
- **Şifre:** `base64(HMAC-SHA1(secret, kullanıcı adı))`

Node.js:

```javascript
import crypto from 'node:crypto'

export function turnCredentials(app, secret, userId, ttlSeconds = 3600) {
  const username = `${Math.floor(Date.now() / 1000) + ttlSeconds}:${app}:${userId}`
  const credential = crypto.createHmac('sha1', secret).update(username).digest('base64')
  return {
    urls: [
      'stun:turn.example.com:3478',
      'turn:turn.example.com:3478?transport=udp',
      'turn:turn.example.com:3478?transport=tcp',
      'turns:turn.example.com:5349?transport=tcp',
    ],
    username,
    credential,
  }
}
// tarayıcıda: new RTCPeerConnection({ iceServers: [turnCredentials(...)] })
```

Go:

```go
func TurnCredentials(app, secret, user string, ttl time.Duration) (username, credential string) {
	username = fmt.Sprintf("%d:%s:%s", time.Now().Add(ttl).Unix(), app, user)
	mac := hmac.New(sha1.New, []byte(secret))
	mac.Write([]byte(username))
	return username, base64.StdEncoding.EncodeToString(mac.Sum(nil))
}
```

Python:

```python
import base64, hashlib, hmac, time

def turn_credentials(app, secret, user, ttl=3600):
    username = f"{int(time.time()) + ttl}:{app}:{user}"
    digest = hmac.new(secret.encode(), username.encode(), hashlib.sha1).digest()
    return username, base64.b64encode(digest).decode()
```

Tam adres listesi, TURN / STUN sekmesinin üstünde "İstemcilerin kullanacağı ICE adresleri" olarak görünür.

> **Sunucunun koyduğu sınırlar.** Relay varsayılan olarak iç ağlara (LAN, localhost, Docker, CGNAT) bağlanmaz; IP başına ve uygulama başına eşzamanlı oturum sınırı ve oturum başına bant genişliği sınırı vardır; art arda hatalı giriş yapan IP geçici olarak engellenir. Yönetici bir oturumu sonlandırırsa o kimlik bilgisi süresi dolana kadar iptal edilir.

## Kayıt ve gizlilik

- Kayıt yalnızca `allow_recording: true` olan odalarda, moderatör ya da Redock yöneticisi tarafından başlatılabilir.
- Kayıt sürerken odada "RECORDING" adlı bir katılımcı görünür ve katılma sayfası herkese "Bu görüşme kaydediliyor" uyarısı gösterir. Habersiz kayıt mümkün değildir.
- Her katılımcı ayrı bir dosyaya kaydedilir (görüntülü: WebM, yalnızca ses: Ogg); birleşik tek video üretilmez.
- Kayıtlar yalnızca Redock panelinden, 5 dakika geçerli imzalı linklerle indirilir; her indirme denetim kaydına yazılır. Yöneticinin belirlediği süreden eski kayıtlar otomatik silinir.
- Kayıt alıyorsanız katılımcılarınızı kullanım koşullarınızda ve gerektiğinde açık rızayla bilgilendirmek sizin sorumluluğunuzdadır (KVKK).

## Güvenlik

- **API anahtarı ve TURN secret'ı yalnızca backend'de** tutulur. İstemciye sadece token ya da kısa ömürlü TURN kimlik bilgisi gider.
- **Token bir giriş anahtarıdır.** Linki alan herkes, süresi dolana kadar o rolle odaya girebilir. Süreleri görüşmenin uzunluğuna yakın tutun; adı sabitlemek istediğinizde `username` verin.
- **Oda silmek** o odaya verilmiş tüm token'ları anında geçersiz kılar ve içindekileri çıkarır.
- **Anahtar sızarsa** yönetici panelden silip yenisini oluşturur; TURN secret'ı da tek tıkla yenilenir (eski kimlik bilgileri hemen çalışmaz).
- **Her işlem kayıt altındadır:** davetler (hangi API anahtarıyla), moderatör girişleri, çıkarmalar, kilitler ve kayıt işlemleri Redock'un denetim kaydına yazılır.
- **HTTPS zorunludur:** tarayıcılar kamera ve mikrofonu yalnızca güvenli bağlantıda açar.

## Hatalar

| HTTP | Ne zaman | Ne yapmalı |
|------|----------|------------|
| `401` | `X-API-Key` yok ya da geçersiz. | Anahtarı kontrol edin; silinmişse yöneticiden yenisini isteyin. |
| `400` | Geçersiz alan: oda adı biçimi, bilinmeyen rol, 720 saati aşan süre, geçersiz kullanıcı adı, olmayan oda için token. | `msg` alanı sorunu söyler; isteği düzeltip tekrar gönderin. |
| `404` | `GET /rooms/:name` için oda yok. | Önce `POST /rooms` ile oluşturun. |
| `503` | Görüşme sunucusu henüz başlatılmamış. | Yöneticiden SFU'yu açmasını isteyin. |

Katılma sırasında görülebilecek durumlar: davetin süresi dolmuş, oda kilitli (yeni katılım alınmıyor), oda dolu (`max_clients`) ya da kullanıcı bir moderatör tarafından çıkarılmış. Katılma sayfası bunları kullanıcıya açıklar; kendi istemcinizde `onjoined`'un `'fail'` durumunda `message`/`error` alanlarına bakın.

## Açılması gereken portlar

Panel, bu tabloyu o anki ayarlardan hesaplayıp **WebRTC** sayfasında gösterir (sunucunun gerçek IP'si ve portlarıyla, kopyalanabilir `ufw` komutlarıyla birlikte). Aşağıdaki değerler varsayılanlardır; `192.168.1.20` örnek sunucu IP'sidir.

| Kaynak | Hedef | Protokol | Açıklama |
|--------|-------|----------|----------|
| `Herhangi:443` | `192.168.1.20:443` | TCP | Katılma sayfası ve sinyalleşme (HTTPS/WSS) — zorunlu |
| `Herhangi:80` | `192.168.1.20:80` | TCP | Let's Encrypt doğrulaması ve HTTPS'e yönlendirme — zorunlu |
| `Herhangi:40000-40199` | `192.168.1.20:40000-40199` | UDP | Görüşme sunucusunun ses/görüntü trafiği — zorunlu |
| `Herhangi:3478` | `192.168.1.20:3478` | TCP+UDP | STUN/TURN — önerilen |
| `Herhangi:5349` | `192.168.1.20:5349` | TCP | TURNS: yalnızca TLS'e izin veren kısıtlı ağlar — önerilen |
| `Herhangi:49160-49359` | `192.168.1.20:49160-49359` | UDP | TURN relay portları (oturum başına bir port) — önerilen |

API Gateway sunucuda farklı portlarda dinliyorsa (ör. `8080`/`8443`), hedef portlar ona göre değişir: `Herhangi:443 → 192.168.1.20:8443`, `Herhangi:80 → 192.168.1.20:8080`. Panel bunu otomatik gösterir.

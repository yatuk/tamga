# Tamga: kurulum ve işletim kılavuzu

Bu kılavuz, Tamga'yı kendi sunucusuna kuracak ve çalışır halde tutacak sistem
yöneticisi için yazıldı. Tek sunucuda Docker Compose ile kurulumu anlatır;
Kubernetes için sondaki kısa bölüme bakın. Komutlar deponun kök dizininden
çalıştırılır. Kılavuz 2026-10-11 tarihinde `main` dalına (v0.9.0,
yayımlanmamış) göre yazıldı. Ayrıntının tamamı İngilizce
[işletim belgesinde](../operations.md); burada kurulum ve günlük işletim
için gerekenler var.

Bazı terimleri sahada kullanıldığı gibi İngilizce bıraktık: **proxy** (vekil
sunucu: uygulama ile model sağlayıcı arasında duran servis), **policy** (kural
dosyası: neyin engelleneceğini, neyin maskeleneceğini söyleyen YAML dosyası),
**container** (kapsayıcı), **endpoint** (uç nokta), **console** (yönetim
arayüzü). Console'un kendi dili İngilizce olduğu için menü ve düğme adlarını
da ekranda göründüğü gibi yazdık.

## Tamga nedir, ne değildir

Tamga, uygulamalarınızın çağırdığı dil modeli API'lerinin (OpenAI, Anthropic
ve diğerleri) önüne koyduğunuz, kendi sunucunuzda çalışan bir proxy'dir.
Uygulama yalnızca adres değiştirir: sağlayıcının adresi yerine Tamga'nın
adresini çağırır. Tamga her isteği okur; içinde T.C. kimlik numarası, IBAN,
kart numarası gibi kişisel veri, API anahtarı gibi gizli bilgi ya da modele
yönelik gizlenmiş talimat (prompt injection) arar. Policy bulguya göre karar
verir: isteği engeller (`BLOCK`), bulunan değeri maskeleyip gönderir
(`REDACT`), yalnızca uyarır (`WARN`) ya da geçirir (`PASS`). Her karar kendi
PostgreSQL veritabanınıza yazılır; istek ve yanıt metinleri yazılmaz.

Tamga bir güvenlik duvarı, ağ DLP'si ya da genel amaçlı bir LLM ağ geçidi
değildir. Yalnızca kendisine gönderilen istekleri görür: birinin web
tarayıcısında bir sohbet sitesine yazdığı metni, sağlayıcıyı doğrudan
çağırmaya devam eden bir uygulamayı ya da bir ajan ile MCP sunucusu
arasındaki trafiği görmez. Görselleri, ses dosyalarını ve PDF'leri okumadan
iletir. Tespit kural tabanlıdır: kuralların öngörmediği bir ifade
yakalanmayabilir, ölçülen başarı oranları da projenin kendi yazdığı örnek
kümelerine aittir ([benchmarks](../benchmarks/README.md)). Proje tek kişi
tarafından sürdürülen açık kaynak bir yazılımdır (AGPL-3.0); ticari sürümü,
destek sözleşmesi ya da yönetilen bir hizmeti yoktur. Sınırların tamamı
[README](../../README.md#project-status) ve
[tehdit modeli](../threat-model.md) belgelerindedir.

## Gereksinimler

- Docker Engine ve Compose v2 (`docker compose` komutu) kurulu bir Linux
  sunucu.
- `git`, `openssl` ve `curl`.
- Kurulum sırasında internet erişimi: imajlar sunucuda derlenir; derleme
  Docker Hub, PyPI, npm ve GitHub'dan paket indirir. Yayımlanmış hazır imaj
  yoktur.
- Sunucudan model sağlayıcılarına (ör. `api.openai.com`) çıkış izni.
- Açılan portlar: `8443` (proxy ve yönetim API'si) ve `3000` (console).
  Jaeger arayüzü (`16686`) ve NATS izleme portu (`8222`) yalnızca
  `127.0.0.1` adresine bağlanır. PostgreSQL, Redis, NATS istemci portu,
  analyzer, classifier ve scanner-service sunucuya port açmaz.

Compose dosyasındaki her servis bir profile bağlıdır. `full` profili şu
servisleri başlatır: `proxy`, `dashboard` (console), `analyzer` (PDF
raporları ve isteğe bağlı derin analiz), `scanner-service` (isteğe bağlı;
varsayılan olarak boşta bekler), `postgres`, `redis`, `nats` ve `jaeger`.
İsteğe bağlı `classifier` servisi ayrı bir profille başlar ve bir model
dosyası ister ([Inline classifier](../operations.md#inline-classifier)).
`minimal` profili yalnızca proxy'yi başlatır (aşağıda).

## Yalnızca proxy ile deneme

Veritabanı ve diğer servisler olmadan, tek container ile:

```bash
git clone https://github.com/yatuk/tamga.git
cd tamga
cp .env.example .env    # sağlayıcı anahtarı olmadan denemek için TAMGA_MOCK_UPSTREAM=true
docker compose --env-file .env -f deploy/docker-compose.yml --profile minimal up -d
```

Bu, 8443 portunda `proxy-minimal` adlı tek bir container başlatır. Tarama,
policy ve engelleme tam kurulumdaki gibi çalışır;
[ilk çalıştırmadaki](#ilk-çalıştırma-ve-sağlık-kontrolü) T.C. kimlik
numaralı istek 403 alır. Şunlar yoktur:

- İstek kaydı (request log) yoktur; hiçbir istek bir yere yazılmaz. Yönetim
  API'si (`TAMGA_ADMIN_KEY` ile) yalnızca bu sürecin gördüğü son 1000 isteği
  gösterir; yeniden başlatınca silinir.
- Console yoktur.
- API anahtarları, özel desenler, roller, SSO ayarları, uyarı hedefleri,
  olaylar ve denetim kaydı bellekte tutulur ve yeniden başlatınca kaybolur.
- Analyzer yoktur: PDF raporları 503 `analyzer_unavailable` döner.
- Rate limit, bütçe ve önbellek tek sürecin belleğindedir.

KVKK ve BDDK açısından iz gereken bir kurulum için bu yeterli değildir; tam
kurulumu kullanın. Tam kuruluma geçmeden önce bu container'ı durdurun, iki
proxy de 8443 portunu kullanır:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml --profile minimal down
```

Ayrıntı: [operations.md, Proxy only](../operations.md#proxy-only).

## Docker Compose ile kurulum

### 1. Depoyu alın

```bash
git clone https://github.com/yatuk/tamga.git
cd tamga
```

Bugün etiketlenmiş bir sürüm yoktur; `main` dalını kullanırsınız. İmzalı bir
sürüm yayımlandığında nasıl doğrulanacağı
[verifying-a-release.md](../verifying-a-release.md) belgesindedir.

### 2. `.env` dosyasını hazırlayın

```bash
cp .env.example .env
chmod 600 .env
```

Üç değer zorunludur ve örnek dosyada boştur. Tam kurulumu başlatmadan önce
her biri için ayrı bir değer üretin:

```bash
openssl rand -hex 32   # TAMGA_ADMIN_KEY için
openssl rand -hex 32   # POSTGRES_PASSWORD için
openssl rand -hex 32   # REDIS_PASSWORD için
```

Veritabanı ve Redis parolaları bağlantı adresinin içine yazıldığı için
`-hex` kullanın; `-base64` çıktısındaki `/`, `+` ve `=` karakterleri adresi
bozar.

| Değişken | Ne için | Ne yazılmalı |
|---|---|---|
| `TAMGA_ADMIN_KEY` | Yönetim API'sinin ve console'un yönetici anahtarı. Zorunlu | `openssl rand -hex 32` |
| `POSTGRES_PASSWORD` | Veritabanı parolası. Zorunlu; boşsa PostgreSQL başlamaz | `openssl rand -hex 32` |
| `REDIS_PASSWORD` | Redis parolası. Zorunlu | `openssl rand -hex 32` |
| `TAMGA_MOCK_UPSTREAM` | `true` iken sağlayıcı çağrılmaz, hazır bir yanıt döner; tarama ve policy yine çalışır. Sağlayıcı anahtarı olmadan denemek için | Deneme: `true`. Üretim: `false` |
| `NEXT_PUBLIC_API_URL` | Kullanıcının **web tarayıcısının** proxy'ye ulaştığı adres. Console imajının içine derlenir | Console'a başka bir makineden girilecekse `https://tamga.kurum.example:8443` gibi gerçek adres. Değiştirince imajı yeniden derleyin (aşağıda) |
| `TAMGA_CORS_ORIGINS` | Yönetim API'sini çağırmasına izin verilen console adresleri, virgülle ayrılmış, birebir eşleşir | Console'un adresi, ör. `https://tamga.kurum.example:3000` |
| `TAMGA_FINDING_HASH_KEY` | Policy'de `data.hash_findings` açıksa bulguların anahtarlı özetinde kullanılan anahtar | `openssl rand -base64 32`. Bkz. [Bulguların saklanması](#bulguların-saklanması) |
| `TAMGA_POLICY_MOUNT` | Policy dosyasının container'a nasıl bağlandığı | Boş bırakılırsa `ro` (salt okunur). Console'dan düzenlemek için `rw` |
| `TAMGA_JWT_SECRET` ve `TAMGA_GITHUB_*` | Oturum açma (yalnızca GitHub OAuth). İsteğe bağlı | Kullanmayacaksanız boş bırakın; o zaman erişim yalnızca anahtarlarla olur |
| `TAMGA_ANALYZER_SEND_CONTENT` | İletilen isteklerin metnini karar sonrası analyzer'a kopyalar | `false` bırakın; ne yaptığını [işletim belgesinden](../operations.md#analyzer) okumadan açmayın |
| `TAMGA_LLM_JUDGE`, `ANTHROPIC_API_KEY` | Analyzer'ın metni Anthropic'e gönderen "LLM judge" özelliği | Boş bırakın. İkisi birlikte verilmedikçe kapalıdır |
| `OPENAI_API_KEY` | Analyzer bunu okur ama kullanmaz; proxy'nin sağlayıcı havuzları da aynı env dosyasından okuyabilir | Kullanılmıyorsa boş bırakın |

Normal geçişte sağlayıcı anahtarlarını (OpenAI, Anthropic) Tamga'ya
vermezsiniz: uygulama kendi anahtarını her istekte gönderir, Tamga onu
sağlayıcıya iletir. Policy'deki bir sağlayıcı havuzu `api_key_env` kullanırsa
adı verilen anahtarın proxy ortamında bulunması bunun istisnasıdır.

Compose, `.env` dosyasını proxy container'ının ortamı olarak da yükler; bu
yüzden `TAMGA_REQUIRE_KEY`, `TAMGA_TRUSTED_PROXIES` ve
`TAMGA_RETENTION_REQUEST_LOGS_DAYS` gibi proxy ayarları doğrudan ulaşır.
Başka bir `--env-file` kullanırsanız, o dosyada `TAMGA_PROXY_ENV_FILE`
değerini de aynı dosyanın `deploy/` dizinine göre yolu olarak verin. Dosya
yolları (`TAMGA_TLS_CERT`, operator-state dosyaları gibi) container içindeki
bir yolu göstermeli ve ilgili dosya ayrı bir volume ile bağlanmalıdır.

### 3. Policy dosyasının bağlanması

Proxy, depodaki `proxy/tamga-policy.yaml` dosyasını kullanır. Dosya
container'a varsayılan olarak **salt okunur** bağlanır: console policy'yi
gösterir ama değiştiremez; kaydetme isteği 409 `policy_read_only` ile
reddedilir. Bu, policy'yi yalnızca sunucudaki dosyayı değiştirebilen kişinin
değiştirmesini istediğiniz durum için güvenli varsayılandır.

Console'daki **Save and apply** düğmesinin dosyayı yazabilmesini
istiyorsanız `.env` dosyasına şunu ekleyin:

```bash
TAMGA_POLICY_MOUNT=rw
```

### 4. Başlatın

```bash
docker compose --env-file .env -f deploy/docker-compose.yml up -d
```

`.env.example` dosyası `COMPOSE_PROFILES=full` içerir; bu yüzden profil
verilmeyen her komut tam kurulumu kullanır. `--profile full` aynı işi görür.
0.9.0'dan önce hazırlanmış bir `.env` dosyasında bu satır yoktur ve komut
`no service selected` hatasıyla durur; satırı ekleyin.

`--env-file .env` şarttır. Compose, `.env` dosyasını kendiliğinden yalnızca
Compose dosyasının bulunduğu dizinde (`deploy/`) arar; bu parametre olmadan
veritabanı parolası boş kalır, PostgreSQL başlamaz ve `COMPOSE_PROFILES`
okunmaz.

İlk çalıştırma imajları derlediği için uzun sürer. Veritabanı şeması,
`deploy/migrations/` altındaki dosyalarla, boş veri biriminde ilk açılışta
kendiliğinden kurulur.

`NEXT_PUBLIC_API_URL` değerini sonradan değiştirirseniz console imajını
yeniden derleyin:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml build dashboard
docker compose --env-file .env -f deploy/docker-compose.yml up -d dashboard
```

### Ek ayarlar

Ek bir volume, container içi dosya yolu ya da başka bir Compose özelliği
için depodaki dosyayı değiştirmek yerine yanına ikinci bir dosya koyun, ör.
`deploy/docker-compose.local.yml`:

```yaml
services:
  proxy:
    volumes:
      - ../secrets/tls:/run/tamga/tls:ro
```

ve her `docker compose` komutuna ikisini birlikte verin:

```bash
docker compose --env-file .env \
  -f deploy/docker-compose.yml -f deploy/docker-compose.local.yml up -d
```

Bu kılavuzdaki diğer komutlarda da, böyle bir dosyanız varsa, ikinci `-f`
parametresini ekleyin. Değişkenlerin tam listesi:
[operations.md, Configuration](../operations.md#configuration).

**TLS.** Compose kurulumu 8443 ve 3000 portlarında düz HTTP sunar. Üretimde
önüne TLS sonlandıran bir ters vekil (nginx, HAProxy, bir yük dengeleyici)
koyun ya da proxy'ye `TAMGA_TLS_CERT` ve `TAMGA_TLS_KEY` ile sertifika verin
(dosyaları container'a ayrıca bağlamanız gerekir).
Önünde ters vekil varsa adresini `TAMGA_TRUSTED_PROXIES` ile bildirin; aksi
halde tüm çağrılar tek bir istemci adresinden geliyor görünür.

## İlk çalıştırma ve sağlık kontrolü

```bash
docker compose --env-file .env -f deploy/docker-compose.yml ps
curl -s http://localhost:8443/health
```

İkinci komutun yanıtında `"status":"ok"` görmelisiniz. Bu uç nokta anahtar
istemez ve yalnızca proxy'nin ayakta olduğunu söyler. Veritabanı, Redis ve
tarama bileşenlerinin durumu için yönetici anahtarıyla sorun:

```bash
export TAMGA_ADMIN_KEY="$(grep '^TAMGA_ADMIN_KEY=' .env | cut -d= -f2-)"

curl -s -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" \
  http://localhost:8443/api/v1/health/detailed
```

Yanıtta `"database":"connected"` ve `"redis":"connected"` olmalıdır. Yanlış
bir anahtarla bu uç nokta 401 dönmez, kısa yanıtı 200 ile verir; yanıttaki
`authenticated` alanına bakın.

Şimdi içinde T.C. kimlik numarası olan bir istek gönderin. Varsayılan policy
bunu sağlayıcıya ulaşmadan engeller (`10000000146`, sağlama basamağı geçerli
bir deneme numarasıdır):

```bash
curl -s -o /dev/null -w "%{http_code}\n" \
  -X POST http://localhost:8443/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Müşteri TC 10000000146"}]}'
# 403
```

Console `http://localhost:3000` adresindedir. Console anahtar tutmaz: ilk
girişte **Settings** sayfasında `TAMGA_ADMIN_KEY` değerini sorar ve onu o web
tarayıcısında saklar. Az önceki engellenen istek **Incidents** sayfasında
görünür.

## Uygulamayı proxy'ye yönlendirme

Uygulamada değişen tek şey temel adrestir (base URL). Sağlayıcı anahtarı
uygulamada kalır.

| Sağlayıcı | Eski adres | Yeni adres |
|---|---|---|
| OpenAI | `https://api.openai.com/v1` | `http://<tamga-adresi>:8443/v1` |
| Anthropic | `https://api.anthropic.com` | `http://<tamga-adresi>:8443/anthropic` |

Diğer yollar: `/gemini`, `/azure`, `/bedrock`, `/mistral`, `/local`.
Varsayılan policy OpenAI, Anthropic, Azure ve Gemini'ye izin verir; bunlardan
yalnızca OpenAI, Anthropic ve Gemini yolları uçtan uca testlidir.

OpenAI Python istemcisi:

```python
import os
from openai import OpenAI

client = OpenAI(
    base_url="http://localhost:8443/v1",
    api_key=os.environ["OPENAI_API_KEY"],                      # sağlayıcıya gider
    default_headers={"X-Tamga-Key": os.environ["TAMGA_KEY"]},  # Tamga'da kalır
)
```

Anthropic istemcileri ve Claude Code için ortam değişkeni yeterlidir:

```bash
export ANTHROPIC_BASE_URL="http://localhost:8443/anthropic"
```

`X-Tamga-Key` başlığı, uygulamanın Tamga'daki kimliğidir (aşağıdaki bölüm).
Başlık gönderilmezse istek varsayılan ayarda yine kabul edilir ve "anonim"
sayılır. Ajanlar ve Claude Code için ayrıntı: [agents.md](../agents.md).

## Anahtarlar ve roller

Tamga'da iki tür anahtar vardır, karıştırmayın:

- **Uygulama anahtarı** (`X-Tamga-Key` başlığı): bir uygulamanın proxy
  üzerinden istek göndermesi içindir. Yönetim API'sinde hiçbir şeyi açmaz.
- **Yönetim anahtarı** (`X-Tamga-Admin-Key` başlığı): console'a ve yönetim
  API'sine erişim içindir. `.env` dosyasındaki `TAMGA_ADMIN_KEY` bunların
  ilkidir ve tam yetkilidir.

Anahtarları console'da **API keys** sayfasındaki **Create key** ile
oluşturursunuz. Kapsam (scope) rolü belirler:

| Scope (console'daki adı) | Kimin için | Rol |
|---|---|---|
| `proxy` (Application) | Bir uygulama ya da servis | Yalnızca proxy yolu ve `POST /api/v1/scan` |
| `read` (Read) | İzleyen kişi | viewer: özet, olaylar, policy ve raporları okur |
| `write` (Write) | Analist | analyst: ayrıca olayları sınıflandırır, policy'nin açık bölümlerini düzenler, denetim kaydını okur |
| `admin` (Admin) | Yönetici | admin: ayrıca anahtarlar, uyarı hedefleri, roller, kişisel veri sorguları |

Aynı işlem API ile:

```bash
curl -s -X POST http://localhost:8443/api/v1/apikeys \
  -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" -H "Content-Type: application/json" \
  -d '{"label":"fatura-servisi","scope":"proxy","expires_in_days":90}'
```

Anahtarın değeri yanıtta `raw_key` alanındadır ve **yalnızca bir kez**
gösterilir; Tamga yalnızca özetini (SHA-256) saklar.

Öneriler:

- Her uygulamaya ve her kişiye ayrı anahtar verin. `TAMGA_ADMIN_KEY` tek bir
  kimliktir: onu birden çok kişi kullanırsa denetim kaydı kimin ne yaptığını
  ayırt edemez. Günlük işte kişisel `read`, `write` ya da `admin` anahtarları
  kullanın; kişi anahtarını console'un **Settings** sayfasına girer.
- Tüm uygulamalara anahtar dağıttıktan sonra `TAMGA_REQUIRE_KEY=true` yapın
  ([Ek ayarlar](#ek-ayarlar)). O zamana kadar 8443 portuna ulaşabilen herkes
  istek gönderebilir; proxy'yi yalnızca güvendiğiniz uygulamaların
  erişebildiği bir ağda tutun.
- Bir anahtarı iptal ettiğinizde bu, diğer kopyalara (replica) 30 saniye
  içinde ulaşır.

**Team** sayfasındaki roller, oturum açan hesaplar içindir; oturum açma
bugün yalnızca GitHub OAuth ile vardır. Kullanmıyorsanız roller yukarıdaki
anahtar kapsamlarıyla verilir. Ayrıntı:
[Who may call what](../operations.md#who-may-call-what),
[Keys for applications](../operations.md#keys-for-applications).

## İlk policy

Policy tek bir YAML dosyasıdır: `proxy/tamga-policy.yaml`. Sıfırdan yazmayın;
depodaki dosyadan başlayıp kuralları değiştirin. Kişisel veriyle ilgili iki
kuralın sadeleştirilmiş hali şöyledir (depodaki dosyada tür listesi daha
uzundur ve her kuralın bir `notify` bölümü vardır):

```yaml
rules:
  # Kritik kimlikler: istek sağlayıcıya hiç gitmez.
  pii:
    action: BLOCK
    sensitivity: medium
    types: [tc_kimlik, credit_card, ssn]

  # İletişim türü veriler: değer maskelenir, istek gönderilir.
  pii_detection:
    action: REDACT
    sensitivity: medium
    types: [iban, email, phone_tr, phone, vkn, ip_public]
```

Bu haliyle T.C. kimlik numarası (`tc_kimlik`) ve kart numarası
(`credit_card`) içeren istek 403 ile **engellenir**; IBAN (`iban`), e-posta
ve telefon **maskelenir**, yani model `TR…` yerine bir yer tutucu görür ve
istek yoluna devam eder. IBAN içeren isteklerin de engellenmesini
istiyorsanız `iban` değerini `pii_detection` listesinden çıkarıp `pii`
listesine ekleyin. Bir bulgu iki kurala da uyuyorsa güçlü olan işlem
(`BLOCK`) kazanır.

Bilmeniz gerekenler:

- Bilinmeyen ya da yanlış yere yazılmış bir anahtar policy'nin
  yüklenmesini durdurur; sessizce yok sayılmaz.
- `REDACT` yalnızca metinde yeri olan bulgular için geçerlidir. Prompt
  injection gibi maskelenecek bir değeri olmayan bulgu için `REDACT` yazılmış
  policy yüklenmez.
- Yanıtların taranması (`output_rules`) depodaki dosyada kapalıdır.
- Kendi kalıplarınızı (müşteri numarası, dosya numarası) console'daki
  **Patterns** sayfasından ya da policy'nin `custom_entities` bölümünden
  ekleyebilirsiniz.
- `proxy/policies/templates/` dizininde başlangıç için hazır policy
  örnekleri vardır (`kvkk-baseline.yaml`, `bddk-bank.yaml`,
  `public-sector.yaml`, `software-team.yaml`). Bir şablon bir başlangıç
  noktasıdır, bir uyum belgesi değildir: kendi trafiğinizle deneyip
  uyarlayın. Her şablonun neyi kapsadığı ve neyi kapsamadığı:
  [policy-templates.md](../policy-templates.md).

Değişikliği uygulamadan önce doğrulayın, sonra yükleyin:

```bash
curl -s -X POST http://localhost:8443/api/v1/policies/validate \
  -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" -H "Content-Type: text/yaml" \
  --data-binary @proxy/tamga-policy.yaml

curl -s -X POST http://localhost:8443/api/v1/policies/reload \
  -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY"
```

Proxy dosyadaki değişikliği kendisi de izler. Bazı metin düzenleyiciler
kaydederken dosyayı yenisiyle değiştirir; tek dosya olarak bağlanmış bir
policy'de container bu durumda eski içeriği görmeye devam edebilir.
Değişiklik yansımadıysa proxy'yi yeniden başlatın:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml restart proxy
```

Bir metnin policy'den nasıl geçeceğini, hiçbir yere göndermeden deneyin:

```bash
curl -s -X POST http://localhost:8443/api/v1/scan \
  -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" -H "Content-Type: application/json" \
  -d '{"text":"IBAN: TR330006100519786457841326","role":"user"}'
```

Console'daki **Playground** sayfası aynı işi yapar.

Policy'yi kimin değiştirebileceği ve iki kişilik onay
(`governance.require_dual_control`):
[Who may change the policy](../operations.md#who-may-change-the-policy).

## Bulguların saklanması

Tamga istek ve yanıt metnini veritabanına yazmaz. Yazdığı şey isteğin üst
verisi (sağlayıcı, model, karar, süre) ve bulgulardır. Bir bulgu, değerin
kendisiyle değil maskelenmiş haliyle saklanır: T.C. kimlik numarası için ilk
ve son iki hane (`10*******46`), kart için yalnızca son dört hane.

Maskelenmiş halin de kayıtta durmasını istemiyorsanız policy'ye şunu ekleyin:

```yaml
data:
  hash_findings: true
```

ve `.env` dosyasında bir anahtar tanımlayın:

```bash
openssl rand -base64 32   # çıktıyı TAMGA_FINDING_HASH_KEY= satırına yazın
```

Bu durumda maskelenmiş değerin yerine anahtarlı özeti (`hmac-sha256:…`)
saklanır. Aynı değer aynı özeti verir, yani bir kişinin kayıtları yine
bulunabilir; anahtar olmadan özet hesaplanamaz ve olası değerler denenerek
geri çözülemez.

- Seçenek açık ama anahtar yoksa bulguların eşleşen değeri **hiç
  saklanmaz**; kayıtta `[not stored: no finding hash key]` yazar ve proxy
  bunu açılışta bildirir.
- Anahtarı kaybetmeyin ve değiştirmeyin: başka bir anahtarla üretilmiş
  özetler artık bulunamaz. Birden çok proxy kopyası varsa hepsinde aynı
  olmalıdır. `.env` dosyasını yedeklerken bunu hesaba katın.
- Seçenek açıkken console'da maskelenmiş değer yerine özet görünür ve
  maskelenmiş değerle arama sonuç vermez.

Ayrıntı: [Hashed findings](../operations.md#hashed-findings).

## Saklama süresi

Compose kurulumunda saklama işi açıktır (`TAMGA_RETENTION_ENABLED=true`) ve
günde bir kez çalışır. Varsayılan süreler:

| Ne | Değişken | Varsayılan |
|---|---|---|
| İstek kayıtları | `TAMGA_RETENTION_REQUEST_LOGS_DAYS` | 30 gün |
| Denetim kaydı (audit log) | `TAMGA_RETENTION_AUDIT_DAYS` | 365 gün |
| Kapatılmış olaylar | `TAMGA_RETENTION_INCIDENT_CLOSED_DAYS` | 0 (silinmez) |

Süreleri değiştirmek için değişkenleri [Ek ayarlar](#ek-ayarlar) bölümündeki
gibi proxy'ye verin. İstek kayıtları aylık bölümler halinde silindiği için
bir satır, süresinden en çok bir ay fazla kalabilir. Policy'deki
`data.retention_days` anahtarının etkisi yoktur. Bu iş yalnızca Tamga'nın
kendi kayıtlarını siler; sağlayıcının sakladığına dokunamaz.

Bir kişinin kayıtlarını sorgulamak ve silmek için (yalnızca admin):
`POST` ve `DELETE /api/v1/events/subject`. Silme kalıcıdır; önce sorgulayıp
sonucu okuyun ([kvkk.md](../compliance/kvkk.md)).

## Yedekleme ve geri yükleme

Yedeklenmesi gerekenler: PostgreSQL veritabanı, `proxy/tamga-policy.yaml` ve
`.env` dosyası (içinde anahtarlar olduğu için şifreli ve ayrı bir yerde).
Redis (hız sınırı ve önbellek durumu) ve NATS akışı yedeklenmez; kaybı
kalıcı kayıt kaybı değildir.

Compose kurulumunda PostgreSQL sunucuya port açmaz, bu yüzden yedeği
container'ın içinden alın:

```bash
docker compose --env-file .env -f deploy/docker-compose.yml exec -T postgres \
  pg_dump -U tamga -d tamga --format=custom > "tamga-$(date +%Y%m%d-%H%M%S).dump"
```

(`POSTGRES_USER` ya da `POSTGRES_DB` değerini değiştirdiyseniz `tamga`
yerine onları yazın.) Dosya `pg_dump` özel biçimindedir; içeriğini
`pg_restore --list` ile görebilirsiniz.

Geri yükleme **hedef veritabanını siler**. Önce veritabanına bağlanan
servisleri durdurun:

```bash
C="docker compose --env-file .env -f deploy/docker-compose.yml"

$C stop proxy analyzer
$C exec -T postgres dropdb -U tamga --if-exists tamga
$C exec -T postgres createdb -U tamga --owner=tamga tamga
$C exec -T postgres pg_restore -U tamga -d tamga --no-owner --no-acl --single-transaction \
  < tamga-YYYYMMDD-HHMMSS.dump
$C start analyzer proxy
```

Yedek daha eski bir sürümden alındıysa, o tarihten sonra eklenen migration
dosyalarını sırayla uygulayın ([Sürüm yükseltme](#sürüm-yükseltme)). Geri
yüklemeden sonra sağlık kontrolünü tekrarlayın. Yedeğinizi, gerçekten geri
yüklenebildiğini bir deneme ortamında görmeden yedek saymayın.

Veritabanına ağ üzerinden erişebildiğiniz kurulumlar için hazır betikler
(`backup.sh`, `restore.sh`) ve zamanlama örneği:
[deploy/backup/README.md](../../deploy/backup/README.md).

## Sürüm yükseltme

1. **Değişiklik günlüğünü okuyun.** [CHANGELOG.md](../../CHANGELOG.md)
   içinde hedef sürümün bölümünde kalın yazılmış cümleler, çalışan bir
   kurulumun fark edeceği değişikliklerdir: değişen bir karar, kaldırılan
   bir ayar, yeniden adlandırılan bir log alanı. Özellikle "Deployment and
   operations" ve "Security" başlıklarına bakın.
2. **Yedek alın** (önceki bölüm).
3. **Kodu güncelleyin.** Bugün etiketli sürüm olmadığı için `git pull`;
   sürümler yayımlandığında `git fetch --tags && git checkout vX.Y.Z`.
4. **İmajları derleyip servisleri yenileyin:**

   ```bash
   docker compose --env-file .env -f deploy/docker-compose.yml build
   docker compose --env-file .env -f deploy/docker-compose.yml up -d
   ```

   `scanner-service` kullanıyorsanız (`TAMGA_SCANNER_SERVICE_ADDR` dolu ise)
   onu **proxy ile birlikte** yükseltin; yukarıdaki komut ikisini de derler.
   Eski bir scanner-service ile yeni proxy arasında bulgu biçimi farklı
   olabilir: v0.9.0'da böyle bir eşleşmede `REDACT` kuralındaki gizli bilgi
   maskelenmek yerine engellenir.
5. **Yeni migration dosyalarını uygulayın.** Var olan bir veritabanına
   migration kendiliğinden uygulanmaz; Compose bunu yalnızca boş veri
   biriminde yapar. Hangi dosyaların uygulandığını da hiçbir şey kaydetmez:
   kendi notunuzu tutun. Önce yeni proxy'yi başlatın, sonra yalnızca sizin
   veritabanınız için yeni olan dosyaları sırayla çalıştırın (dosya adı
   örnektir):

   ```bash
   docker compose --env-file .env -f deploy/docker-compose.yml exec -T postgres \
     psql -U tamga -d tamga -v ON_ERROR_STOP=1 < deploy/migrations/019_drop_unused_tables.up.sql
   ```

   Hepsini baştan döngüyle çalıştırmayın: `001`, `010` ve `011` ikinci kez
   çalıştırılınca hata verir. `017`, `018` ve `019` ile ilgili notlar:
   [Upgrading the database](../operations.md#upgrading-the-database).
6. **Sağlık kontrolünü ve engelleme denemesini tekrarlayın** (yukarıdaki
   "İlk çalıştırma ve sağlık kontrolü" bölümü).

Yükseltmeden önce yazılmış kayıtlar yeniden yazılmaz: eski biçimdeki
bulgular, saklama süresi dolup silinene kadar eski halleriyle kalır.

## Loglar ve metrikler

```bash
docker compose --env-file .env -f deploy/docker-compose.yml logs -f proxy
```

Proxy'nin logunda istek metni, bulunan değer ya da anahtar yer almaz;
hata satırları hatanın türünü (`error_kind`) verir, metnini vermez. Her
yanıtta `X-Tamga-Request-Id` başlığı vardır; bir isteği logda ve console'da
bu numarayla bulursunuz.

Metrikler Prometheus biçimindedir ve bir yönetim anahtarı ister (`read`
kapsamlı bir anahtar yeterlidir):

```bash
curl -s -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" http://localhost:8443/api/v1/metrics
```

İzlemeye değer olanlar:

| Metrik | Ne söyler |
|---|---|
| `tamga_requests_total`, `tamga_blocked_total` | Taranan ve engellenen istek sayısı |
| `tamga_scan_degraded_total` | Tamamlanamayan taramalar (`reason` etiketiyle). Artıyorsa 503'lerin nedeni budur |
| `tamga_malformed_json_total` | Belirsiz JSON gövdesi yüzünden reddedilen istekler |
| `tamga_request_log_unrecorded_total` | Veritabanına yazılamayan istek kayıtları |
| `tamga_alerts_total` | Gönderilen, engellenen ve atlanan uyarılar |

İzler (trace) Jaeger'a gider (`http://localhost:16686`, isteklerin %5'i).
Bu adres yalnızca sunucunun kendisinden açılır. Ağdan erişimi geri getirmek
için `.env` içinde `TAMGA_OBSERVABILITY_BIND=0.0.0.0` yazılabilir; Jaeger ve
NATS izleme uçlarında kimlik doğrulama olmadığı için bunu yalnızca ayrı bir
ağ kontrolü varsa yapın.
Yönetim işlemleri (policy, anahtar, rol değişiklikleri ve reddedilen
kimlik bilgileri) console'daki **Audit** sayfasındadır. Uyarıları Slack,
Teams ya da bir SIEM'e göndermek için: **Integrations** sayfası ve
[Alerts](../operations.md#alerts). Bu hedeflerin hiçbiri gerçek servise
karşı denenmemiştir; kendi ortamınızda sınayın.

## LiteLLM köprüsü

Kurumunuzda zaten bir LiteLLM proxy'si çalışıyorsa trafiği Tamga'nın
üzerinden geçirmeniz gerekmez: LiteLLM, isteğin ve yanıtın metinlerini karar
için Tamga'ya sorabilir. LiteLLM'in `generic_guardrail_api` türünde bir
guardrail tanımına `api_base` olarak Tamga'nın `/api/v1` adresini, `api_key`
olarak `proxy` kapsamlı bir Tamga anahtarını yazmak yeterlidir; karar aynı
policy ile verilir ve console'da "LiteLLM bridge" olarak görünür. Gerçek bir
LiteLLM container'ı ile uçtan uca test yazılmış ama henüz çalıştırılmamıştır.
Kurulum: [litellm.md](../litellm.md).

## Web tarayıcısı eklentisi

Çalışanların web tarayıcısında claude.ai, chatgpt.com ve gemini.google.com
sitelerine yazdığı metin proxy'den geçmez. `extension/` dizinindeki eklenti
bu boşluk içindir: yapıştırma ve gönderme anında metni kendi Tamga'nıza
sorar, karara göre gönderimi durdurur ya da maskelenmiş metni önerir.
Eklenti bir MVP'dir: hiçbir mağazada yoktur (paketlenmemiş yüklenir ya da
kurum kendi imzalayıp grup ilkesiyle dağıtır), canlı sitelerde yalnızca bir
kez ve tek bir Chromium tarayıcıda denenmiştir, dosya yüklemelerini ve
sitelerin mobil ya da masaüstü uygulamalarını kapsamaz. Siteler sayfalarını
haber vermeden değiştirir; güvenmeden önce her sitede kendiniz deneyin.
Kurulum ve kurumsal ilke örneği: [browser-extension.md](../browser-extension.md).

## Kubernetes ve Helm

`deploy/helm/tamga-proxy/` altındaki chart yalnızca proxy'yi kurar;
PostgreSQL, Redis, analyzer ve console'u siz sağlarsınız. Yayımlanmış imaj
olmadığı için önce `proxy/Dockerfile` ile imajı derleyip kendi kayıt
deponuza (registry) göndermeniz gerekir. Chart'ta policy her zaman salt
okunurdur; `policy.inline` değerini değiştirip sürümü yükselterek
güncellersiniz. Değerler, gizli bilgilerin Secret ile verilmesi ve bilinen
sınırlar: [deploy/README.md](../../deploy/README.md).

## Sorun giderme

| Belirti | Neden | Ne yapılır |
|---|---|---|
| PostgreSQL başlamıyor, proxy `postgres` bekliyor | `--env-file .env` verilmedi, parola boş kaldı | Komutu `--env-file .env` ile çalıştırın |
| İstek 403 dönüyor | Policy isteği engelledi. Bu, sistemin çalıştığı durumdur | Console'da **Incidents** sayfasından hangi kuralın tetiklendiğine bakın |
| 503 `tamga_scan_unavailable` | Tarama tamamlanamadı ve policy taranmamış isteği iletmeye izin vermiyor (`scan.on_error: block`). Yanıtlarda: yanıt taraması bitmedi ve `output_rules.fail_open` kapalı | İstek tekrar denenebilir (`Retry-After`). Sürüyorsa proxy logunda `error_kind` alanına ve `tamga_scan_degraded_total` metriğine bakın; classifier ya da scanner-service kullanıyorsanız ayakta olduklarını doğrulayın |
| 503 `tamga_redaction_incomplete` | Maskeleme yapılacaktı ama tarama metnin sonuna ulaşamadı; metin yarım maskelenmiş halde gönderilmez. Gövdedeki `reason`: `findings_cap`, `time_budget`, `deadline`, `cancelled`, `scanner_failed` | `findings_cap` ise aynı metin her seferinde reddedilir (çok fazla bulgu var): metni bölün ya da kuralı `BLOCK` yapın. Diğerlerinde tekrar deneyin |
| 401 `tamga_key_required` ya da `tamga_key_invalid` | `TAMGA_REQUIRE_KEY=true` iken `X-Tamga-Key` yok, ya da anahtar yanlış, süresi dolmuş, iptal edilmiş | Uygulamaya geçerli bir `proxy` anahtarı verin |
| 403 `permission_denied` | Yönetim API'sini çağıran anahtarın rolü bu işlem için yetmiyor. Gövdede `required_permission` yazar | Uygun kapsamda (`write` ya da `admin`) bir anahtar kullanın |
| 401 `session_revoked` | Oturum sonlandırılmış: rol değişti ya da kaldırıldı, ya da çıkış yapıldı | Yeniden oturum açın |
| Console'da "The proxy did not accept the admin key." (olay akışında "Live feed off: the proxy did not accept the admin key") | **Settings** sayfasındaki anahtar yanlış, iptal edilmiş ya da süresi dolmuş; ya da proxy başka bir `TAMGA_ADMIN_KEY` ile yeniden başlatılmış | **Settings** sayfasında geçerli anahtarı girin |
| Console açılıyor ama veri gelmiyor | Web tarayıcısı proxy'ye ulaşamıyor: `NEXT_PUBLIC_API_URL` tarayıcıdan erişilebilen adres değil, ya da console'un adresi `TAMGA_CORS_ORIGINS` içinde yok | İkisini düzeltin; ilki için console imajını yeniden derleyin |
| Console'da policy kaydedilemiyor, 409 `policy_read_only` | Policy salt okunur bağlı | `.env` içinde `TAMGA_POLICY_MOUNT=rw` yapıp `up -d` ile proxy'yi yeniden oluşturun, ya da dosyayı sunucuda düzenleyin |
| 400 `tamga_invalid_json` | İstek gövdesi, iki ayrıştırıcının farklı okuyabileceği bir JSON (yinelenen anahtar, geçersiz UTF-8) | Uygulamanın gönderdiği gövdeyi düzeltin |
| PDF raporu 503 `analyzer_unavailable` | Analyzer çalışmıyor ya da proxy ona ulaşamıyor | `docker compose … ps` ve `logs analyzer` |
| Bulgularda `[not stored: no finding hash key]` | `data.hash_findings` açık, anahtar tanımlı değil | `TAMGA_FINDING_HASH_KEY` tanımlayın ([Bulguların saklanması](#bulguların-saklanması)) |

Bir güvenlik açığı bulduysanız herkese açık bir kayıt açmayın;
[SECURITY.md](../../SECURITY.md) belgesindeki yolu izleyin.

## KVKK ve BDDK açısından

**Bu bölüm hukuki görüş değildir.** Tamga'nın hangi özelliğinin hangi
yükümlülükle ilgili olduğunu gösterir. Bir kurumun mevzuata uyup uymadığı
kendi süreçlerine ve yetkin bir kişinin değerlendirmesine bağlıdır; bir
yazılımı kurmak tek başına uyum sağlamaz.

Tamga'nın denetimleri yalnızca proxy'den geçen trafik için geçerlidir ve
engellenmeyen her istek, adı geçen sağlayıcıya yine iletilir. Bu sınırlar
içinde:

- **KVKK md. 4 (amaçla bağlantılı, sınırlı ve ölçülü olma):** `REDACT`
  kuralı, listelenen kişisel veri türlerini istek iletilmeden önce maskeler.
  Bu, gönderilen veriyi sınırlamaya **yardımcı olur**; yalnızca listelenen
  türleri ve kalıpların tanıdığını maskeler.
- **KVKK md. 7 ve md. 11:** saklama işi ve bir kişinin kayıtlarını sorgulama
  ve silme uç noktaları, Tamga'nın **kendi** istek kayıtları için bu
  taleplerin karşılanmasını **destekler**. Sağlayıcının ya da uygulamanın
  tuttuğu veriye dokunmaz; başvuruyu almak ve yanıtlamak kurumun sürecidir.
- **KVKK md. 9 (yurt dışına aktarım):** sağlayıcı izin listesi isteklerin
  hangi sağlayıcılara gidebileceğini sınırlar. Dış bir sağlayıcıya iletilen
  istek altyapınızdan çıkar; Tamga o sağlayıcının veriyi nerede işlediğini
  ve aktarımın hukuki dayanağı olup olmadığını bilemez.
- **KVKK md. 12 (veri güvenliği):** roller, kapsamlı anahtarlar, TLS ve
  denetim kaydı bu yükümlülüğe yönelik teknik tedbirlerden **birkaçıdır**.
  Denetim kaydının özet zinciri yönetim işlemlerini kapsar;
  `GET /api/v1/audit/verify` tablonun tamamını değil, sürecin bellekte
  tuttuğu son 512 kaydı doğrular.
- **BDDK (Bankaların Bilgi Sistemleri ve Elektronik Bankacılık Hizmetleri
  Hakkında Yönetmelik):** Yönetmelik süreç, komite, belge ve sözleşme ister;
  bunları yazılım karşılamaz. Tamga'nın istek kaydı, denetim kaydı ve
  uyarıları bankanın kendi iz kaydı ve olay yönetimi süreçlerine **girdi
  olabilir**. Bir bankada Tamga'nın kendisi de bankanın envanter, erişim,
  değişiklik, kayıt ve süreklilik kontrollerinin kapsaması gereken bir bilgi
  sistemidir. Model sağlayıcının seçimi, sözleşmesi ve denetimi (dış hizmet
  alımı) bankanın işidir.

Madde madde eşleme, kaynak metinler ve her satırın neyi karşılamadığı:
[kvkk.md](../compliance/kvkk.md) ve [bddk.md](../compliance/bddk.md)
(İngilizce). Kurul'un üretken ve etken yapay zekâ rehberleriyle eşleme de
KVKK sayfasındadır.

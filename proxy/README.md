# Tamga proxy

OpenAI/Anthropic trafiği için güvenlik tarayıcıları ve YAML politikası uygulayan Go reverse proxy.

## Sprint 2 özellikleri

- **Politika hot-reload**: `PolicyStore` + dosya watcher; çalışma zamanında YAML yeniden yüklenir (veya `POST /api/v1/policies/reload` ile manuel; admin rolü gerekir).
- **Event bus**: Tarama / blok / çıktı ipuçları için buffered publish–subscribe; metrik, log, analyzer, DB ve dashboard buffer handler’ları.
- **Opsiyonel PostgreSQL logging**: `TAMGA_DB_URL` ile istek telemetrisi; kapalıyken no-op store.
- **Rate limiting**: Politikadaki `rate_limit` ile API anahtarı başına token bucket (farklı anahtarlar bağımsız sayaçlar).
- **Dashboard REST API**: Aynı port üzerinde `/api/v1/*` (admin anahtarı + CORS). Roller: admin, analyst, viewer; her route hangi yetkiyi istediğini `internal/api/routes.go` içinde belirtir.

## Sprint 3 (demo-ready) eklemeleri

- **Canlı terminal anlatımı için geliştirilmiş loglar**: kısa request ID (`request_id_short`), action ikonları (✓ PASS / ✗ BLOCK / ↻ REDACT / ⚠ WARN) ve daha okunabilir alanlar.
- **REDACT header desteği**: redaksiyon durumunda yanıt header’ı olarak `X-Tamga-Redacted-Count`.
- **Body boyut limiti**: varsayılan `1MB` (opsiyonel `TAMGA_MAX_BODY_BYTES`) — limit aşılırsa `413 Payload Too Large`.
- **Mock upstream modu (offline demo)**: `TAMGA_MOCK_UPSTREAM=true` ile proxy gerçek provider’lara bağlanmadan demo için sahte yanıt döner.
- **Stats/Events endpoint zenginleşmesi**: `/api/v1/stats` artık demo’da beklenen “top providers / top finding types / avg scan latency” bilgilerini de döndürür; `/api/v1/events` son olayları listeler.

### Demo script

`ANTHROPIC_API_KEY` ve `TAMGA_ADMIN_KEY` ayarladıktan sonra tek komutla senaryoları çalıştırabilirsiniz:

Proxy klasörüne geçin (`cd proxy`):

Windows:

```powershell
powershell -ExecutionPolicy Bypass -File scripts/demo.ps1
```

Linux/Mac:

```bash
bash scripts/demo.sh
```

Mock upstream için:

```bash
TAMGA_MOCK_UPSTREAM=true go run ./cmd/tamga
```

## Gereksinimler

- Go 1.25+ (`go.mod`: 1.25.4)

## Hızlı başlangıç

```bash
cd proxy
go run ./cmd/tamga
```

Varsayılan dinleme portu **8443** (`TAMGA_PROXY_PORT` ile değiştirilebilir). Politika dosyası: `./tamga-policy.yaml` (`TAMGA_POLICY_PATH`).

Başka bir terminalde örnek istek:

```bash
curl -X POST http://localhost:8443/v1/chat/completions \
  -H "Content-Type: application/json" \
  -H "Authorization: Bearer $OPENAI_API_KEY" \
  -d '{"model":"gpt-4o-mini","messages":[{"role":"user","content":"Merhaba"}]}'
```

## REST API (`/api/v1`)

Aynı HTTP sunucusu üzerinde; geliştirme için CORS varsayılan `*` (`TAMGA_CORS_ORIGIN`).

| Metot | Yol | Açıklama |
|--------|-----|----------|
| `GET` | `/api/v1/health/detailed` | Anahtarsız: yalnızca proxy ayakta mı ve tarama süreleri. Yönetim anahtarıyla: DB, tarayıcı sayısı, uptime, sağlayıcı havuzları |
| `GET` | `/api/v1/stats` | Son 7 gün özeti (DB veya bellek metrikleri) |
| `GET` | `/api/v1/events?page=&limit=` | Güvenlik olayları (DB veya bellekteki son 1000 istek) |
| `GET` | `/api/v1/policies` | Aktif politika JSON |
| `POST` | `/api/v1/policies/reload` | Politika dosyasını diskten yeniden yükle (admin rolü) |

Bu tablo bir seçkidir. Tüm route’lar ve her birinin istediği rol (`x-required-role`): [`docs/openapi.yaml`](docs/openapi.yaml).

Korumalı route’lar için header: **`X-Tamga-Admin-Key`**. Değeri `TAMGA_ADMIN_KEY` (admin) ya da `read` / `write` / `admin` kapsamlı bir API anahtarıdır (viewer / analyst / admin). Anahtar yalnızca bu header’dan okunur; URL’deki `?key=` hiçbir route’ta okunmaz. Anahtar boşsa ya da geçersizse korumalı endpoint’ler **401**, rol yetmiyorsa **403** döner.

`GET /health` anahtarsız canlılık kontrolüdür. `/api/v1/health/detailed` bir bağımlılık (DB, Redis, analyzer) düştüğünde **503** döner.

Örnek:

```bash
curl -s http://localhost:8443/api/v1/health/detailed | jq .
curl -s -H "X-Tamga-Admin-Key: $TAMGA_ADMIN_KEY" http://localhost:8443/api/v1/stats | jq .
```

## Ortam değişkenleri (seçilmiş)

| Değişken | Açıklama |
|----------|----------|
| `TAMGA_PROXY_PORT` | Dinleme portu (varsayılan 8443) |
| `TAMGA_POLICY_PATH` | Politika YAML yolu |
| `TAMGA_DB_URL` | PostgreSQL (boş = DB logging kapalı) |
| `TAMGA_ORG_ID` | DB sorgularında varsayılan org (dashboard/istatistik) |
| `TAMGA_ADMIN_KEY` | Dashboard API admin anahtarı |
| `TAMGA_CORS_ORIGIN` | CORS `Access-Control-Allow-Origin` |
| `TAMGA_ANALYZER_ADDR` | Analyzer gRPC adresi (varsayılan `localhost:50051`) |
| `TAMGA_ANALYZER_HTTP_URL` | Analyzer HTTP tabanı (varsayılan `http://localhost:8444`); PDF raporları buradan alınır |
| `REDIS_URL` | Redis (boş = her kopya kendi belleğindeki sayaçları kullanır) |

Tam liste: [`docs/operations.md`](../docs/operations.md#configuration).

## Docker Compose ile çalıştırma

Tam yığın (proxy, analyzer, scanner-service, postgres, redis, nats, jaeger, dashboard) `deploy/` altında tanımlıdır. Depo kökünden:

```bash
cp .env.example .env
docker compose --env-file .env -f deploy/docker-compose.yml up -d --build
```

`--env-file .env` gereklidir: Compose yalnızca compose dosyasının yanındaki `.env` dosyasını kendiliğinden okur.

Proxy **8443**, dashboard **3000** portlarında host’a açılır; analyzer (**8444**), postgres ve redis yalnızca iç ağdadır. Proxy için örnek ortam:

- `TAMGA_POLICY_PATH=/app/tamga-policy.yaml` (volume ile bağlanır)
- `TAMGA_ADMIN_KEY` (yönetim API’si için; boşsa korumalı route’lar 401 döner), isteğe bağlı `TAMGA_ORG_ID`

Compose dosyası: [`deploy/docker-compose.yml`](../deploy/docker-compose.yml).

## Geliştirme

```bash
go build ./cmd/tamga
go vet ./...
go test ./... -v -count=1
```

### Tarayıcı benchmark’ları

Tüm `BenchmarkScanAll_*` senaryoları (küçük/orta/büyük metin, PII, secret, injection, karışık tehdit):

```bash
go test ./internal/scanner/ -bench=. -benchmem -count=3
```

**PowerShell:** `-bench=.` bazen yanlış parse edilir; gerekirse şunu kullanın:

```powershell
go test ./internal/scanner/ "-bench=." -benchmem -count=3
```

Bu dosyada ölçüm değeri tutulmaz; sonuç donanıma göre değişir. Yayınlanan ölçümler ve yöntem: [`docs/benchmarks/README.md`](../docs/benchmarks/README.md). Üretim öncesi kendi ortamınızda `go test ./internal/scanner/ "-bench=." -benchmem -count=5` ile ölçün.

## Docker (yalnızca proxy imajı)

```bash
docker build -t tamga-proxy .
```

## Sprint 2 sonunda özet

- Production’a yakın Go proxy (tarayıcılar + politika + hot-reload)
- Event-driven mimari (bus + handler’lar)
- İsteğe bağlı PostgreSQL ile istek loglama
- API anahtarı başına yapılandırılabilir rate limiting
- Dashboard için REST API
- Geniş test seti + bellek profilli benchmark’lar

# Tamga proxy

OpenAI/Anthropic trafiği için güvenlik tarayıcıları ve YAML politikası uygulayan Go reverse proxy.

## Sprint 2 özellikleri

- **Politika hot-reload**: `PolicyStore` + dosya watcher; çalışma zamanında YAML yeniden yüklenir (veya `POST /api/v1/policies/reload` ile manuel).
- **Event bus**: Tarama / blok / çıktı ipuçları için buffered publish–subscribe; metrik, log, analyzer, DB ve dashboard buffer handler’ları.
- **Opsiyonel PostgreSQL logging**: `TAMGA_DB_URL` ile istek telemetrisi; kapalıyken no-op store.
- **Rate limiting**: Politikadaki `rate_limit` ile API anahtarı başına token bucket (farklı anahtarlar bağımsız sayaçlar).
- **Dashboard REST API**: Aynı port üzerinde `/api/v1/*` (admin anahtarı + CORS).

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

- Go 1.22+

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
| `GET` | `/api/v1/health/detailed` | Proxy, DB, tarayıcı sayısı, uptime (admin anahtarı gerekmez) |
| `GET` | `/api/v1/stats` | Son 7 gün özeti (DB veya bellek metrikleri) |
| `GET` | `/api/v1/events?page=&limit=` | Güvenlik olayları (DB veya son ~100 bellek) |
| `GET` | `/api/v1/policies` | Aktif politika JSON |
| `POST` | `/api/v1/policies/reload` | Politika dosyasını diskten yeniden yükle |

Korumalı route’lar için header: **`X-Tamga-Admin-Key`** (`TAMGA_ADMIN_KEY`). Anahtar boşsa korumalı endpoint’ler **401** döner.

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
| `TAMGA_ANALYZER_URL` | Analyzer servis tabanı |

## Docker Compose ile çalıştırma

Tam yığın (proxy, analyzer, postgres, redis, dashboard) `tamga/deploy` altında tanımlıdır:

```bash
cd deploy
docker compose up --build
```

Proxy **8443**, analyzer **8444**, dashboard **3000** portlarında açılır. Proxy için örnek ortam:

- `TAMGA_POLICY_PATH=/app/tamga-policy.yaml` (volume ile bağlanır)
- İsteğe bağlı: `TAMGA_ADMIN_KEY`, `TAMGA_ORG_ID` (dashboard API + DB satırları için)

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

### Örnek benchmark çıktısı (referans)

Aşağıdaki tablo tek bir geliştirme makinesinde (Windows, Go `test -bench=. -benchmem -count=3`) alınmış özet değerlerdir; donanıma göre değişir.

| Benchmark | Tipik süre / iterasyon | Not |
|-----------|-------------------------|-----|
| `BenchmarkScanAll_SmallPrompt` (~100 B içerik) | ~0.27 ms | Düz metin |
| `BenchmarkScanAll_MediumPrompt` (~1 KB) | ~1.0 ms | |
| `BenchmarkScanAll_LargePrompt` (~10 KB) | ~19 ms | |
| `BenchmarkScanAll_WithPII` (~1 KB, e-posta + kart) | ~1.2 ms | PII yoğun |
| `BenchmarkScanAll_WithSecrets` (~500 B, API anahtarı) | ~0.55 ms | |
| `BenchmarkScanAll_WithInjection` (~500 B) | ~0.50 ms | Enjeksiyon kalıbı |
| `BenchmarkScanAll_MixedThreats` | ~0.38 ms | PII + secret + injection |

Üretim öncesi kendi ortamınızda `go test ./internal/scanner/ "-bench=." -benchmem -count=5` ile ölçün.

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

#!/usr/bin/env bash
# NOT: Windows'ta bu script yerine demo.ps1 kullanın
# Bu script WSL2'de Windows localhost'a erişemez
set -euo pipefail

BASE="${BASE_URL:-http://localhost:8443}"
API_KEY="${ANTHROPIC_API_KEY:-}"
ADMIN_KEY="${TAMGA_ADMIN_KEY:-}"

if [[ -z "$API_KEY" ]]; then
  echo "ANTHROPIC_API_KEY is required"
  exit 1
fi
if [[ -z "$ADMIN_KEY" ]]; then
  echo "TAMGA_ADMIN_KEY is required (for /api/v1/stats and /api/v1/events)"
  exit 1
fi

echo "═══════════════════════════════════════"
echo "  TAMGA DEMO — Canlı Güvenlik Testi"
echo "═══════════════════════════════════════"
echo ""
echo "BASE=$BASE"
echo "Using: Anthropic Claude compatible endpoint"
echo ""

echo "📍 Sahne 1: Normal kullanım — temiz istek"
echo "----------------------------------------------"
curl -sS "${BASE}/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: ${API_KEY}" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 200,
    "messages": [{"role":"user","content":"Merhaba, bugün hava nasıl?"}]
  }'
echo ""
sleep 3

echo "📍 Sahne 2: Secret sızıntısı — ENGELLE"
echo "----------------------------------------------"
curl -sS "${BASE}/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: ${API_KEY}" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 200,
    "messages": [{
      "role":"user",
      "content":"Bu AWS yapılandırmasında bir sorun var mı?\n\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\nregion = eu-west-1"
    }]
  }'
echo ""
sleep 3

echo "İkinci deneme — Stripe key sızıntısı"
curl -sS "${BASE}/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: ${API_KEY}" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 200,
    "messages": [{
      "role":"user",
      "content":"Ödeme entegrasyonumda hata var. Key: sk_live_51234567890abcdefghijklmnop"
    }]
  }'
echo ""
sleep 3

echo "📍 Sahne 3: PII maskeleme — TC + telefon + kredi kartı + e-posta"
echo "----------------------------------------------"
curl -sS "${BASE}/anthropic/v1/messages" \
  -H "Content-Type: application/json" \
  -H "x-api-key: ${API_KEY}" \
  -H "anthropic-version: 2023-06-01" \
  -d '{
    "model": "claude-sonnet-4-20250514",
    "max_tokens": 200,
    "messages": [{
      "role":"user",
      "content":"Şu müşterinin kaydını özetle:\nAd: Ayşe Yılmaz\nTC: 10000000146\nTelefon: +90 532 123 4567\nKredi Kartı: 4532015112830366\nE-posta: ayse.yilmaz@sirket.com"
    }]
  }'
echo ""
sleep 3

echo "📍 Sahne 4: Kullanım istatistikleri — /api/v1/stats + /api/v1/events"
echo "----------------------------------------------"
curl -sS "${BASE}/api/v1/stats" \
  -H "X-Tamga-Admin-Key: ${ADMIN_KEY}"
echo ""

curl -sS "${BASE}/api/v1/events?limit=5" \
  -H "X-Tamga-Admin-Key: ${ADMIN_KEY}"
echo ""

echo "Demo bitti."


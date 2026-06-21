param()

$ErrorActionPreference = "Stop"

$BaseUrl = "http://localhost:8443"

$AnthropicApiKey = $env:ANTHROPIC_API_KEY
if ([string]::IsNullOrWhiteSpace($AnthropicApiKey)) {
  throw "ANTHROPIC_API_KEY is missing. Set $env:ANTHROPIC_API_KEY and retry."
}

$AdminKey = $env:TAMGA_ADMIN_KEY
if ([string]::IsNullOrWhiteSpace($AdminKey)) {
  $AdminKey = "test-admin-key"
}

function Invoke-TamgaCurl {
  param(
    [Parameter(Mandatory=$true)][ValidateSet("GET","POST")][string]$Method,
    [Parameter(Mandatory=$true)][string]$Url,
    [Parameter(Mandatory=$false)][hashtable]$Headers,
    [Parameter(Mandatory=$false)][string]$Body,
    [Parameter(Mandatory=$false)][switch]$CaptureHeaders
  )

  $bodyFile = New-TemporaryFile
  $headersFile = $null
  $curlArgs = @(
    "-sS",
    "-o", $bodyFile.FullName,
    "-w", "%{http_code}"
  )

  if ($CaptureHeaders) {
    $headersFile = New-TemporaryFile
    $curlArgs += @("-D", $headersFile.FullName)
  }

  if ($Method -eq "POST") {
    $curlArgs += @("-X", "POST")
    if ($null -ne $Body) {
      $curlArgs += @("--data-binary", $Body)
    }
  } else {
    $curlArgs += @("-X", "GET")
  }

  if ($Headers) {
    foreach ($k in $Headers.Keys) {
      $curlArgs += @("-H", "${k}: $($Headers[$k])")
    }
  }

  $curlArgs += @($Url)
  $statusText = & curl.exe @curlArgs
  $statusCode = [int]$statusText

  $bodyText = Get-Content -LiteralPath $bodyFile.FullName -Raw -ErrorAction SilentlyContinue
  $headersText = $null
  if ($CaptureHeaders -and $headersFile) {
    $headersText = Get-Content -LiteralPath $headersFile.FullName -Raw -ErrorAction SilentlyContinue
  }

  return [pscustomobject]@{
    StatusCode = $statusCode
    Body = $bodyText
    Headers = $headersText
  }
}

function Extract-HeaderValue {
  param(
    [Parameter(Mandatory=$true)][string]$HeadersText,
    [Parameter(Mandatory=$true)][string]$HeaderName
  )
  # Example line: X-Tamga-Redacted-Count: 4
  $pattern = [regex]::Escape($HeaderName) + ":\s*([0-9]+)"
  $m = [regex]::Match($HeadersText, $pattern, "IgnoreCase")
  if ($m.Success) { return $m.Groups[1].Value }
  return $null
}

function Extract-HeaderTextValue {
  param(
    [Parameter(Mandatory=$true)][string]$HeadersText,
    [Parameter(Mandatory=$true)][string]$HeaderName
  )
  $pattern = [regex]::Escape($HeaderName) + ":\s*([^\r\n]+)"
  $m = [regex]::Match($HeadersText, $pattern, "IgnoreCase")
  if ($m.Success) { return $m.Groups[1].Value.Trim() }
  return $null
}

Write-Host "╔═══════════════════════════════════════╗" -ForegroundColor Cyan
Write-Host "║  TAMGA DEMO — Canlı Güvenlik Testi  ║" -ForegroundColor Cyan
Write-Host "╚═══════════════════════════════════════╝" -ForegroundColor Cyan
Write-Host ""
Write-Host "Base URL: $BaseUrl" -ForegroundColor Cyan
Write-Host "Provider: Anthropic Claude uyumlu endpoint" -ForegroundColor Cyan
Write-Host ""

# Scene 1
Write-Host "📍 Sahne 1: Normal kullanım — temiz istek" -ForegroundColor Cyan
Write-Host "----------------------------------------------"
$s1Body = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{"role":"user","content":"Merhaba, bugün hava nasıl?"}]
}
'@

$headers = @{
  "Content-Type" = "application/json"
  "x-api-key" = $AnthropicApiKey
  "anthropic-version" = "2023-06-01"
}
$res1 = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s1Body
if ($res1.StatusCode -ge 200 -and $res1.StatusCode -lt 300) {
  Write-Host "PASS (HTTP $($res1.StatusCode))" -ForegroundColor Green
} else {
  Write-Host "Beklenmeyen sonuç (HTTP $($res1.StatusCode))" -ForegroundColor Red
}
Write-Host $res1.Body
Start-Sleep -Seconds 3

# Scene 2
Write-Host ""
Write-Host "📍 Sahne 2: Secret sızıntısı — ENGELLE" -ForegroundColor Cyan
Write-Host "----------------------------------------------"
$s2Body = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{
    "role":"user",
    "content":"Bu AWS yapılandırmasında bir sorun var mı?\n\naws_access_key_id = AKIAIOSFODNN7EXAMPLE\naws_secret_access_key = wJalrXUtnFEMI/K7MDENG/bPxRfiCYEXAMPLEKEY\nregion = eu-west-1"
  }]
}
'@

$res2 = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s2Body
if ($res2.StatusCode -eq 403) {
  Write-Host "BLOCK (HTTP 403)" -ForegroundColor Red
} else {
  Write-Host "Beklenen BLOCK olmadı (HTTP $($res2.StatusCode))" -ForegroundColor Red
}
Write-Host $res2.Body
Start-Sleep -Seconds 3

# Scene 3
Write-Host ""
Write-Host "📍 Sahne 3: PII maskeleme — TC + telefon + kredi kartı + e-posta" -ForegroundColor Cyan
Write-Host "----------------------------------------------"
$s3Body = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{
    "role":"user",
    "content":"Şu müşterinin kaydını özetle:\nAd: Ayşe Yılmaz\nTC: 10000000146\nTelefon: +90 532 123 4567\nKredi Kartı: 4532015112830366\nE-posta: ayse.yilmaz@sirket.com"
  }]
}
'@

$res3 = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s3Body -CaptureHeaders
$redactedCount = $null
if ($res3.Headers) {
  $redactedCount = Extract-HeaderValue -HeadersText $res3.Headers -HeaderName "X-Tamga-Redacted-Count"
}

if ($res3.StatusCode -ge 200 -and $res3.StatusCode -lt 300 -and $null -ne $redactedCount -and [int]$redactedCount -gt 0) {
  Write-Host "REDACT (HTTP $($res3.StatusCode), X-Tamga-Redacted-Count=$redactedCount)" -ForegroundColor Yellow
} else {
  Write-Host "REDACT beklenenden farklı çıktı (HTTP $($res3.StatusCode), RedactedCount=$redactedCount)" -ForegroundColor Yellow
}
Write-Host $res3.Body
Start-Sleep -Seconds 3

# Scene 4
Write-Host ""
Write-Host "📍 Sahne 4: Kullanım istatistikleri — /api/v1/stats + /api/v1/events" -ForegroundColor Cyan
Write-Host "----------------------------------------------"
$statsHeaders = @{
  "X-Tamga-Admin-Key" = $AdminKey
}

$res4a = Invoke-TamgaCurl -Method "GET" -Url "$BaseUrl/api/v1/stats" -Headers $statsHeaders
if ($res4a.StatusCode -ge 200 -and $res4a.StatusCode -lt 300) {
  Write-Host "Stats OK (HTTP $($res4a.StatusCode))" -ForegroundColor Green
} else {
  Write-Host "Stats failed (HTTP $($res4a.StatusCode))" -ForegroundColor Red
}
Write-Host $res4a.Body

$res4b = Invoke-TamgaCurl -Method "GET" -Url "$BaseUrl/api/v1/events?limit=5" -Headers $statsHeaders
if ($res4b.StatusCode -ge 200 -and $res4b.StatusCode -lt 300) {
  Write-Host "Events OK (HTTP $($res4b.StatusCode))" -ForegroundColor Green
} else {
  Write-Host "Events failed (HTTP $($res4b.StatusCode))" -ForegroundColor Red
}
Write-Host $res4b.Body

# Scene 5 — Custom entity (MN + sicil)
Write-Host ""
Write-Host "📍 Sahne 5: Custom entity - musteri / sicil pattern" -ForegroundColor Cyan
Write-Host "----------------------------------------------"
$s5Body = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{"role":"user","content":"Müşteri MN-12345678 hakkında bilgi ver. Sicil: SN654321"}]
}
'@

$res5 = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s5Body -CaptureHeaders
$rc5 = $null
if ($res5.Headers) {
  $rc5 = Extract-HeaderValue -HeadersText $res5.Headers -HeaderName "X-Tamga-Redacted-Count"
}
$isHTTP2xx = ($res5.StatusCode -ge 200 -and $res5.StatusCode -lt 300)
$hasRedaction = $false
if ($null -ne $rc5) {
  try {
    $hasRedaction = ([int]$rc5 -ge 1)
  } catch {
    $hasRedaction = $false
  }
}
if ($isHTTP2xx -and $hasRedaction) {
  Write-Host "REDACT (HTTP $($res5.StatusCode), X-Tamga-Redacted-Count=$rc5) - custom + PII birlikte sayilir" -ForegroundColor Yellow
} elseif ($isHTTP2xx) {
  Write-Host "HTTP OK fakat Redacted-Count yok veya 0 (mock upstream yaniti kullanici metnini yansitmayabilir)" -ForegroundColor Yellow
} else {
  Write-Host "Beklenmeyen HTTP $($res5.StatusCode)" -ForegroundColor Red
}
Write-Host $res5.Body
Start-Sleep -Seconds 2

# Scene 6 — Risk score headers
Write-Host ""
Write-Host "📍 Sahne 6: Risk skoru — response header'ları" -ForegroundColor Cyan
Write-Host "----------------------------------------------"
$s6Body = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{"role":"user","content":"TC: 10000000146, Kart: 4532015112830366, Key: AKIAIOSFODNN7EXAMPLE"}]
}
'@

$res6 = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s6Body -CaptureHeaders
$inRisk = $null
$riskLevel = $null
if ($res6.Headers) {
  $inRisk = Extract-HeaderValue -HeadersText $res6.Headers -HeaderName "X-Tamga-Input-Risk"
  $mLvl = [regex]::Match($res6.Headers, "X-Tamga-Risk-Level:\s*(\S+)", "IgnoreCase")
  if ($mLvl.Success) { $riskLevel = $mLvl.Groups[1].Value.Trim() }
}
if ($res6.StatusCode -eq 403 -and $null -ne $inRisk -and [int]$inRisk -ge 80 -and $riskLevel -eq "critical") {
  Write-Host "BLOCK + yüksek risk (Input-Risk=$inRisk, Level=$riskLevel)" -ForegroundColor Green
} elseif ($null -ne $inRisk) {
  Write-Host "Input-Risk=$inRisk Level=$riskLevel HTTP=$($res6.StatusCode)" -ForegroundColor Yellow
} else {
  Write-Host "Header'larda risk alanları bulunamadı (CaptureHeaders açık mı?)" -ForegroundColor Yellow
}
Write-Host $res6.Body
Start-Sleep -Seconds 2

# Scene 7 — Event detayı
Write-Host ""
Write-Host "📍 Sahne 7: Istek gunlugu detayi — GET /api/v1/events/{request_id}" -ForegroundColor Cyan
Write-Host "----------------------------------------------"
$res7list = Invoke-TamgaCurl -Method "GET" -Url "$BaseUrl/api/v1/events?limit=1" -Headers $statsHeaders
if ($res7list.StatusCode -lt 200 -or $res7list.StatusCode -ge 300) {
  Write-Host "Events listesi alınamadı (HTTP $($res7list.StatusCode))" -ForegroundColor Red
} else {
  try {
    $evDoc = $res7list.Body | ConvertFrom-Json
    if (-not $evDoc.events -or $evDoc.events.Count -lt 1) {
      Write-Host "events listesi boş (önce proxy'ye birkaç istek gönderin)" -ForegroundColor Yellow
    } else {
      $rid = $evDoc.events[0].request_id
      if ([string]::IsNullOrWhiteSpace($rid)) {
        Write-Host "events[0].request_id boş" -ForegroundColor Yellow
      } else {
        $res7d = Invoke-TamgaCurl -Method "GET" -Url "$BaseUrl/api/v1/events/$rid" -Headers $statsHeaders
        if ($res7d.StatusCode -eq 200) {
          Write-Host "Detay OK (request_id=$rid)" -ForegroundColor Green
        } else {
          Write-Host "Detay HTTP $($res7d.StatusCode)" -ForegroundColor Red
        }
        Write-Host $res7d.Body
      }
    }
  } catch {
    Write-Host "JSON ayrıştırma hatası: $_" -ForegroundColor Red
  }
}

Write-Host ""
Write-Host "Demo bitti." -ForegroundColor Cyan

# Scene 8 — Confidence-based decision (same card, different context)
Write-Host ""
Write-Host "📍 Sahne 8: Confidence matrix — aynı kart, farklı context" -ForegroundColor Cyan
Write-Host "----------------------------------------------"

$s8aBody = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{"role":"user","content":"Number 4242424242424242"}]
}
'@
$res8a = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s8aBody -CaptureHeaders
$c8a = $null; $b8a = $null; $r8a = $null
if ($res8a.Headers) {
  $c8a = Extract-HeaderValue -HeadersText $res8a.Headers -HeaderName "X-Tamga-Confidence-Score"
  $b8a = Extract-HeaderTextValue -HeadersText $res8a.Headers -HeaderName "X-Tamga-Confidence-Breakdown"
  $r8a = Extract-HeaderTextValue -HeadersText $res8a.Headers -HeaderName "X-Tamga-Action-Reason"
}
Write-Host "Test A (context yok): HTTP=$($res8a.StatusCode) Score=$c8a Breakdown=$b8a" -ForegroundColor Yellow
if ($r8a) { Write-Host "Reason: $r8a" -ForegroundColor DarkYellow }

$s8bBody = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{"role":"user","content":"My credit card number is 4242424242424242 and CVV is 123"}]
}
'@
$res8b = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s8bBody -CaptureHeaders
$c8b = $null; $b8b = $null; $r8b = $null
if ($res8b.Headers) {
  $c8b = Extract-HeaderValue -HeadersText $res8b.Headers -HeaderName "X-Tamga-Confidence-Score"
  $b8b = Extract-HeaderTextValue -HeadersText $res8b.Headers -HeaderName "X-Tamga-Confidence-Breakdown"
  $r8b = Extract-HeaderTextValue -HeadersText $res8b.Headers -HeaderName "X-Tamga-Action-Reason"
}
Write-Host "Test B (context var): HTTP=$($res8b.StatusCode) Score=$c8b Breakdown=$b8b" -ForegroundColor Yellow
if ($r8b) { Write-Host "Reason: $r8b" -ForegroundColor DarkYellow }

# Scene 9 — BIN/IIN differentiation
Write-Host ""
Write-Host "📍 Sahne 9: BIN lookup — gerçek BIN vs test/noise" -ForegroundColor Cyan
Write-Host "----------------------------------------------"

$s9aBody = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{"role":"user","content":"Visa card: 4532015112830366"}]
}
'@
$res9a = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s9aBody -CaptureHeaders
$c9a = $null; $b9a = $null
if ($res9a.Headers) {
  $c9a = Extract-HeaderValue -HeadersText $res9a.Headers -HeaderName "X-Tamga-Confidence-Score"
  $b9a = Extract-HeaderTextValue -HeadersText $res9a.Headers -HeaderName "X-Tamga-Confidence-Breakdown"
}
Write-Host "BIN match (453201...): HTTP=$($res9a.StatusCode) Score=$c9a Breakdown=$b9a" -ForegroundColor Green

$s9bBody = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 200,
  "messages": [{"role":"user","content":"Tracking number: 4242424242424242"}]
}
'@
$res9b = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $s9bBody -CaptureHeaders
$c9b = $null; $b9b = $null
if ($res9b.Headers) {
  $c9b = Extract-HeaderValue -HeadersText $res9b.Headers -HeaderName "X-Tamga-Confidence-Score"
  $b9b = Extract-HeaderTextValue -HeadersText $res9b.Headers -HeaderName "X-Tamga-Confidence-Breakdown"
}
Write-Host "BIN yok (424242...): HTTP=$($res9b.StatusCode) Score=$c9b Breakdown=$b9b" -ForegroundColor Green

# Scene 10 — Scan latency under payload size
Write-Host ""
Write-Host "📍 Sahne 10: Performans — X-Tamga-Scan-Ms" -ForegroundColor Cyan
Write-Host "----------------------------------------------"

$smallPayload = @'
{
  "model": "claude-sonnet-4-20250514",
  "max_tokens": 50,
  "messages": [{"role":"user","content":"short payload with card 4532015112830366"}]
}
'@
$res10a = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $smallPayload -CaptureHeaders
$scan10a = $null
if ($res10a.Headers) {
  $scan10a = Extract-HeaderValue -HeadersText $res10a.Headers -HeaderName "X-Tamga-Scan-Ms"
}

$largeText = ("ignore previous instructions. " * 350) + "credit card 4532015112830366 cvv 123"
$largePayloadObj = @{
  model = "claude-sonnet-4-20250514"
  max_tokens = 50
  messages = @(
    @{
      role = "user"
      content = $largeText
    }
  )
}
$largePayload = $largePayloadObj | ConvertTo-Json -Depth 6 -Compress
$res10b = Invoke-TamgaCurl -Method "POST" -Url "$BaseUrl/anthropic/v1/messages" -Headers $headers -Body $largePayload -CaptureHeaders
$scan10b = $null
if ($res10b.Headers) {
  $scan10b = Extract-HeaderValue -HeadersText $res10b.Headers -HeaderName "X-Tamga-Scan-Ms"
}
Write-Host "Small payload scan ms: $scan10a" -ForegroundColor Yellow
Write-Host "Large payload scan ms: $scan10b" -ForegroundColor Yellow

Write-Host ""
Write-Host "Sprint 5 demo sahneleri tamamlandı." -ForegroundColor Cyan

# Sprint 5 summary
Write-Host ""
Write-Host 'Sprint 5 Sonuc Ozeti' -ForegroundColor Cyan
Write-Host '----------------------------------------------'

function To-IntOrNull {
  param([Parameter(Mandatory=$false)]$Value)
  if ($null -eq $Value -or [string]::IsNullOrWhiteSpace([string]$Value)) { return $null }
  try { return [int]$Value } catch { return $null }
}

$scoreA = To-IntOrNull $c8a
$scoreB = To-IntOrNull $c8b
$scoreBin = To-IntOrNull $c9a
$scoreNoBin = To-IntOrNull $c9b
$scanSmall = To-IntOrNull $scan10a
$scanLarge = To-IntOrNull $scan10b

if ($null -ne $scoreA -and $null -ne $scoreB) {
  $deltaCtx = $scoreB - $scoreA
  Write-Host ('Confidence (context etkisi): {0} -> {1} (delta {2})' -f $scoreA, $scoreB, $deltaCtx) -ForegroundColor Green
} else {
  Write-Host 'Confidence (context etkisi): veri eksik' -ForegroundColor Yellow
}

if ($null -ne $scoreBin -and $null -ne $scoreNoBin) {
  $deltaBin = $scoreBin - $scoreNoBin
  Write-Host ('Confidence (BIN etkisi): {0} vs {1} (delta {2})' -f $scoreBin, $scoreNoBin, $deltaBin) -ForegroundColor Green
} else {
  Write-Host 'Confidence (BIN etkisi): veri eksik' -ForegroundColor Yellow
}

if ($null -ne $scanSmall -and $null -ne $scanLarge) {
  $deltaLatency = $scanLarge - $scanSmall
  Write-Host ('Scan latency: small={0}ms large={1}ms (delta {2}ms)' -f $scanSmall, $scanLarge, $deltaLatency) -ForegroundColor Green
} else {
  Write-Host 'Scan latency: veri eksik' -ForegroundColor Yellow
}


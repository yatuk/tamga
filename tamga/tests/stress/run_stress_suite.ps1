# ─────────────────────────────────────────────────────────────────────────────
# Tamga Stress Test Suite Runner (Windows PowerShell)
#
# Single-command orchestration:
#   docker compose up → wait healthy → adversarial tests → k6 load tests →
#   collect results → check regression → docker compose down (always)
#
# Usage:
#   .\run_stress_suite.ps1                          # default: 100/500/1000 RPS
#   .\run_stress_suite.ps1 -SkipLoad                # adversarial only
#   .\run_stress_suite.ps1 -SkipAdversarial         # load only
#   .\run_stress_suite.ps1 -Rps @("100")            # single RPS level
#
# Exit codes:
#   0 — all tests passed, no regression
#   1 — regression detected
#   2 — infrastructure error
# ─────────────────────────────────────────────────────────────────────────────

$ErrorActionPreference = "Stop"

$ScriptDir = Split-Path -Parent $MyInvocation.MyCommand.Path
$ProjectRoot = Resolve-Path "$ScriptDir\..\.."
$ComposeFile = "$ProjectRoot\deploy\docker-compose.yml"
$ResultsBase = "$ScriptDir\results"
$BaselineFile = "$ScriptDir\baseline.json"
$Timestamp = Get-Date -Format "yyyyMMdd-HHmmss"
$ResultsDir = "$ResultsBase\$Timestamp"

$HealthUrl = if ($env:TAMGA_HEALTH_URL) { $env:TAMGA_HEALTH_URL } else { "http://localhost:8443/api/v1/health" }
$HealthTimeout = if ($env:TAMGA_HEALTH_TIMEOUT) { [int]$env:TAMGA_HEALTH_TIMEOUT } else { 60 }
$K6Bin = if ($env:K6_BIN) { $env:K6_BIN } else { "k6" }
$PythonBin = if ($env:PYTHON_BIN) { $env:PYTHON_BIN } else { "python" }
$ProxyUrl = if ($env:TAMGA_BASE_URL) { $env:TAMGA_BASE_URL } else { "http://localhost:8443" }
$ApiKey = if ($env:TAMGA_API_KEY) { $env:TAMGA_API_KEY } else { "test-key" }

$SkipLoad = $false
$SkipAdversarial = $false
$RpsLevels = @("100", "500", "1000")
$WorkloadDuration = "180s"

# ── arg parsing ─────────────────────────────────────────────────────────────

for ($i = 0; $i -lt $args.Count; $i++) {
    switch ($args[$i]) {
        "-SkipLoad"         { $SkipLoad = $true }
        "-SkipAdversarial"  { $SkipAdversarial = $true }
        "-Rps"              { $RpsLevels = $args[++$i] }
        "-HealthUrl"        { $HealthUrl = $args[++$i] }
        default             { Write-Host "Unknown flag: $($args[$i])"; exit 2 }
    }
}

# ── pre-flight ──────────────────────────────────────────────────────────────

Write-Host "[$(Get-Date -Format 'HH:mm:ss')] Tamga Stress Test Suite" -ForegroundColor Cyan
Write-Host "  Results dir : $ResultsDir"
Write-Host "  Baseline    : $BaselineFile"
Write-Host "  Compose file: $ComposeFile"
Write-Host ""

if (-not (Get-Command docker -ErrorAction SilentlyContinue)) {
    Write-Host "ERROR: docker not found in PATH" -ForegroundColor Red
    exit 2
}

if (-not $SkipLoad -and -not (Get-Command $K6Bin -ErrorAction SilentlyContinue)) {
    Write-Host "ERROR: k6 not found. Install: https://k6.io/docs/get-started/installation/" -ForegroundColor Red
    Write-Host "  or run with -SkipLoad"
    exit 2
}

if (-not (Test-Path $ComposeFile)) {
    Write-Host "ERROR: docker-compose.yml not found at $ComposeFile" -ForegroundColor Red
    exit 2
}

New-Item -ItemType Directory -Force -Path $ResultsDir | Out-Null

# ── cleanup trap ────────────────────────────────────────────────────────────

$CleanupScript = {
    $exitCode = $LASTEXITCODE
    Write-Host "[$(Get-Date -Format 'HH:mm:ss')] Cleaning up... (exit=$exitCode)" -ForegroundColor Yellow
    Push-Location $ProjectRoot
    docker compose -f $ComposeFile down --timeout 30 2>$null
    Pop-Location
    if ($exitCode -eq 0) {
        Write-Host "Stress suite complete — exit 0" -ForegroundColor Green
    } elseif ($exitCode -eq 1) {
        Write-Host "Stress suite complete — REGRESSION DETECTED (exit 1)" -ForegroundColor Red
    } else {
        Write-Host "Stress suite complete — INFRA ERROR (exit $exitCode)" -ForegroundColor Red
    }
    exit $exitCode
}

# Register trap for common termination signals
try { Register-EngineEvent -SourceIdentifier PowerShell.Exiting -Action $CleanupScript | Out-Null } catch { }

# ── infrastructure ──────────────────────────────────────────────────────────

Write-Host "[$(Get-Date -Format 'HH:mm:ss')] Starting Tamga stack..." -ForegroundColor Cyan
Push-Location $ProjectRoot
docker compose -f $ComposeFile up -d --wait 2>&1 | Out-Null
Pop-Location

Write-Host "[$(Get-Date -Format 'HH:mm:ss')] Waiting for proxy health ($HealthTimeout seconds)..." -ForegroundColor Cyan
$sw = [System.Diagnostics.Stopwatch]::StartNew()
while ($true) {
    try {
        $null = Invoke-WebRequest -Uri $HealthUrl -UseBasicParsing -TimeoutSec 3
        Write-Host "  ✓ Proxy healthy at $HealthUrl" -ForegroundColor Green
        break
    } catch {
        if ($sw.Elapsed.TotalSeconds -ge $HealthTimeout) {
            Write-Host "  ✗ Health check timed out after ${HealthTimeout}s" -ForegroundColor Red
            & $CleanupScript
            exit 2
        }
        Start-Sleep -Seconds 2
    }
}
Start-Sleep -Seconds 3

# ── adversarial tests ───────────────────────────────────────────────────────

if (-not $SkipAdversarial) {
    Write-Host "[$(Get-Date -Format 'HH:mm:ss')] Running adversarial bypass tests..." -ForegroundColor Cyan
    $AdvDir = "$ScriptDir\adversarial"

    foreach ($scriptName in @("pii_bypass.py", "injection_bypass.py", "secret_bypass.py", "policy_bypass.py")) {
        $scriptPath = "$AdvDir\$scriptName"
        if (-not (Test-Path $scriptPath)) {
            Write-Host "  ⚠ Skipping $scriptName (not found)"
            continue
        }
        Write-Host "  → $scriptName" -ForegroundColor Gray
        $outFile = "$ResultsDir\adversarial_$($scriptName.Replace('.py', '.json'))"
        $exitCode = 0
        & $PythonBin $scriptPath --json --output-dir $ResultsDir *> $outFile
        $exitCode = $LASTEXITCODE
        if ($exitCode -ne 0) {
            Write-Host "  ⚠ $scriptName exited non-zero (bypasses found — this is expected)"
        }
        Write-Host "  ✓ $scriptName complete" -ForegroundColor Green
    }

    # Merge adversarial results
    $mergeScript = @"
import json, pathlib
results = {}
for f in sorted(pathlib.Path(r'$ResultsDir').glob('adversarial_*.json')):
    try:
        data = json.loads(f.read_text())
        cat = data.get('category', f.stem.replace('adversarial_', ''))
        results[cat] = {
            'total': data.get('total', 0),
            'detected': data.get('detected', 0),
            'bypassed': data.get('bypassed', 0),
        }
    except: pass
total_bypassed = sum(r['bypassed'] for r in results.values())
results['total_bypassed'] = total_bypassed
json.dump(results, open(r'$ResultsDir\adversarial_results.json', 'w'), indent=2)
print(f'Adversarial complete: {total_bypassed} total bypassed across {len(results)-1} categories')
"@
    & $PythonBin -c $mergeScript
}

# ── k6 load tests ───────────────────────────────────────────────────────────

if (-not $SkipLoad) {
    Write-Host "[$(Get-Date -Format 'HH:mm:ss')] Running k6 load tests..." -ForegroundColor Cyan
    $K6Dir = "$ScriptDir\k6"

    foreach ($rps in $RpsLevels) {
        $scriptFile = "$K6Dir\baseline_${rps}rps.js"
        if (-not (Test-Path $scriptFile)) {
            Write-Host "  ⚠ Skipping ${rps}rps (script not found)"
            continue
        }
        Write-Host "  → ${rps} RPS baseline" -ForegroundColor Gray
        $env:TAMGA_BASE_URL = $ProxyUrl
        $env:TAMGA_API_KEY = $ApiKey
        & $K6Bin run --summary-export="$ResultsDir\load_test_${rps}rps.json" $scriptFile 2>&1 | Out-Null
        Write-Host "  ✓ ${rps} RPS complete" -ForegroundColor Green
    }

    $workloadFile = "$K6Dir\workload_mix.js"
    if (Test-Path $workloadFile) {
        Write-Host "  → workload_mix (${WorkloadDuration})" -ForegroundColor Gray
        $env:TAMGA_BASE_URL = $ProxyUrl
        $env:TAMGA_API_KEY = $ApiKey
        & $K6Bin run --duration $WorkloadDuration --summary-export="$ResultsDir\workload_mix.json" $workloadFile 2>&1 | Out-Null
        Write-Host "  ✓ workload_mix complete" -ForegroundColor Green
    }
}

# ── regression check ────────────────────────────────────────────────────────

Write-Host "[$(Get-Date -Format 'HH:mm:ss')] Running regression check..." -ForegroundColor Cyan
& $PythonBin "$ScriptDir\check_regression.py" --results-dir "$ResultsDir" --baseline "$BaselineFile"
$RegressionExit = $LASTEXITCODE

# ── done ────────────────────────────────────────────────────────────────────

Write-Host ""
if ($RegressionExit -eq 0) {
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Green
    Write-Host "  STRESS SUITE PASSED — No regression detected" -ForegroundColor Green
    Write-Host "  Results: $ResultsDir" -ForegroundColor Green
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Green
} else {
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Red
    Write-Host "  STRESS SUITE FAILED — Regression detected" -ForegroundColor Red
    Write-Host "  Results: $ResultsDir" -ForegroundColor Red
    Write-Host "━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━" -ForegroundColor Red
}

& $CleanupScript
exit $RegressionExit

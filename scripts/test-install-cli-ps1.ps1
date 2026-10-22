# Runs install-cli.ps1 end to end on a Windows runner:
#   .\scripts\test-install-cli-ps1.ps1 -Binary .\levelrail-cli-windows-amd64.exe
# Covers install, PATH (registry and session), re-run, checksum verification
# against a local http mirror, and uninstall. Meant for CI, it edits the
# current user's PATH and installs under %LOCALAPPDATA%.
[CmdletBinding()]
param(
    [Parameter(Mandatory = $true)][string]$Binary
)

Set-StrictMode -Version Latest
# Continue, not Stop: Windows PowerShell 5.1 turns native stderr into a terminating error under Stop.
$ErrorActionPreference = 'Continue'

$repoRoot = Split-Path -Parent $PSScriptRoot
$script = Join-Path $repoRoot 'install-cli.ps1'
$Binary = (Resolve-Path -LiteralPath $Binary).Path
$psExe = (Get-Process -Id $PID).Path
$installDir = Join-Path $env:LOCALAPPDATA 'Levelrail\bin'
$exe = Join-Path $installDir 'levelrail-cli.exe'
$work = Join-Path ([IO.Path]::GetTempPath()) ("lr-install-test-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $work | Out-Null
$server = $null

function Fail([string]$Message) { throw "FAIL: $Message" }

function Get-UserPath {
    $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment')
    try { return [string]$key.GetValue('Path', '', 'DoNotExpandEnvironmentNames') } finally { $key.Close() }
}

function Count-PathEntries([string]$PathValue) {
    return @($PathValue.Split(';') | Where-Object { $_.TrimEnd('\') -ieq $installDir }).Count
}

# Runs the installer in a fresh process so error output and exit codes are real.
function Invoke-Installer([string[]]$ScriptArgs = @(), [switch]$ViaIex) {
    if ($ViaIex) {
        $cmd = "Get-Content -Raw -LiteralPath '$script' | Invoke-Expression"
        return (& $psExe -NoProfile -NonInteractive -Command $cmd 2>&1 | Out-String)
    }
    return (& $psExe -NoProfile -NonInteractive -File $script @ScriptArgs 2>&1 | Out-String)
}

try {
    $env:LEVELRAIL_BINARY_FILE = $Binary

    Write-Host '== install via the irm | iex path'
    $out = Invoke-Installer -ViaIex
    if (-not (Test-Path -LiteralPath $exe)) { Fail "binary not installed at ${exe}: $out" }
    if ((Count-PathEntries (Get-UserPath)) -ne 1) { Fail "user PATH should hold $installDir once: $(Get-UserPath)" }
    & $exe version *> $null
    if ($LASTEXITCODE -ne 0) { Fail 'installed levelrail-cli.exe did not run' }

    Write-Host '== re-run is idempotent and does not duplicate PATH'
    $out = Invoke-Installer
    if ($out -notmatch 'already on your user PATH') { Fail "second run did not report PATH as present: $out" }
    if ((Count-PathEntries (Get-UserPath)) -ne 1) { Fail 'PATH entry was duplicated' }

    Write-Host '== release verification against a local http mirror'
    $arch = if ($env:PROCESSOR_ARCHITECTURE -eq 'ARM64') { 'arm64' } else { 'amd64' }
    $asset = "levelrail-cli-windows-$arch.exe"
    foreach ($d in 'good', 'tampered') {
        $dir = Join-Path $work "rel\v9.9.9\$d"
        New-Item -ItemType Directory -Path $dir -Force | Out-Null
        Copy-Item -LiteralPath $Binary -Destination (Join-Path $dir $asset)
    }
    $hash = (Get-FileHash -LiteralPath (Join-Path $work "rel\v9.9.9\good\$asset") -Algorithm SHA256).Hash.ToLower()
    Set-Content -LiteralPath (Join-Path $work 'rel\v9.9.9\good\checksums.txt') -Value "$hash  $asset"
    Set-Content -LiteralPath (Join-Path $work 'rel\v9.9.9\tampered\checksums.txt') -Value ("0" * 64 + "  $asset")
    $server = Start-Process -FilePath python -ArgumentList @('-m', 'http.server', '8099', '--bind', '127.0.0.1', '--directory', (Join-Path $work 'rel')) -PassThru -WindowStyle Hidden
    Start-Sleep -Seconds 2

    Remove-Item Env:LEVELRAIL_BINARY_FILE
    $env:LEVELRAIL_VERSION = 'v9.9.9'
    $env:LEVELRAIL_INSECURE_MIRROR = '1'
    $env:LEVELRAIL_SKIP_PATH = '1'

    $env:LEVELRAIL_RELEASE_BASE_URL = 'http://127.0.0.1:8099/v9.9.9/tampered'
    $out = Invoke-Installer
    if ($out -notmatch 'checksum mismatch') { Fail "tampered checksums.txt was not rejected: $out" }

    $env:LEVELRAIL_RELEASE_BASE_URL = 'http://127.0.0.1:8099/v9.9.9/good'
    $out = Invoke-Installer
    if ($out -notmatch 'Checksum verified') { Fail "good checksum was not accepted: $out" }

    Remove-Item Env:LEVELRAIL_INSECURE_MIRROR
    $out = Invoke-Installer
    if ($out -notmatch 'must be https') { Fail "http mirror was accepted without LEVELRAIL_INSECURE_MIRROR: $out" }

    if (-not (Get-Command cosign -ErrorAction SilentlyContinue)) {
        $env:LEVELRAIL_INSECURE_MIRROR = '1'
        $env:APP_INSTALL_VERIFY = 'require'
        $out = Invoke-Installer
        if ($out -notmatch 'cosign is not installed') { Fail "require mode without cosign did not fail closed: $out" }
        Remove-Item Env:APP_INSTALL_VERIFY
    }

    foreach ($name in 'LEVELRAIL_VERSION', 'LEVELRAIL_INSECURE_MIRROR', 'LEVELRAIL_SKIP_PATH', 'LEVELRAIL_RELEASE_BASE_URL') {
        Remove-Item "Env:$name" -ErrorAction SilentlyContinue
    }

    Write-Host '== uninstall removes the binary and the PATH entry'
    $out = Invoke-Installer -ScriptArgs @('-Uninstall')
    if (Test-Path -LiteralPath $exe) { Fail "binary still present after uninstall: $out" }
    if ((Count-PathEntries (Get-UserPath)) -ne 0) { Fail 'PATH entry still present after uninstall' }

    Write-Host 'install-cli.ps1 passed on Windows'
} finally {
    if ($server) { Stop-Process -Id $server.Id -Force -ErrorAction SilentlyContinue }
    Remove-Item -LiteralPath $work -Recurse -Force -ErrorAction SilentlyContinue
}

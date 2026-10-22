# irm https://levelrail.com/install-cli.ps1 | iex
#
# Installs levelrail-cli on Windows, the client for an already-running
# Levelrail control plane. Per-user install, never needs admin.
#
# Usage (as a file): .\install-cli.ps1 [-Version v0.3.0] [-Uninstall]
#   -Version    release tag to install (same as LEVELRAIL_VERSION)
#   -Uninstall  remove the binary and its PATH entry
#
# Env overrides (the only way to pass options through "irm | iex"):
#   LEVELRAIL_VERSION          release tag to install, e.g. v0.3.0
#   LEVELRAIL_CHANNEL          stable or beta, same meaning as install-cli.sh
#   LEVELRAIL_CLI_INSTALL_DIR  where the binary goes
#                              (default: $env:LOCALAPPDATA\Levelrail\bin)
#   LEVELRAIL_BINARY_FILE      install this local binary instead of downloading
#   LEVELRAIL_SKIP_CHECKSUM=1  install even when checksums.txt is missing or
#                              does not list the binary (unverified; opt-out only)
#   APP_INSTALL_VERIFY         signature check on checksums.txt (cosign keyless):
#                              auto, require, or off
#   LEVELRAIL_RELEASE_BASE_URL download release assets from this https mirror
#   LEVELRAIL_INSECURE_MIRROR=1  allow an http mirror (tests only)
#   LEVELRAIL_SKIP_PATH=1      do not touch PATH (tests only)

[CmdletBinding()]
param(
    [string]$Version,
    [switch]$Uninstall
)

function Invoke-LevelrailCliInstall {
    [CmdletBinding()]
    param(
        [string]$Version,
        [switch]$Uninstall
    )

    # Scoped to this function so "irm | iex" does not leave strict mode on in the caller's session.
    Set-StrictMode -Version Latest
    $ErrorActionPreference = 'Stop'
    $ProgressPreference = 'SilentlyContinue'

    $repo = 'glincker/levelrail'
    $binaryName = 'levelrail-cli'
    $exeName = "$binaryName.exe"
    $cosignIdentity = "https://github.com/$repo/\.github/workflows/release\.yml@.*"
    $cosignIssuer = 'https://token.actions.githubusercontent.com'

    function Get-EnvValue([string]$Name) {
        $v = [Environment]::GetEnvironmentVariable($Name, 'Process')
        if ($null -eq $v) { return '' }
        return $v
    }

    function Write-Info([string]$Message) { Write-Host $Message }
    function Write-Warn([string]$Message) { Write-Warning $Message }

    function Stop-Install([string]$Message) { throw $Message }

    # TLS 1.2 is not the default on Windows PowerShell 5.1.
    try {
        [Net.ServicePointManager]::SecurityProtocol = [Net.ServicePointManager]::SecurityProtocol -bor [Net.SecurityProtocolType]::Tls12
    } catch {
        Write-Verbose 'could not force TLS 1.2, relying on the system default'
    }

    $installDir = Get-EnvValue 'LEVELRAIL_CLI_INSTALL_DIR'
    if (-not $installDir) {
        $localAppData = Get-EnvValue 'LOCALAPPDATA'
        if (-not $localAppData) { Stop-Install 'LOCALAPPDATA is not set. Set LEVELRAIL_CLI_INSTALL_DIR to a folder you can write to.' }
        $installDir = Join-Path $localAppData 'Levelrail\bin'
    }
    $binPath = Join-Path $installDir $exeName
    $verifyMode = Get-EnvValue 'APP_INSTALL_VERIFY'
    if (-not $verifyMode) { $verifyMode = 'auto' }
    if (@('auto', 'require', 'off') -notcontains $verifyMode) {
        Stop-Install "APP_INSTALL_VERIFY must be auto, require or off, got: $verifyMode"
    }

    function ConvertTo-PathKey([string]$Entry) {
        $expanded = [Environment]::ExpandEnvironmentVariables($Entry.Trim())
        return $expanded.TrimEnd('\').ToLowerInvariant()
    }

    function Get-UserPathRaw {
        $key = [Microsoft.Win32.Registry]::CurrentUser.OpenSubKey('Environment', $false)
        if ($null -eq $key) { return '' }
        try {
            $value = $key.GetValue('Path', '', [Microsoft.Win32.RegistryValueOptions]::DoNotExpandEnvironmentNames)
            if ($null -eq $value) { return '' }
            return [string]$value
        } finally { $key.Close() }
    }

    function Set-UserPathRaw([string]$Value) {
        $key = [Microsoft.Win32.Registry]::CurrentUser.CreateSubKey('Environment')
        try {
            $key.SetValue('Path', $Value, [Microsoft.Win32.RegistryValueKind]::ExpandString)
        } finally { $key.Close() }
        Send-EnvironmentChange
    }

    # Tells Explorer and new terminals to re-read the environment without a logoff.
    function Send-EnvironmentChange {
        try {
            if (-not ('Levelrail.NativeMethods' -as [type])) {
                Add-Type -Namespace Levelrail -Name NativeMethods -MemberDefinition @'
[System.Runtime.InteropServices.DllImport("user32.dll", SetLastError = true, CharSet = System.Runtime.InteropServices.CharSet.Auto)]
public static extern System.IntPtr SendMessageTimeout(System.IntPtr hWnd, uint Msg, System.UIntPtr wParam, string lParam, uint fuFlags, uint uTimeout, out System.UIntPtr lpdwResult);
'@
            }
            $result = [UIntPtr]::Zero
            [void][Levelrail.NativeMethods]::SendMessageTimeout([IntPtr]0xffff, 0x1A, [UIntPtr]::Zero, 'Environment', 2, 5000, [ref]$result)
        } catch {
            Write-Verbose "could not broadcast the environment change: $($_.Exception.Message)"
        }
    }

    function Test-PathContains([string]$PathValue, [string]$Dir) {
        if (-not $PathValue) { return $false }
        $want = ConvertTo-PathKey $Dir
        foreach ($entry in $PathValue.Split(';')) {
            if ($entry.Trim() -and (ConvertTo-PathKey $entry) -eq $want) { return $true }
        }
        return $false
    }

    function Add-ToPath([string]$Dir) {
        if ((Get-EnvValue 'LEVELRAIL_SKIP_PATH') -eq '1') { return 'skipped' }
        $changed = 'already'
        $userPath = Get-UserPathRaw
        if (-not (Test-PathContains $userPath $Dir)) {
            $newValue = if ($userPath) { $userPath.TrimEnd(';') + ';' + $Dir } else { $Dir }
            Set-UserPathRaw $newValue
            $changed = 'added'
        }
        if (-not (Test-PathContains $env:Path $Dir)) {
            $env:Path = $env:Path.TrimEnd(';') + ';' + $Dir
        }
        return $changed
    }

    function Remove-FromPath([string]$Dir) {
        if ((Get-EnvValue 'LEVELRAIL_SKIP_PATH') -eq '1') { return }
        $want = ConvertTo-PathKey $Dir
        $userPath = Get-UserPathRaw
        if (Test-PathContains $userPath $Dir) {
            $kept = $userPath.Split(';') | Where-Object { $_.Trim() -and (ConvertTo-PathKey $_) -ne $want }
            Set-UserPathRaw ($kept -join ';')
        }
        $sessionKept = $env:Path.Split(';') | Where-Object { $_.Trim() -and (ConvertTo-PathKey $_) -ne $want }
        $env:Path = $sessionKept -join ';'
    }

    function Get-GoArch {
        $arch = Get-EnvValue 'PROCESSOR_ARCHITEW6432'
        if (-not $arch) { $arch = Get-EnvValue 'PROCESSOR_ARCHITECTURE' }
        switch ($arch.ToUpperInvariant()) {
            'AMD64' { return 'amd64' }
            'ARM64' { return 'arm64' }
            default { Stop-Install "unsupported architecture: $arch. Published builds cover amd64 and arm64." }
        }
    }

    function Invoke-Download([string]$Url, [string]$Dest, [int]$TimeoutSec) {
        Invoke-WebRequest -UseBasicParsing -Uri $Url -OutFile $Dest -TimeoutSec $TimeoutSec | Out-Null
    }

    function Resolve-Version([string]$Asset) {
        $pinned = if ($Version) { $Version } else { Get-EnvValue 'LEVELRAIL_VERSION' }
        if ($pinned) { return $pinned }
        $channel = Get-EnvValue 'LEVELRAIL_CHANNEL'
        if (@('', 'stable', 'beta') -notcontains $channel) {
            Stop-Install "LEVELRAIL_CHANNEL must be stable or beta, got: $channel"
        }
        try {
            $releases = Invoke-RestMethod -UseBasicParsing -TimeoutSec 30 -Headers @{ Accept = 'application/vnd.github+json' } `
                -Uri "https://api.github.com/repos/$repo/releases?per_page=30"
        } catch {
            Stop-Install "could not list releases from GitHub ($($_.Exception.Message)). Set LEVELRAIL_VERSION=vX.Y.Z to pin one."
        }
        $shipping = @($releases | Where-Object {
                -not $_.draft -and ($_.assets | Where-Object { $_.name -eq $Asset })
            } | Sort-Object { [datetime]$_.created_at } -Descending)
        $stable = $shipping | Where-Object { -not $_.prerelease } | Select-Object -First 1
        $any = $shipping | Select-Object -First 1
        $pick = switch ($channel) {
            'stable' { $stable }
            'beta' { $any }
            default { if ($stable) { $stable } else { $any } }
        }
        if (-not $pick) {
            Stop-Install "no release ships $Asset. Set LEVELRAIL_VERSION=vX.Y.Z to pin one."
        }
        if ($pick.prerelease -and -not $channel) {
            Write-Info "No stable release with a $Asset binary yet, using pre-release $($pick.tag_name)."
        }
        return [string]$pick.tag_name
    }

    function Confirm-Signature([string]$SumsFile, [string]$BaseUrl, [string]$VersionTag) {
        if ($verifyMode -eq 'off') { return }
        $cosign = Get-Command cosign -ErrorAction SilentlyContinue
        if (-not $cosign) {
            if ($verifyMode -eq 'require') {
                Stop-Install 'APP_INSTALL_VERIFY=require but cosign is not installed. Install it from https://docs.sigstore.dev/cosign/system_config/installation/ or unset APP_INSTALL_VERIFY.'
            }
            Write-Info 'cosign not found: verifying the SHA-256 checksum only (install cosign to also verify the release signature).'
            return
        }
        $bundle = "$SumsFile.sigstore.json"
        try {
            Invoke-Download "$BaseUrl/checksums.txt.sigstore.json" $bundle 60
        } catch {
            if ($verifyMode -eq 'require') { Stop-Install "APP_INSTALL_VERIFY=require but $VersionTag publishes no checksums.txt.sigstore.json" }
            Write-Warn "$VersionTag publishes no signature for checksums.txt, verifying the SHA-256 checksum only"
            return
        }
        & $cosign.Source verify-blob --bundle $bundle --certificate-identity-regexp $cosignIdentity --certificate-oidc-issuer $cosignIssuer $SumsFile *> $null
        if ($LASTEXITCODE -ne 0) {
            Stop-Install "cosign signature verification of checksums.txt failed for $VersionTag, refusing to install"
        }
        Write-Info 'Signature verified (cosign, keyless).'
    }

    function Stop-ChecksumUnavailable([string]$Reason) {
        if ($verifyMode -eq 'require') {
            Stop-Install "$Reason, and APP_INSTALL_VERIFY=require does not allow an unverified install"
        }
        if ((Get-EnvValue 'LEVELRAIL_SKIP_CHECKSUM') -eq '1') {
            Write-Warn "$Reason, skipping verification because LEVELRAIL_SKIP_CHECKSUM=1"
            return
        }
        Stop-Install "$Reason, refusing to install an unverified binary. Pick another release with LEVELRAIL_VERSION, or set LEVELRAIL_SKIP_CHECKSUM=1 to install without verification."
    }

    # Returns the version label; the binary lands at $Dest.
    function Get-Binary([string]$Dest, [string]$Asset) {
        $local = Get-EnvValue 'LEVELRAIL_BINARY_FILE'
        if ($local) {
            if (-not (Test-Path -LiteralPath $local -PathType Leaf)) { Stop-Install "LEVELRAIL_BINARY_FILE not found: $local" }
            Write-Info "Using local binary $local."
            Copy-Item -LiteralPath $local -Destination $Dest -Force
            $pinned = if ($Version) { $Version } else { Get-EnvValue 'LEVELRAIL_VERSION' }
            if ($pinned) { return $pinned } else { return 'local build' }
        }

        $tag = Resolve-Version $Asset
        $baseUrl = Get-EnvValue 'LEVELRAIL_RELEASE_BASE_URL'
        if (-not $baseUrl) { $baseUrl = "https://github.com/$repo/releases/download/$tag" }
        $baseUrl = $baseUrl.TrimEnd('/')
        if ($baseUrl -like 'http://*') {
            if ((Get-EnvValue 'LEVELRAIL_INSECURE_MIRROR') -ne '1') {
                Stop-Install 'LEVELRAIL_RELEASE_BASE_URL must be https (LEVELRAIL_INSECURE_MIRROR=1 allows http for tests)'
            }
        } elseif ($baseUrl -notlike 'https://*') {
            Stop-Install 'LEVELRAIL_RELEASE_BASE_URL must start with https://'
        }

        Write-Info "Downloading $Asset $tag..."
        try {
            Invoke-Download "$baseUrl/$Asset" $Dest 120
        } catch {
            Stop-Install "download failed: $baseUrl/$Asset ($($_.Exception.Message)). The release may not ship this binary; pick another with LEVELRAIL_VERSION."
        }

        $sums = "$Dest.checksums"
        $haveSums = $true
        try { Invoke-Download "$baseUrl/checksums.txt" $sums 60 } catch { $haveSums = $false }
        if (-not $haveSums) {
            Stop-ChecksumUnavailable "no checksums.txt published for $tag"
            return $tag
        }
        Confirm-Signature $sums $baseUrl $tag
        $expected = $null
        foreach ($line in Get-Content -LiteralPath $sums) {
            $parts = $line.Trim() -split '\s+'
            if ($parts.Count -ge 2 -and $parts[1].TrimStart('*') -eq $Asset) { $expected = $parts[0]; break }
        }
        if (-not $expected) {
            Stop-ChecksumUnavailable "$Asset is not listed in checksums.txt for $tag"
            return $tag
        }
        $actual = (Get-FileHash -LiteralPath $Dest -Algorithm SHA256).Hash
        if ($expected.ToLowerInvariant() -ne $actual.ToLowerInvariant()) {
            Stop-Install "checksum mismatch for ${Asset}: expected $expected, got $actual"
        }
        Write-Info 'Checksum verified.'
        return $tag
    }

    if ($Uninstall) {
        $removed = $false
        if (Test-Path -LiteralPath $binPath) {
            try {
                Remove-Item -LiteralPath $binPath -Force
            } catch {
                Stop-Install "could not remove ${binPath}: $($_.Exception.Message). Close any terminal running $binaryName and try again."
            }
            $removed = $true
            Write-Info "Removed $binPath."
        } else {
            Write-Info "$binPath not found, nothing to remove."
        }
        if ((Test-Path -LiteralPath $installDir) -and -not (Get-ChildItem -LiteralPath $installDir -Force)) {
            Remove-Item -LiteralPath $installDir -Force
        }
        Remove-FromPath $installDir
        Write-Info "Removed $installDir from your user PATH (if it was there)."
        if ($removed) { Write-Info "Your saved login stays in %APPDATA%; delete it by hand if you want it gone." }
        return
    }

    $goArch = Get-GoArch
    $asset = "$binaryName-windows-$goArch.exe"

    if (-not (Test-Path -LiteralPath $installDir)) {
        New-Item -ItemType Directory -Path $installDir -Force | Out-Null
    }
    $tmpDir = Join-Path ([IO.Path]::GetTempPath()) ("levelrail-cli-" + [Guid]::NewGuid().ToString('N'))
    New-Item -ItemType Directory -Path $tmpDir -Force | Out-Null
    try {
        $tmpBin = Join-Path $tmpDir $exeName
        $tag = Get-Binary $tmpBin $asset
        try {
            Move-Item -LiteralPath $tmpBin -Destination $binPath -Force
        } catch {
            Stop-Install "could not write ${binPath}: $($_.Exception.Message). If $binaryName is running, close it and try again."
        }
    } finally {
        Remove-Item -LiteralPath $tmpDir -Recurse -Force -ErrorAction SilentlyContinue
    }
    Write-Info "Installed $binPath ($tag)."

    $pathState = Add-ToPath $installDir
    switch ($pathState) {
        'added' { Write-Info "Added $installDir to your user PATH. New terminals pick it up; this one already has it." }
        'already' { Write-Info "$installDir is already on your user PATH." }
        default { Write-Info "Left PATH alone (LEVELRAIL_SKIP_PATH=1). Add $installDir to it yourself." }
    }

    Write-Info ''
    Write-Info 'Next: point it at your running instance and log in.'
    Write-Info '  $env:APP_API_URL = "https://your-dashboard-domain"'
    Write-Info "  $binaryName auth login --device"
    Write-Info ''
    Write-Info 'To uninstall, run in PowerShell:'
    Write-Info "  & ([scriptblock]::Create((irm https://levelrail.com/install-cli.ps1))) -Uninstall"
    Write-Info "or delete $binPath and remove $installDir from your user PATH."
}

$levelrailFailed = $false
try {
    Invoke-LevelrailCliInstall -Version $Version -Uninstall:$Uninstall
} catch {
    $levelrailFailed = $true
    [Console]::Error.WriteLine("error: $($_.Exception.Message)")
}
# Under "irm | iex" there is no script path, and exit would close the user's terminal.
if ($levelrailFailed -and $MyInvocation.MyCommand.Path) { exit 1 }

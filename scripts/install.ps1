# ly Windows installer
#
#   irm https://raw.githubusercontent.com/Din-Studio/lingying-cli/main/scripts/install.ps1 | iex
#
# Downloads the current Windows release, verifies its SHA-256 checksum, and
# installs ly.exe for the current user without requiring Node.js or npm.

[CmdletBinding()]
param(
    [string]$InstallDir = $env:LY_INSTALL_DIR
)

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$ProgramName = 'ly'
$Repository = 'Din-Studio/lingying-cli'
$ReleaseBase = "https://github.com/$Repository/releases/download"
$ManifestUrl = "https://raw.githubusercontent.com/$Repository/main/scripts/version.json"

function Write-InstallMessage {
    param([string]$Message)
    Write-Host "ly-installer: $Message"
}

function Stop-Install {
    param([string]$Message)
    throw "ly-installer: $Message"
}

function Download-File {
    param(
        [Parameter(Mandatory = $true)][string]$Uri,
        [Parameter(Mandatory = $true)][string]$Destination
    )

    $requestParameters = @{
        Uri                = $Uri
        OutFile            = $Destination
        MaximumRedirection = 5
    }
    if ((Get-Command Invoke-WebRequest).Parameters.ContainsKey('UseBasicParsing')) {
        $requestParameters.UseBasicParsing = $true
    }
    Invoke-WebRequest @requestParameters
}

function Get-WindowsArchitecture {
    # PROCESSOR_ARCHITEW6432 identifies the native architecture when this
    # installer is started from a 32-bit PowerShell process on 64-bit Windows.
    $rawArchitecture = $env:PROCESSOR_ARCHITEW6432
    if ([string]::IsNullOrWhiteSpace($rawArchitecture)) {
        $rawArchitecture = $env:PROCESSOR_ARCHITECTURE
    }

    switch ($rawArchitecture.ToUpperInvariant()) {
        'AMD64' { return 'amd64' }
        'ARM64' { return 'arm64' }
        default { Stop-Install "unsupported Windows architecture: $rawArchitecture" }
    }
}

function Get-ExpectedChecksum {
    param(
        [Parameter(Mandatory = $true)][string]$ChecksumFile,
        [Parameter(Mandatory = $true)][string]$AssetName
    )

    foreach ($line in Get-Content -LiteralPath $ChecksumFile) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -ne 2) {
            continue
        }
        $hash = $parts[0]
        $name = $parts[1].Trim().TrimStart('*')
        if ($name -eq $AssetName) {
            if ($hash -notmatch '^[0-9a-fA-F]{64}$') {
                Stop-Install "invalid SHA-256 checksum for $AssetName"
            }
            return $hash.ToLowerInvariant()
        }
    }
    Stop-Install "checksums.txt does not contain $AssetName"
}

function Add-InstallDirectoryToUserPath {
    param([Parameter(Mandatory = $true)][string]$Directory)

    $normalizedDirectory = [IO.Path]::GetFullPath($Directory).TrimEnd('\\')
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @()
    if (-not [string]::IsNullOrWhiteSpace($userPath)) {
        $entries = @($userPath -split ';' | Where-Object { -not [string]::IsNullOrWhiteSpace($_) })
    }
    $alreadyPresent = $entries | Where-Object { $_.TrimEnd('\\') -ieq $normalizedDirectory }
    if (-not $alreadyPresent) {
        $newPath = @($entries + $normalizedDirectory) -join ';'
        [Environment]::SetEnvironmentVariable('Path', $newPath, 'User')
        Write-InstallMessage "added $normalizedDirectory to the user PATH"
    }

    $sessionEntries = @($env:Path -split ';')
    $sessionHasDirectory = $sessionEntries | Where-Object { $_.TrimEnd('\\') -ieq $normalizedDirectory }
    if (-not $sessionHasDirectory) {
        $env:Path = "$normalizedDirectory;$env:Path"
    }
}

if ($env:OS -ne 'Windows_NT') {
    Stop-Install 'this installer only supports native Windows PowerShell or PowerShell on Windows'
}

if ([string]::IsNullOrWhiteSpace($InstallDir)) {
    $localAppData = [Environment]::GetFolderPath([Environment+SpecialFolder]::LocalApplicationData)
    if ([string]::IsNullOrWhiteSpace($localAppData)) {
        Stop-Install 'LOCALAPPDATA is unavailable; set LY_INSTALL_DIR to choose an installation directory'
    }
    $InstallDir = Join-Path $localAppData 'ly\bin'
}

$tempDirectory = Join-Path ([IO.Path]::GetTempPath()) ("ly-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tempDirectory -Force | Out-Null

try {
    $version = $env:LY_VERSION
    if ([string]::IsNullOrWhiteSpace($version)) {
        $manifestFile = Join-Path $tempDirectory 'version.json'
        Write-InstallMessage "downloading version manifest $ManifestUrl"
        Download-File -Uri $ManifestUrl -Destination $manifestFile
        $manifest = Get-Content -LiteralPath $manifestFile -Raw | ConvertFrom-Json
        $version = [string]$manifest.version
    }
    $version = $version.Trim().TrimStart('v')
    if ([string]::IsNullOrWhiteSpace($version)) {
        Stop-Install 'version manifest is missing a version'
    }

    $architecture = Get-WindowsArchitecture
    $asset = "ly-$version-windows-$architecture.zip"
    $checksumUrl = "$ReleaseBase/v$version/checksums.txt"
    $assetUrl = "$ReleaseBase/v$version/$asset"
    $checksumFile = Join-Path $tempDirectory 'checksums.txt'
    $archiveFile = Join-Path $tempDirectory $asset

    Write-InstallMessage "platform: windows-$architecture  version: $version"
    Write-InstallMessage "downloading checksums $checksumUrl"
    Download-File -Uri $checksumUrl -Destination $checksumFile
    $expectedHash = Get-ExpectedChecksum -ChecksumFile $checksumFile -AssetName $asset

    Write-InstallMessage "downloading $assetUrl"
    Download-File -Uri $assetUrl -Destination $archiveFile
    $actualHash = (Get-FileHash -LiteralPath $archiveFile -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actualHash -ne $expectedHash) {
        Stop-Install "SHA-256 checksum mismatch for $asset"
    }
    Write-InstallMessage 'SHA-256 checksum verified'

    $extractDirectory = Join-Path $tempDirectory 'extract'
    Expand-Archive -LiteralPath $archiveFile -DestinationPath $extractDirectory -Force
    $binary = Get-ChildItem -LiteralPath $extractDirectory -Filter 'ly.exe' -File -Recurse | Select-Object -First 1
    if ($null -eq $binary) {
        Stop-Install "ly.exe was not found in $asset"
    }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $destination = Join-Path $InstallDir 'ly.exe'
    Copy-Item -LiteralPath $binary.FullName -Destination $destination -Force
    Add-InstallDirectoryToUserPath -Directory $InstallDir

    & $destination --version
    if ($LASTEXITCODE -ne 0) {
        Stop-Install 'installed ly.exe did not start successfully'
    }
    Write-InstallMessage "ly installed: $destination"
    Write-InstallMessage 'open a new terminal, then run ly --help'
}
finally {
    if (Test-Path -LiteralPath $tempDirectory) {
        Remove-Item -LiteralPath $tempDirectory -Recurse -Force
    }
}

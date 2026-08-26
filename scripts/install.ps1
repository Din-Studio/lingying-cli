# ly Windows 引导安装器
#
#   irm https://github.com/Din-Studio/lingying-cli/releases/latest/download/install.ps1 | iex
#
# 唯一职责：取得一个校验通过的 ly.exe 并写入 PATH。此后的更新请用 ly update。
# 环境变量：LY_VERSION 锁版本、LY_MIRROR 自定义镜像、LY_INSTALL_DIR 安装目录。

Set-StrictMode -Version Latest
$ErrorActionPreference = 'Stop'
$ProgressPreference = 'SilentlyContinue'

$Repo = 'Din-Studio/lingying-cli'
$Direct = 'https://github.com'
$InstallDir = if ($env:LY_INSTALL_DIR) { $env:LY_INSTALL_DIR }
              else { Join-Path $env:LOCALAPPDATA 'ly\bin' }

function Say { param([string]$m) Write-Host "ly-installer: $m" }
function Die { param([string]$m) throw "ly-installer: $m" }

# 候选源基址，直连优先。顺序必须与 internal/updater/source.go 的 Sources() 一致。
function Get-Sources {
    $list = @($Direct)
    if ($env:LY_MIRROR) { $list += ($env:LY_MIRROR.TrimEnd('/') + '/' + $Direct) }
    $list += "https://ghfast.top/$Direct"
    $list += "https://gh-proxy.com/$Direct"
    return $list
}

# 资产名不含版本号：latest/download/X 只是到 download/<最新tag>/X 的重定向，
# 因此无需先发现版本号即可下载。
function Get-AssetUrl {
    param([string]$Base, [string]$Asset)
    if ($env:LY_VERSION) {
        $v = $env:LY_VERSION.TrimStart('v')
        return "$Base/$Repo/releases/download/v$v/$Asset"
    }
    return "$Base/$Repo/releases/latest/download/$Asset"
}

function Get-Remote {
    param([string]$Uri, [string]$OutFile)
    try {
        $p = @{ Uri = $Uri; OutFile = $OutFile; MaximumRedirection = 5; TimeoutSec = 600 }
        if ((Get-Command Invoke-WebRequest).Parameters.ContainsKey('UseBasicParsing')) {
            $p.UseBasicParsing = $true
        }
        Invoke-WebRequest @p
        return $true
    } catch { return $false }
}

if ($env:OS -ne 'Windows_NT') { Die '此安装器仅支持 Windows' }

$rawArch = if ($env:PROCESSOR_ARCHITEW6432) { $env:PROCESSOR_ARCHITEW6432 } else { $env:PROCESSOR_ARCHITECTURE }
$arch = switch ($rawArch.ToUpperInvariant()) {
    'AMD64' { 'amd64' }
    'ARM64' { 'arm64' }
    default { Die "不支持的架构: $rawArch" }
}
$asset = "ly-windows-$arch.zip"

$tmp = Join-Path ([IO.Path]::GetTempPath()) ("ly-install-" + [Guid]::NewGuid().ToString('N'))
New-Item -ItemType Directory -Path $tmp -Force | Out-Null

try {
    # checksums.txt 只有几百字节，慢链路上也容易直连成功。先只向直连索取：
    # 校验和一旦来自可信源，归档包就可以安全地走镜像——镜像换不掉包。
    $sumFile = Join-Path $tmp 'checksums.txt'
    $trusted = Get-Remote -Uri (Get-AssetUrl $Direct 'checksums.txt') -OutFile $sumFile
    if (-not $trusted) {
        foreach ($base in Get-Sources) {
            if ($base -eq $Direct) { continue }
            if (Get-Remote -Uri (Get-AssetUrl $base 'checksums.txt') -OutFile $sumFile) { break }
        }
    }
    if (-not (Test-Path $sumFile)) { Die '无法获取校验和文件，已中止以免安装未经校验的二进制' }

    $expected = $null
    foreach ($line in Get-Content -LiteralPath $sumFile) {
        $parts = $line -split '\s+', 2
        if ($parts.Count -eq 2 -and $parts[1].Trim().TrimStart('*') -eq $asset) {
            $expected = $parts[0].ToLowerInvariant()
        }
    }
    if ($expected -notmatch '^[0-9a-f]{64}$') { Die "校验和文件中没有 $asset" }

    $version = if ($env:LY_VERSION) { $env:LY_VERSION } else { 'latest' }
    Say "平台 windows-$arch  版本 $version"

    $archive = Join-Path $tmp $asset
    $ok = $false
    foreach ($base in Get-Sources) {
        Say "下载 $asset （$base）"
        if (Get-Remote -Uri (Get-AssetUrl $base $asset) -OutFile $archive) { $ok = $true; break }
        Say '该来源不可用，尝试下一个'
    }
    if (-not $ok) { Die '所有下载来源均失败' }

    $actual = (Get-FileHash -LiteralPath $archive -Algorithm SHA256).Hash.ToLowerInvariant()
    if ($actual -ne $expected) { Die "SHA-256 校验和不匹配，已中止`n  期望: $expected`n  实际: $actual" }
    Say 'SHA-256 校验通过'
    if (-not $trusted) { Say '警告 —— 校验和取自镜像而非 GitHub 直连，只能防传输损坏，不能防篡改' }

    $extract = Join-Path $tmp 'extract'
    Expand-Archive -LiteralPath $archive -DestinationPath $extract -Force
    $bin = Get-ChildItem -LiteralPath $extract -Filter 'ly.exe' -File -Recurse | Select-Object -First 1
    if ($null -eq $bin) { Die '归档中没有 ly.exe' }

    New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
    $dest = Join-Path $InstallDir 'ly.exe'
    Copy-Item -LiteralPath $bin.FullName -Destination $dest -Force

    $normalized = [IO.Path]::GetFullPath($InstallDir).TrimEnd('\')
    $userPath = [Environment]::GetEnvironmentVariable('Path', 'User')
    $entries = @($userPath -split ';' | Where-Object { $_ })
    if (-not ($entries | Where-Object { $_.TrimEnd('\') -ieq $normalized })) {
        [Environment]::SetEnvironmentVariable('Path', (@($entries + $normalized) -join ';'), 'User')
        Say "已将 $normalized 写入用户 PATH"
    }
    $env:Path = "$normalized;$env:Path"

    & $dest --version
    Say "ly 已安装: $dest"
    Say '打开新终端后运行 ly --help；后续更新请运行 ly update'
}
finally {
    if (Test-Path -LiteralPath $tmp) { Remove-Item -LiteralPath $tmp -Recurse -Force }
}

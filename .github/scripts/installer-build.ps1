# vMeltype のインストーラー (vMeltype-<バージョン>-setup.exe) を Inno Setup で作り、そのパスを返す。
# SourceDir に置いたものを、すべてインストーラーに入れる (vmeltype.exe と LICENSE.txt は必ず置く)。
# Inno Setup は $InnoSetupVersion に固定する。GitHub Actions では、runner に入っているものが違えば Chocolatey で入れ直す。
# 上げるときは、ここを変えて、インストーラーの検査 (installer.yml) が通ることを確かめる。
param(
    [Parameter(Mandatory)] [string] $Version,
    [Parameter(Mandatory)] [string] $SourceDir,
    [Parameter(Mandatory)] [string] $OutputDir
)

$ErrorActionPreference = 'Stop'
$InnoSetupVersion = '6.7.1'

function Find-Iscc {
    $candidates = @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
    )
    $candidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
}

# Get-InnoSetupVersion は、入っている Inno Setup 6 のバージョンを、「設定 → アプリ」の登録から読む。
# ISCC.exe のファイルのバージョンは数字の欄が 0 のままなので使えない。
function Get-InnoSetupVersion {
    $keys = @(
        'HKLM:\SOFTWARE\WOW6432Node\Microsoft\Windows\CurrentVersion\Uninstall\Inno Setup 6_is1',
        'HKLM:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Inno Setup 6_is1',
        'HKCU:\SOFTWARE\Microsoft\Windows\CurrentVersion\Uninstall\Inno Setup 6_is1'
    )
    foreach ($key in $keys) {
        $version = (Get-ItemProperty -LiteralPath $key -ErrorAction SilentlyContinue).DisplayVersion
        if ($version) { return $version }
    }
    $null
}

$iscc = Find-Iscc
$found = if ($iscc) { Get-InnoSetupVersion } else { $null }
if ($found -ne $InnoSetupVersion) {
    if ($env:GITHUB_ACTIONS -ne 'true') {
        if (-not $iscc) {
            throw "Inno Setup $InnoSetupVersion (ISCC.exe) が見つかりません。https://jrsoftware.org/isinfo.php から入れてください"
        }
        Write-Warning "Inno Setup $found で作ります。リリースでは $InnoSetupVersion を使います"
    } else {
        choco install innosetup --version $InnoSetupVersion --allow-downgrade --force -y --no-progress | Out-Host
        $iscc = Find-Iscc
        $found = Get-InnoSetupVersion
        if (-not $iscc -or $found -ne $InnoSetupVersion) { throw "Inno Setup $InnoSetupVersion を入れられませんでした (見つかったもの: $found)" }
    }
}
Write-Host "Inno Setup $found ($iscc)"

$script = Join-Path $PSScriptRoot '..\..\installer\vMeltype.iss'
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
# ISCC は相対パスを .iss の場所から解くので、絶対パスにして渡す
$source = (Resolve-Path -LiteralPath $SourceDir).Path
$output = (Resolve-Path -LiteralPath $OutputDir).Path
$log = & $iscc "/DAppVersion=$Version" "/DSourceDir=$source" "/DOutputDir=$output" $script
$log | Out-Host
if ($LASTEXITCODE -ne 0) {
    throw "ISCC が終了コード $LASTEXITCODE で失敗しました"
}
# 登録と実体が食い違っていないかを、ISCC 自身が出すバージョンでも確かめる
$engine = $log | Select-String -Pattern 'Compiler engine version: Inno Setup (\S+)' | Select-Object -First 1
if ($env:GITHUB_ACTIONS -eq 'true' -and (-not $engine -or $engine.Matches[0].Groups[1].Value -ne $InnoSetupVersion)) {
    throw "Inno Setup $InnoSetupVersion で作れていません: $engine"
}
$setup = Join-Path $output "vMeltype-$Version-setup.exe"
if (-not (Test-Path -LiteralPath $setup)) {
    throw "$setup ができていません"
}
$setup

# vMeltype のインストーラー (vMeltype-<バージョン>-setup.exe) を Inno Setup で作り、そのパスを返す。
# SourceDir には vmeltype.exe と LICENSE.txt を置いておく。Inno Setup 6 がなければ Chocolatey で入れるので、GitHub Actions の Windows で動かす。
param(
    [Parameter(Mandatory)] [string] $Version,
    [Parameter(Mandatory)] [string] $SourceDir,
    [Parameter(Mandatory)] [string] $OutputDir
)

$ErrorActionPreference = 'Stop'

function Find-Iscc {
    $candidates = @(
        "${env:ProgramFiles(x86)}\Inno Setup 6\ISCC.exe",
        "$env:ProgramFiles\Inno Setup 6\ISCC.exe",
        "$env:LOCALAPPDATA\Programs\Inno Setup 6\ISCC.exe"
    )
    $candidates | Where-Object { Test-Path -LiteralPath $_ } | Select-Object -First 1
}

$iscc = Find-Iscc
if (-not $iscc) {
    if ($env:GITHUB_ACTIONS -ne 'true') {
        throw 'Inno Setup 6 (ISCC.exe) が見つかりません。https://jrsoftware.org/isinfo.php から入れてください'
    }
    choco install innosetup -y --no-progress | Out-Host
    $iscc = Find-Iscc
    if (-not $iscc) { throw 'Inno Setup 6 を入れられませんでした' }
}

$script = Join-Path $PSScriptRoot '..\..\installer\vMeltype.iss'
New-Item -ItemType Directory -Force -Path $OutputDir | Out-Null
# ISCC は相対パスを .iss の場所から解くので、絶対パスにして渡す
$source = (Resolve-Path -LiteralPath $SourceDir).Path
$output = (Resolve-Path -LiteralPath $OutputDir).Path
& $iscc "/DAppVersion=$Version" "/DSourceDir=$source" "/DOutputDir=$output" $script | Out-Host
if ($LASTEXITCODE -ne 0) {
    throw "ISCC が終了コード $LASTEXITCODE で失敗しました"
}
$setup = Join-Path $output "vMeltype-$Version-setup.exe"
if (-not (Test-Path -LiteralPath $setup)) {
    throw "$setup ができていません"
}
$setup

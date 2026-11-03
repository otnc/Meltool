# vMeltype のインストーラーで入れて、入れ直し、vmeltype app uninstall とアンインストーラーで消せることを、画面なしで通しで確かめる。
# 利用者の %LOCALAPPDATA%\Programs\vMeltype、PATH、「設定 → アプリ」の登録、管理ルートを書き換えるので、GitHub Actions の使い捨ての Windows でだけ動かす。
param(
    [Parameter(Mandatory)] [string] $Setup,
    [Parameter(Mandatory)] [string] $Version
)

$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true') {
    throw 'このスクリプトは、利用者の PATH や「設定 → アプリ」の登録を書き換えるので、GitHub Actions の中でだけ動かします'
}

$appDir = Join-Path $env:LOCALAPPDATA 'Programs\vMeltype'
$exe = Join-Path $appDir 'vmeltype.exe'
$root = Join-Path $env:LOCALAPPDATA 'vMeltype'
$uninstallKey = 'HKCU:\Software\Microsoft\Windows\CurrentVersion\Uninstall\vMeltype_is1'

function Invoke-Silent([string] $Program, [string[]] $Arguments) {
    Write-Host "> $Program $Arguments"
    $process = Start-Process -FilePath $Program -ArgumentList $Arguments -Wait -PassThru
    if ($process.ExitCode -ne 0) {
        throw "$Program が終了コード $($process.ExitCode) で失敗しました"
    }
}

function Install-Vmeltype {
    Invoke-Silent $Setup @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART', '/SP-', "/LOG=$env:RUNNER_TEMP\vmeltype-setup.log")
}

function Get-UserPathEntries {
    # レジストリの値をそのまま読む (展開しない)
    $key = Get-Item -LiteralPath 'HKCU:\Environment'
    $paths = $key.GetValue('Path', '', 'DoNotExpandEnvironmentNames')
    @($paths -split ';' | Where-Object { $_ -ne '' })
}

function Assert-Installed {
    if (-not (Test-Path -LiteralPath $exe)) { throw "$exe がありません" }
    $output = & $exe --version
    if ($LASTEXITCODE -ne 0 -or -not ($output -match [regex]::Escape($Version))) {
        throw "vmeltype --version の結果が違います: $output"
    }
    if (-not (Test-Path -LiteralPath $uninstallKey)) { throw '「設定 → アプリ」に登録されていません' }
    $count = @(Get-UserPathEntries | Where-Object { $_.TrimEnd('\') -eq $appDir }).Count
    if ($count -ne 1) { throw "PATH の $appDir の数が 1 ではありません: $count" }
}

function Wait-Removed {
    $deadline = (Get-Date).AddMinutes(2)
    while ((Test-Path -LiteralPath $exe) -or (Test-Path -LiteralPath $uninstallKey)) {
        if ((Get-Date) -gt $deadline) { throw "アンインストールが終わりません。$exe か「設定 → アプリ」の登録が残っています" }
        Start-Sleep -Milliseconds 500
    }
    if (@(Get-UserPathEntries | Where-Object { $_.TrimEnd('\') -eq $appDir }).Count -ne 0) {
        throw "PATH に $appDir が残っています"
    }
}

Write-Host '--- インストールし、入れ直しても PATH が重ならない'
Install-Vmeltype
Assert-Installed
Install-Vmeltype
Assert-Installed

Write-Host '--- vmeltype app uninstall で、管理ルートとインストールを消す'
& $exe app config set auto-snapshot-keep 5
if ($LASTEXITCODE -ne 0) { throw '管理ルートを作れませんでした' }
& $exe app uninstall -y
if ($LASTEXITCODE -ne 0) { throw "vmeltype app uninstall が終了コード $LASTEXITCODE で失敗しました" }
Wait-Removed
if (Test-Path -LiteralPath $root) { throw "管理ルート $root が残っています" }

Write-Host '--- アンインストーラーは、ファイルを消す前に管理ルートを片付ける'
Install-Vmeltype
Assert-Installed
& $exe app config set auto-snapshot-keep 5
if ($LASTEXITCODE -ne 0) { throw '管理ルートを作れませんでした' }
$uninstaller = (Get-ItemProperty -LiteralPath $uninstallKey).UninstallString.Trim('"')
Invoke-Silent $uninstaller @('/VERYSILENT', '/SUPPRESSMSGBOXES', '/NORESTART')
Wait-Removed
# 画面なしのアンインストールはスナップショットを残すので、管理ルートには snapshots だけが残ってよい
$left = @(Get-ChildItem -LiteralPath $root -ErrorAction SilentlyContinue | Where-Object { $_.Name -ne 'snapshots' })
if ($left.Count -ne 0) { throw "管理ルートを片付けていません: $($left.Name -join ', ')" }

Write-Host 'インストーラーの通しの確認がすべて通りました'

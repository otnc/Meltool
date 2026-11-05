# track enable で、本物のタスク スケジューラーに追従の処理を登録し、動かし、track disable で外せることを確かめる。
# 利用者のタスク スケジューラーを書き換えるので、GitHub Actions の使い捨ての Windows でだけ動かす。
param(
    [Parameter(Mandatory)] [string] $Vmeltype
)

$ErrorActionPreference = 'Stop'
if ($env:GITHUB_ACTIONS -ne 'true') {
    throw 'このスクリプトは、利用者のタスク スケジューラーを書き換えるので、GitHub Actions の中でだけ動かします'
}

$root = Join-Path $env:RUNNER_TEMP 'vmeltype-track-root'
$task = 'vMeltype\track'

function Invoke-Vmeltype {
    Write-Host "> vmeltype $args"
    & $Vmeltype --root $root @args
    if ($LASTEXITCODE -ne 0) { throw "vmeltype $args が終了コード $LASTEXITCODE で失敗しました" }
}

Write-Host '--- track enable で、タスク スケジューラーに登録する'
Invoke-Vmeltype track enable --interval 24h
$xml = schtasks.exe /Query /TN $task /XML | Out-String
if ($LASTEXITCODE -ne 0) { throw 'タスクが登録されていません' }
foreach ($want in @('<Interval>PT24H</Interval>', '<Command>conhost.exe</Command>', '--headless', 'track run --root')) {
    if (-not $xml.Contains($want)) { throw "タスクの定義に $want がありません:`n$xml" }
}

Write-Host '--- 登録したタスクを動かすと、追従の処理が結果を記録する'
schtasks.exe /Run /TN $task | Out-Host
if ($LASTEXITCODE -ne 0) { throw 'タスクを動かせませんでした' }
$deadline = (Get-Date).AddMinutes(2)
do {
    Start-Sleep -Seconds 2
    $status = & $Vmeltype --root $root track status --json | ConvertFrom-Json
    if ((Get-Date) -gt $deadline) { throw '追従の処理が結果を記録しません' }
} while (-not $status.last)
Write-Host "結果: $($status.last.outcome) $($status.last.message)"

Write-Host '--- track disable で、登録を外す'
Invoke-Vmeltype track disable
schtasks.exe /Query /TN $task 2>$null | Out-Null
if ($LASTEXITCODE -eq 0) { throw 'タスクが残っています' }

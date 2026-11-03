//go:build windows

package winime

import (
	"context"
	"encoding/base64"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"unicode/utf16"

	"golang.org/x/sys/windows"
)

// prelude は、本家の meltype-ime.ps1 を読み込み、使う関数があるかと、DLL が壊れていないかを確かめる。
// パスは文字列に埋め込まず、環境変数で渡す。本家の Install-MeltypeIme は既定の入力方式を変えることがあるので使わない。
const prelude = `
$ErrorActionPreference = 'Stop'
function Done([string]$result) {
    [IO.File]::WriteAllText($env:VMELTYPE_IME_RESULT, $result)
    exit 0
}
$imeScript = $env:VMELTYPE_IME_SCRIPT
if (-not (Test-Path -LiteralPath $imeScript)) { Done 'unsupported' }
. $imeScript
foreach ($name in 'Test-Arm64Windows', 'Get-MeltypeImeSourceHashes', 'Test-MeltypeImeCurrent', 'Invoke-Elevated') {
    if (-not (Get-Command $name -CommandType Function -ErrorAction SilentlyContinue)) { Done 'unsupported' }
}
$source = $env:VMELTYPE_IME_SOURCE
if (Test-Arm64Windows) { Done 'arm64' }
$hashes = Get-MeltypeImeSourceHashes $source
if (-not $hashes) { Done 'corrupt' }
`

// replaceScript は、登録済みの DLL が違うときだけ入れ替える。入力方式の一覧は変えない。
const replaceScript = prelude + `
if (Test-MeltypeImeCurrent $source) { Done 'current' }
switch (Invoke-Elevated $source $hashes 'Register') {
    'ok' { Done 'replaced' }
    'cancelled' { Done 'cancelled' }
    default { Done 'failed' }
}
`

// registerScript は、DLL を登録し、日本語のキーボードの一覧の末尾に Meltype を足す。既定の入力方式とほかの入力方式の順は変えない。
// 本家の Install-MeltypeIme と同じく、登録できたら、前に登録を断った記録を消す。
// 一覧に足す処理は利用者の設定を変えるので、テストで差し替えられるよう、読み込んだスクリプトに同じ名前の関数があればそれを使う (本家にはない名前)。
const registerScript = prelude + `
if (-not (Get-Command Complete-VmeltypeImeRegistration -CommandType Function -ErrorAction SilentlyContinue)) {
    function Complete-VmeltypeImeRegistration {
        Remove-ItemProperty -LiteralPath 'HKCU:\Software\Meltype' -Name ImeDeclined -ErrorAction SilentlyContinue
        $list = Get-WinUserLanguageList
        $japanese = $list | Where-Object { $_.LanguageTag -like 'ja*' } | Select-Object -First 1
        if (-not $japanese) {
            $list.Add('ja-JP')
            $japanese = $list | Where-Object { $_.LanguageTag -like 'ja*' } | Select-Object -First 1
        }
        if ($japanese.InputMethodTips -notcontains $MeltypeImeTip) {
            $japanese.InputMethodTips.Add($MeltypeImeTip)
            Set-WinUserLanguageList $list -Force
        }
    }
}
if (-not (Test-MeltypeImeCurrent $source)) {
    switch (Invoke-Elevated $source $hashes 'Register') {
        'ok' { }
        'cancelled' { Done 'cancelled' }
        default { Done 'failed' }
    }
}
try { Complete-VmeltypeImeRegistration } catch { Done 'unlisted' }
Done 'registered'
`

// Replace は、appDir (切り替え先のバージョンの、本家の ZIP の app に当たるディレクトリ) の meltype-ime.ps1 と tip を使って、
// 登録済みの IME の DLL を入れ替える。DLL が違うときだけ、本家が管理者権限の確認 (UAC) を出す。
// 呼び出し側は、Meltype IME が登録済みであることと、Meltype が止まっていることを確かめておく。
func Replace(ctx context.Context, appDir string) (Result, error) {
	result, err := run(ctx, replaceScript, appDir)
	if err != nil {
		return "", fmt.Errorf("Meltype IME の DLL を入れ替えられませんでした: %w", err)
	}
	return result, nil
}

// Register は、appDir (有効なバージョンの、本家の ZIP の app に当たるディレクトリ) の meltype-ime.ps1 と tip を使って、
// Meltype IME を登録し、日本語のキーボードの一覧に足す。登録では本家が管理者権限の確認 (UAC) を出す。既定の入力方式は変えない。
// 呼び出し側は、利用者が登録を明示したことと、通常版であることを確かめておく。
func Register(ctx context.Context, appDir string) (Result, error) {
	result, err := run(ctx, registerScript, appDir)
	if err != nil {
		return "", fmt.Errorf("Meltype IME を登録できませんでした: %w", err)
	}
	return result, nil
}

// run は、本家の meltype-ime.ps1 を読み込んだ PowerShell で script を動かし、結果を返す。
func run(ctx context.Context, script, appDir string) (Result, error) {
	powershell, err := windowsPowerShell()
	if err != nil {
		return "", err
	}
	resultFile, err := os.CreateTemp("", "vmeltype-ime-*.txt")
	if err != nil {
		return "", err
	}
	resultFile.Close()
	defer os.Remove(resultFile.Name())

	cmd := exec.CommandContext(ctx, powershell, "-NoProfile", "-NonInteractive", "-ExecutionPolicy", "Bypass", "-EncodedCommand", encode(script))
	cmd.Env = append(os.Environ(),
		"VMELTYPE_IME_SCRIPT="+filepath.Join(appDir, "meltype-ime.ps1"),
		"VMELTYPE_IME_SOURCE="+filepath.Join(appDir, "tip"),
		"VMELTYPE_IME_RESULT="+resultFile.Name(),
	)
	output, runErr := cmd.CombinedOutput()
	data, _ := os.ReadFile(resultFile.Name())
	result := Result(strings.TrimSpace(string(data)))
	if result == "" {
		return "", fmt.Errorf("%w\n%s", errors.Join(runErr, errors.New("結果が返りませんでした")), strings.TrimSpace(string(output)))
	}
	return result, nil
}

// windowsPowerShell は、PATH から探さず、Windows のフォルダーの powershell.exe を返す。本家のインストーラーと同じ。
func windowsPowerShell() (string, error) {
	dir, err := windows.GetSystemWindowsDirectory()
	if err != nil {
		return "", fmt.Errorf("Windows のフォルダーを読めませんでした: %w", err)
	}
	return filepath.Join(dir, "System32", "WindowsPowerShell", "v1.0", "powershell.exe"), nil
}

// encode は、PowerShell の -EncodedCommand の形 (UTF-16LE の Base64) にする。
func encode(s string) string {
	units := utf16.Encode([]rune(s))
	b := make([]byte, len(units)*2)
	for i, u := range units {
		b[2*i] = byte(u)
		b[2*i+1] = byte(u >> 8)
	}
	return base64.StdEncoding.EncodeToString(b)
}

//go:build windows

package winime

import (
	"context"
	"os"
	"path/filepath"
	"testing"
)

// fakeAppDir は、本家の meltype-ime.ps1 と同じ名前の関数を持つ偽のスクリプトを置く。
// 本物は管理者権限で DLL を登録するので、テストでは呼ばれた引数を記録して、決めた結果を返すだけにする。
// パスの扱いを確かめるため、ディレクトリ名に空白と引用符を入れる。
func fakeAppDir(t *testing.T, functions string) (appDir, record string) {
	t.Helper()
	appDir = filepath.Join(t.TempDir(), "it's a dir", "app")
	if err := os.MkdirAll(filepath.Join(appDir, "tip"), 0o700); err != nil {
		t.Fatal(err)
	}
	record = filepath.Join(t.TempDir(), "record.txt")
	// 本家のスクリプトと同じく BOM を付ける。付けないと、Windows PowerShell 5.1 が日本語のパスを読み違える
	script := "\xEF\xBB\xBF$record = '" + record + "'\n" + functions
	if err := os.WriteFile(filepath.Join(appDir, "meltype-ime.ps1"), []byte(script), 0o600); err != nil {
		t.Fatal(err)
	}
	return appDir, record
}

const commonFunctions = `
function Test-Arm64Windows { return $false }
function Get-MeltypeImeSourceHashes([string]$Source) { return @{ 'Register-Tip.ps1' = 'ABC' } }
`

func TestReplace(t *testing.T) {
	tests := []struct {
		name      string
		functions string
		want      Result
	}{
		{
			name: "同じ DLL なら、管理者権限を求めずに終わる",
			functions: commonFunctions + `function Test-MeltypeImeCurrent([string]$Source) { return $true }
function Invoke-Elevated { [IO.File]::WriteAllText($record, 'elevated'); return 'ok' }`,
			want: ResultCurrent,
		},
		{
			name: "違う DLL なら、本家の Invoke-Elevated で入れ替える",
			functions: commonFunctions + `function Test-MeltypeImeCurrent([string]$Source) { return $false }
function Invoke-Elevated([string]$Source, [hashtable]$Hashes, [string]$Mode) { [IO.File]::WriteAllText($record, "$Source|$Mode|$($Hashes['Register-Tip.ps1'])"); Write-Host '  登録しました'; return 'ok' }`,
			want: ResultReplaced,
		},
		{
			name: "UAC で断られたことを返す",
			functions: commonFunctions + `function Test-MeltypeImeCurrent([string]$Source) { return $false }
function Invoke-Elevated { return 'cancelled' }`,
			want: ResultCanceled,
		},
		{
			name: "登録に失敗したことを返す",
			functions: commonFunctions + `function Test-MeltypeImeCurrent([string]$Source) { return $false }
function Invoke-Elevated { return 'failed' }`,
			want: ResultFailed,
		},
		{
			name: "DLL が壊れていれば、管理者権限を求めない",
			functions: `function Test-Arm64Windows { return $false }
function Get-MeltypeImeSourceHashes([string]$Source) { return $null }
function Test-MeltypeImeCurrent([string]$Source) { return $false }
function Invoke-Elevated { [IO.File]::WriteAllText($record, 'elevated'); return 'ok' }`,
			want: ResultCorrupt,
		},
		{
			name:      "本家の関数がなければ、対応していないことを返す",
			functions: `function Test-Arm64Windows { return $false }`,
			want:      ResultUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appDir, record := fakeAppDir(t, tt.functions)

			got, err := Replace(context.Background(), appDir)

			if err != nil || got != tt.want {
				t.Fatalf("Replace = %q, %v; want %q", got, err, tt.want)
			}
			data, _ := os.ReadFile(record)
			switch tt.want {
			case ResultReplaced:
				if want := filepath.Join(appDir, "tip") + "|Register|ABC"; string(data) != want {
					t.Errorf("Invoke-Elevated の引数 = %q, want %q", data, want)
				}
			case ResultCurrent, ResultCorrupt:
				if len(data) != 0 {
					t.Errorf("管理者権限を求めた: %q", data)
				}
			}
		})
	}
}

func TestReplaceWithoutScript(t *testing.T) {
	got, err := Replace(context.Background(), t.TempDir())

	if err != nil || got != ResultUnsupported {
		t.Errorf("Replace = %q, %v", got, err)
	}
}

// completeFunction は、登録のあとに一覧へ足す処理の代わり。本物は利用者のキーボードの一覧を変えるので、テストでは必ず差し替える。
const completeFunction = `
function Complete-VmeltypeImeRegistration { [IO.File]::AppendAllText($record, '|completed') }
`

func TestRegister(t *testing.T) {
	tests := []struct {
		name       string
		functions  string
		want       Result
		wantRecord string
	}{
		{
			name: "本家の Invoke-Elevated で登録し、キーボードの一覧に足す",
			functions: commonFunctions + completeFunction + `function Test-MeltypeImeCurrent([string]$Source) { return $false }
function Invoke-Elevated([string]$Source, [hashtable]$Hashes, [string]$Mode) { [IO.File]::AppendAllText($record, "$Mode"); return 'ok' }`,
			want:       ResultRegistered,
			wantRecord: "Register|completed",
		},
		{
			name: "同じ DLL を置いてあれば、管理者権限を求めずに一覧に足す",
			functions: commonFunctions + completeFunction + `function Test-MeltypeImeCurrent([string]$Source) { return $true }
function Invoke-Elevated { [IO.File]::AppendAllText($record, 'elevated'); return 'ok' }`,
			want:       ResultRegistered,
			wantRecord: "|completed",
		},
		{
			name: "UAC で断られたら、一覧に足さない",
			functions: commonFunctions + completeFunction + `function Test-MeltypeImeCurrent([string]$Source) { return $false }
function Invoke-Elevated { return 'cancelled' }`,
			want: ResultCanceled,
		},
		{
			name: "一覧に足せなければ、登録はしたことを返す",
			functions: commonFunctions + `function Complete-VmeltypeImeRegistration { throw '足せません' }
function Test-MeltypeImeCurrent([string]$Source) { return $false }
function Invoke-Elevated { return 'ok' }`,
			want: ResultUnlisted,
		},
		{
			name: "DLL が壊れていれば、管理者権限を求めない",
			functions: completeFunction + `function Test-Arm64Windows { return $false }
function Get-MeltypeImeSourceHashes([string]$Source) { return $null }
function Test-MeltypeImeCurrent([string]$Source) { return $false }
function Invoke-Elevated { [IO.File]::AppendAllText($record, 'elevated'); return 'ok' }`,
			want: ResultCorrupt,
		},
		{
			name:      "本家の関数がなければ、対応していないことを返す",
			functions: completeFunction + `function Test-Arm64Windows { return $false }`,
			want:      ResultUnsupported,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			appDir, record := fakeAppDir(t, tt.functions)

			got, err := Register(context.Background(), appDir)

			if err != nil || got != tt.want {
				t.Fatalf("Register = %q, %v; want %q", got, err, tt.want)
			}
			data, _ := os.ReadFile(record)
			if string(data) != tt.wantRecord {
				t.Errorf("記録 = %q, want %q", data, tt.wantRecord)
			}
		})
	}
}

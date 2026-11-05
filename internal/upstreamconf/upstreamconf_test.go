package upstreamconf

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func writeConfig(t *testing.T, body string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "config.json")
	if err := os.WriteFile(path, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func read(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

func TestDisableAutoUpdate(t *testing.T) {
	tests := []struct {
		name     string
		body     string
		want     string
		original Original
	}{
		{
			name:     "true を false にし、ほかの項目と並びと空白を保つ",
			body:     "{\n  \"Enabled\": true,\n  \"AutoUpdate\": true,\n  \"Future\": {\"AutoUpdate\": true}\n}",
			want:     "{\n  \"Enabled\": true,\n  \"AutoUpdate\": false,\n  \"Future\": {\"AutoUpdate\": true}\n}",
			original: Original{FileExisted: true, AutoUpdate: new(true)},
		},
		{
			name:     "既に false なら変えない",
			body:     "{\"AutoUpdate\":false}",
			want:     "{\"AutoUpdate\":false}",
			original: Original{FileExisted: true, AutoUpdate: new(false)},
		},
		{
			name:     "BOM を保つ",
			body:     "\xEF\xBB\xBF{\"AutoUpdate\": true}",
			want:     "\xEF\xBB\xBF{\"AutoUpdate\": false}",
			original: Original{FileExisted: true, AutoUpdate: new(true)},
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, tt.body)

			original, err := DisableAutoUpdate(path)

			if err != nil {
				t.Fatal(err)
			}
			if got := read(t, path); got != tt.want {
				t.Errorf("中身 = %q, want %q", got, tt.want)
			}
			if original.FileExisted != tt.original.FileExisted || !sameBool(original.AutoUpdate, tt.original.AutoUpdate) {
				t.Errorf("元の値 = %+v, want %+v", original, tt.original)
			}
		})
	}

	// 足す位置と空白は、書き換えに使うライブラリに任せるので、中身の値で確かめる
	for _, body := range []string{"{\n  \"Enabled\": true\n}", "{}"} {
		t.Run("項目がなければ足し、ほかの項目は変えない: "+body, func(t *testing.T) {
			path := writeConfig(t, body)

			original, err := DisableAutoUpdate(path)

			var got map[string]any
			if err := json.Unmarshal([]byte(read(t, path)), &got); err != nil {
				t.Fatal(err)
			}
			var before map[string]any
			json.Unmarshal([]byte(body), &before)
			if err != nil || got["AutoUpdate"] != false || len(got) != len(before)+1 || (before["Enabled"] != nil && got["Enabled"] != true) {
				t.Errorf("中身 = %q, err = %v", read(t, path), err)
			}
			if !original.FileExisted || original.AutoUpdate != nil {
				t.Errorf("元の値 = %+v", original)
			}
		})
	}

	t.Run("ファイルがなければ、自動更新を止めた設定を作る", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")

		original, err := DisableAutoUpdate(path)

		if err != nil || original.FileExisted || read(t, path) != "{\"AutoUpdate\": false}" {
			t.Errorf("original = %+v, err = %v", original, err)
		}
	})
}

func TestDisableAutoUpdateLeavesUnreadableConfig(t *testing.T) {
	for name, body := range map[string]string{
		"壊れた JSON":       "{\"AutoUpdate\": tru",
		"真偽値でない値":        "{\"AutoUpdate\": \"yes\"}",
		"オブジェクトでない JSON": "[true]",
		"後ろに余計なもの":       "{\"AutoUpdate\": true} {}",
	} {
		t.Run(name+"は変えずにエラーにする", func(t *testing.T) {
			path := writeConfig(t, body)

			_, err := DisableAutoUpdate(path)

			if err == nil || read(t, path) != body {
				t.Errorf("err = %v, 中身 = %q", err, read(t, path))
			}
		})
	}
}

//go:fix inline
func ptr(b bool) *bool { return new(b) }

func sameBool(a, b *bool) bool {
	return (a == nil && b == nil) || (a != nil && b != nil && *a == *b)
}

func TestRestoreAutoUpdate(t *testing.T) {
	tests := []struct {
		name     string
		original Original
		want     string
	}{
		{name: "管理前が true なら true に戻し、ほかの項目は変えない", original: Original{FileExisted: true, AutoUpdate: new(true)}, want: "{\n  \"Enabled\": true,\n  \"AutoUpdate\": true\n}"},
		{name: "管理前に項目がなければ、本家の既定の true に戻す", original: Original{FileExisted: true}, want: "{\n  \"Enabled\": true,\n  \"AutoUpdate\": true\n}"},
		{name: "管理前が false なら false のまま", original: Original{FileExisted: true, AutoUpdate: new(false)}, want: "{\n  \"Enabled\": true,\n  \"AutoUpdate\": false\n}"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeConfig(t, "{\n  \"Enabled\": true,\n  \"AutoUpdate\": false\n}")

			if err := RestoreAutoUpdate(path, tt.original); err != nil {
				t.Fatal(err)
			}
			if got := read(t, path); got != tt.want {
				t.Errorf("中身 = %q, want %q", got, tt.want)
			}
		})
	}
	t.Run("ファイルがなければ作らない", func(t *testing.T) {
		path := filepath.Join(t.TempDir(), "config.json")

		if err := RestoreAutoUpdate(path, Original{AutoUpdate: new(true)}); err != nil {
			t.Fatal(err)
		}
		if _, err := os.Stat(path); !os.IsNotExist(err) {
			t.Errorf("設定を作った")
		}
	})
}

func TestAutoUpdateDisabled(t *testing.T) {
	tests := []struct {
		name string
		body string
		want bool
	}{
		{name: "false なら止まっている", body: "\xEF\xBB\xBF{\"AutoUpdate\": false}", want: true},
		{name: "true なら動いている", body: `{"AutoUpdate": true}`, want: false},
		{name: "項目がなければ、本家の既定の true で動いている", body: `{"Enabled": true}`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := AutoUpdateDisabled(writeConfig(t, tt.body))

			if err != nil || got != tt.want {
				t.Errorf("AutoUpdateDisabled = %v, %v; want %v", got, err, tt.want)
			}
		})
	}
	t.Run("ファイルがなければ、本家が既定の設定を作るので動いているとみなす", func(t *testing.T) {
		got, err := AutoUpdateDisabled(filepath.Join(t.TempDir(), "config.json"))

		if err != nil || got {
			t.Errorf("AutoUpdateDisabled = %v, %v", got, err)
		}
	})
	t.Run("読めない設定はエラーにする", func(t *testing.T) {
		if _, err := AutoUpdateDisabled(writeConfig(t, "{壊れている")); err == nil {
			t.Error("エラーにしていない")
		}
	})
}

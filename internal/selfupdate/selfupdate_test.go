package selfupdate

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/otnc/vmeltype/internal/catalog"
)

func release(tag string, pre bool, assets ...string) catalog.Release {
	r := catalog.Release{Tag: tag, Prerelease: pre}
	for _, name := range assets {
		r.Assets = append(r.Assets, catalog.Asset{Name: name, Digest: "sha256:aa"})
	}
	return r
}

func TestSelect(t *testing.T) {
	releases := []catalog.Release{
		release("v0.1.0-pre.1", true, "vMeltype-0.1.0-pre.1-windows.zip"),
		release("v0.1.0", false, "vMeltype-0.1.0-windows.zip"),
		release("v0.2.0-pre.1", true, "vMeltype-0.2.0-pre.1-windows.zip"),
		release("v0.3.0", false, "vMeltype-0.3.0-linux.zip"),
		release("v0.4.0-pre.1", true, "vMeltype-0.4.0-pre.1-windows.zip", "vMeltype-0.4.0-pre.1-setup.exe"),
	}
	tests := []struct {
		name       string
		current    string
		includePre bool
		kind       Kind
		wantAsset  string
	}{
		{name: "正式版の中で新しいものを選ぶ", current: "0.1.0-pre.1", wantAsset: "vMeltype-0.1.0-windows.zip"},
		{name: "--pre ならプレリリースも含める", current: "0.1.0", includePre: true, wantAsset: "vMeltype-0.4.0-pre.1-windows.zip"},
		{name: "この端末の配布物がないリリースは選ばない", current: "0.2.0-pre.1", wantAsset: ""},
		{name: "インストーラーで入れたものは setup.exe を選ぶ", current: "0.1.0", includePre: true, kind: Setup, wantAsset: "vMeltype-0.4.0-pre.1-setup.exe"},
		{name: "setup.exe がないリリースは、インストーラーで入れたものには選ばない", current: "0.1.0", kind: Setup, wantAsset: ""},
		{name: "最新なら選ばない", current: "0.1.0", wantAsset: ""},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			_, asset, ok := Select(releases, tt.current, tt.includePre, "windows", "amd64", tt.kind)

			if ok != (tt.wantAsset != "") || asset.Name != tt.wantAsset {
				t.Errorf("Select = %q (%v), want %q", asset.Name, ok, tt.wantAsset)
			}
		})
	}
}

func makeZip(t *testing.T, files map[string]string) string {
	t.Helper()
	var buf bytes.Buffer
	w := zip.NewWriter(&buf)
	for name, body := range files {
		fw, _ := w.Create(name)
		fw.Write([]byte(body))
	}
	w.Close()
	path := filepath.Join(t.TempDir(), "update.zip")
	if err := os.WriteFile(path, buf.Bytes(), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func installed(t *testing.T, body string) string {
	t.Helper()
	exe := filepath.Join(t.TempDir(), "vmeltype.exe")
	if err := os.WriteFile(exe, []byte(body), 0o700); err != nil {
		t.Fatal(err)
	}
	return exe
}

func read(path string) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return "<なし>"
	}
	return string(data)
}

func TestApply(t *testing.T) {
	t.Run("ZIP の vmeltype.exe に置き換え、前のものを退避する", func(t *testing.T) {
		exe := installed(t, "old")
		archive := makeZip(t, map[string]string{"vmeltype.exe": "new", "LICENSE.txt": "license"})

		if err := Apply(exe, archive); err != nil {
			t.Fatal(err)
		}
		if read(exe) != "new" || read(exe+".old") != "old" {
			t.Errorf("exe = %q, old = %q", read(exe), read(exe+".old"))
		}
		entries, _ := os.ReadDir(filepath.Dir(exe))
		if len(entries) != 2 {
			t.Errorf("作業用のファイルが残っている: %v", entries)
		}
	})
	t.Run("前の更新で退避したものを消してから置き換える", func(t *testing.T) {
		exe := installed(t, "second")
		os.WriteFile(exe+".old", []byte("first"), 0o600)
		archive := makeZip(t, map[string]string{"vmeltype.exe": "third"})

		if err := Apply(exe, archive); err != nil {
			t.Fatal(err)
		}
		if read(exe) != "third" || read(exe+".old") != "second" {
			t.Errorf("exe = %q, old = %q", read(exe), read(exe+".old"))
		}
	})
	t.Run("ZIP に vmeltype.exe がなければ、何も変えない", func(t *testing.T) {
		exe := installed(t, "old")
		archive := makeZip(t, map[string]string{"other.exe": "x"})

		if err := Apply(exe, archive); err == nil || read(exe) != "old" {
			t.Errorf("err = %v, exe = %q", err, read(exe))
		}
		if _, err := os.Stat(exe + ".old"); err == nil {
			t.Errorf("失敗したのに退避した")
		}
	})
}

func TestApplySetup(t *testing.T) {
	t.Run("退避してから setup.exe で入れ直す", func(t *testing.T) {
		exe := installed(t, "old")

		err := ApplySetup(exe, func() error {
			if read(exe) != "<なし>" {
				t.Errorf("動いている exe を退避する前に setup.exe を動かした")
			}
			return os.WriteFile(exe, []byte("new"), 0o700)
		})

		if err != nil {
			t.Fatal(err)
		}
		if read(exe) != "new" || read(exe+".old") != "old" {
			t.Errorf("exe = %q, old = %q", read(exe), read(exe+".old"))
		}
	})
	t.Run("setup.exe が失敗したら、退避したものを戻す", func(t *testing.T) {
		exe := installed(t, "old")

		err := ApplySetup(exe, func() error { return errors.New("失敗") })

		if err == nil || read(exe) != "old" {
			t.Errorf("err = %v, exe = %q", err, read(exe))
		}
	})
	t.Run("setup.exe が成功しても vmeltype.exe を置かなければ、退避したものを戻す", func(t *testing.T) {
		exe := installed(t, "old")

		err := ApplySetup(exe, func() error { return nil })

		if err == nil || read(exe) != "old" {
			t.Errorf("err = %v, exe = %q", err, read(exe))
		}
	})
}

func TestCleanup(t *testing.T) {
	exe := installed(t, "current")
	os.WriteFile(exe+".old", []byte("previous"), 0o600)

	Cleanup(exe)

	if read(exe) != "current" || read(exe+".old") != "<なし>" {
		t.Errorf("exe = %q, old = %q", read(exe), read(exe+".old"))
	}
}

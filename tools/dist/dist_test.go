package main

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"testing"
)

func TestPlatformName(t *testing.T) {
	for _, tt := range []struct{ goos, goarch, want string }{
		{"windows", "amd64", "windows"},
		{"darwin", "arm64", "mac"},
		{"linux", "amd64", "linux"},
		{"linux", "arm64", "linux-arm64"},
	} {
		t.Run(tt.goos+"/"+tt.goarch+" は "+tt.want, func(t *testing.T) {
			if got, err := platformName(tt.goos, tt.goarch); err != nil || got != tt.want {
				t.Errorf("got = %q, err = %v", got, err)
			}
		})
	}
	t.Run("配布しない組は誤りにする", func(t *testing.T) {
		if _, err := platformName("windows", "arm64"); err == nil {
			t.Error("誤りにしていない")
		}
	})
}

func TestWriteZip(t *testing.T) {
	t.Run("フォルダーの中のファイルを、フォルダーの名前を付けずに入れる", func(t *testing.T) {
		dir := t.TempDir()
		os.WriteFile(filepath.Join(dir, "vmeltype.exe"), []byte("exe"), 0o600)
		os.WriteFile(filepath.Join(dir, "LICENSE.txt"), []byte("license"), 0o600)
		out := filepath.Join(t.TempDir(), "a.zip")

		if err := writeZip(dir, out); err != nil {
			t.Fatal(err)
		}
		r, err := zip.OpenReader(out)
		if err != nil {
			t.Fatal(err)
		}
		defer r.Close()
		var names []string
		for _, f := range r.File {
			names = append(names, f.Name)
		}
		sort.Strings(names)
		if len(names) != 2 || names[0] != "LICENSE.txt" || names[1] != "vmeltype.exe" {
			t.Errorf("names = %v", names)
		}
	})
}

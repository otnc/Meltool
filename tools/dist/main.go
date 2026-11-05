// dist は、vMeltype の配布物を作る開発用の道具。"pnpm dist" で動かす。
// vmeltype をバージョンを埋め込んでビルドし、LICENSE と一緒に <out>/vMeltype-<バージョン>-<platform>/ に置く。-zip で、同じ名前の ZIP も作る。
//
//	pnpm dist -version 0.1.0 -goos windows -goarch amd64 -zip
package main

import (
	"archive/zip"
	"flag"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

// platformNames は、配布物の名前に付ける platform。本家の配布物の名前に合わせる。
var platformNames = map[[2]string]string{
	{"windows", "amd64"}: "windows",
	{"darwin", "arm64"}:  "mac",
	{"linux", "amd64"}:   "linux",
	{"linux", "arm64"}:   "linux-arm64",
}

func platformName(goos, goarch string) (string, error) {
	name, ok := platformNames[[2]string{goos, goarch}]
	if !ok {
		return "", fmt.Errorf("%s/%s の配布物は作りません", goos, goarch)
	}
	return name, nil
}

func main() {
	version := flag.String("version", "0.0.0-dev", "埋め込むバージョン")
	goos := flag.String("goos", runtime.GOOS, "対象の OS")
	goarch := flag.String("goarch", runtime.GOARCH, "対象の CPU")
	out := flag.String("out", "dist", "置き場所")
	withZip := flag.Bool("zip", false, "ZIP も作る")
	flag.Parse()
	dir, err := build(*version, *goos, *goarch, *out, *withZip)
	if err != nil {
		fmt.Fprintln(os.Stderr, "dist:", err)
		os.Exit(1)
	}
	fmt.Println(dir)
}

func build(version, goos, goarch, out string, withZip bool) (string, error) {
	platform, err := platformName(goos, goarch)
	if err != nil {
		return "", err
	}
	name := fmt.Sprintf("vMeltype-%s-%s", version, platform)
	dir := filepath.Join(out, name)
	if err := os.RemoveAll(dir); err != nil {
		return "", err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	exe := "vmeltype"
	if goos == "windows" {
		exe += ".exe"
	}
	cmd := exec.Command("go", "build", "-trimpath", "-ldflags", "-s -w -X main.version="+version, "-o", filepath.Join(dir, exe), "./cmd/vmeltype")
	cmd.Env = append(os.Environ(), "GOOS="+goos, "GOARCH="+goarch, "CGO_ENABLED=0")
	cmd.Stdout, cmd.Stderr = os.Stderr, os.Stderr
	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("go build: %w", err)
	}
	if err := copyFile("LICENSE", filepath.Join(dir, "LICENSE.txt")); err != nil {
		return "", err
	}
	if withZip {
		if err := writeZip(dir, filepath.Join(out, name+".zip")); err != nil {
			return "", err
		}
	}
	return dir, nil
}

func copyFile(src, dst string) error {
	data, err := os.ReadFile(src)
	if err != nil {
		return err
	}
	return os.WriteFile(dst, data, 0o644)
}

// writeZip は、dir の直下のファイルを、フォルダーの名前を付けずに ZIP にする。実行ファイルは実行できる権限で入れる。
func writeZip(dir, out string) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	f, err := os.Create(out)
	if err != nil {
		return err
	}
	w := zip.NewWriter(f)
	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		if err := addFile(w, filepath.Join(dir, e.Name()), e.Name()); err != nil {
			w.Close()
			f.Close()
			return err
		}
	}
	if err := w.Close(); err != nil {
		f.Close()
		return err
	}
	return f.Close()
}

func addFile(w *zip.Writer, path, name string) error {
	in, err := os.Open(path)
	if err != nil {
		return err
	}
	defer in.Close()
	header := &zip.FileHeader{Name: name, Method: zip.Deflate}
	mode := os.FileMode(0o644)
	if name == "vmeltype" || name == "vmeltype.exe" {
		mode = 0o755
	}
	header.SetMode(mode)
	dst, err := w.CreateHeader(header)
	if err != nil {
		return err
	}
	_, err = io.Copy(dst, in)
	return err
}

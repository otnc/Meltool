// Package upstreamreg は、本家が「設定 → アプリ」に登録した Meltype の項目を書き換える。項目は作らず、本家が作ったものだけを変える。
package upstreamreg

import (
	"path/filepath"
	"strings"
)

// keys は、本家が登録する項目。setup.exe (Inno Setup) は Meltype_is1、Install.cmd (install.ps1) は Meltype を使う。
var keys = []string{
	`Software\Microsoft\Windows\CurrentVersion\Uninstall\Meltype_is1`,
	`Software\Microsoft\Windows\CurrentVersion\Uninstall\Meltype`,
}

// sameLocation は、登録のインストール先が appDir かを返す。Inno Setup は末尾に \ を付けて記録する。
func sameLocation(location, appDir string) bool {
	return location != "" && strings.EqualFold(filepath.Clean(location), filepath.Clean(appDir))
}

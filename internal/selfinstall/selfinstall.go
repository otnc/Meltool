// Package selfinstall は、vMeltype 自身のインストーラーで入れたかを調べ、インストーラーとアンインストーラーを動かす。
package selfinstall

import (
	"path/filepath"
	"strings"
)

// Installation は、vMeltype のインストーラーで入れた vMeltype。
type Installation struct {
	// Dir は、インストール先。
	Dir string
	// Uninstaller は、インストーラーが置いたアンインストーラー (unins000.exe)。
	Uninstaller string
}

// Holds は、exe がこのインストール先にあるかを返す。Windows のパスなので、大文字と小文字を区別しない。
func (i Installation) Holds(exe string) bool {
	return i.Dir != "" && strings.EqualFold(filepath.Dir(filepath.Clean(exe)), filepath.Clean(i.Dir))
}

// silentArgs は、Inno Setup のインストーラーとアンインストーラーを、画面も確認も出さずに動かす引数。
var silentArgs = []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}

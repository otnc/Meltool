// Package selfinstall は、vMeltype 自身のインストーラーで入れたかを調べ、インストーラーとアンインストーラーを動かす。
package selfinstall

// Installation は、vMeltype のインストーラーで入れた vMeltype。
type Installation struct {
	// Dir は、インストール先。
	Dir string
	// Uninstaller は、インストーラーが置いたアンインストーラー (unins000.exe)。
	Uninstaller string
}

// silentArgs は、Inno Setup のインストーラーとアンインストーラーを、画面も確認も出さずに動かす引数。
var silentArgs = []string{"/VERYSILENT", "/SUPPRESSMSGBOXES", "/NORESTART"}

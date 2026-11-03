//go:build windows

package selfinstall

import "testing"

func TestMatch(t *testing.T) {
	const location = `C:\Users\u\AppData\Local\Programs\vMeltype\`
	const uninstaller = `"C:\Users\u\AppData\Local\Programs\vMeltype\unins000.exe"`
	tests := []struct {
		name string
		exe  string
		want bool
	}{
		{name: "インストール先の vmeltype.exe ならインストーラーで入れたもの", exe: `C:\Users\u\AppData\Local\Programs\vMeltype\vmeltype.exe`, want: true},
		{name: "大文字と小文字の違いは同じ場所とみなす", exe: `c:\users\u\appdata\local\programs\VMELTYPE\vmeltype.exe`, want: true},
		{name: "別の場所の vmeltype.exe は ZIP で入れたもの", exe: `D:\tools\vmeltype.exe`, want: false},
		{name: "インストール先の下のフォルダーは別の場所", exe: `C:\Users\u\AppData\Local\Programs\vMeltype\old\vmeltype.exe`, want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, ok := match(location, uninstaller, tt.exe)

			if ok != tt.want {
				t.Fatalf("ok = %v, want %v", ok, tt.want)
			}
			if ok && got.Uninstaller != `C:\Users\u\AppData\Local\Programs\vMeltype\unins000.exe` {
				t.Errorf("Uninstaller = %q", got.Uninstaller)
			}
		})
	}
	t.Run("アンインストーラーの記録がなければ、インストーラーで入れたものとみなさない", func(t *testing.T) {
		if _, ok := match(location, "", `C:\Users\u\AppData\Local\Programs\vMeltype\vmeltype.exe`); ok {
			t.Error("アンインストーラーがないのに見つけた")
		}
	})
}

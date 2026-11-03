//go:build windows

package selfinstall

import "testing"

func TestParse(t *testing.T) {
	t.Run("インストール先とアンインストーラーを読む", func(t *testing.T) {
		inst, ok := parse(`C:\Users\u\AppData\Local\Programs\vMeltype\`, `"C:\Users\u\AppData\Local\Programs\vMeltype\unins000.exe"`)

		if !ok || inst.Dir != `C:\Users\u\AppData\Local\Programs\vMeltype` || inst.Uninstaller != `C:\Users\u\AppData\Local\Programs\vMeltype\unins000.exe` {
			t.Errorf("inst = %+v, ok = %v", inst, ok)
		}
	})
	t.Run("アンインストーラーの記録がなければ、インストーラーで入れたものとみなさない", func(t *testing.T) {
		if _, ok := parse(`C:\Users\u\AppData\Local\Programs\vMeltype\`, ""); ok {
			t.Error("アンインストーラーがないのに見つけた")
		}
	})
}

func TestHolds(t *testing.T) {
	inst := Installation{Dir: `C:\Users\u\AppData\Local\Programs\vMeltype`}
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
			if got := inst.Holds(tt.exe); got != tt.want {
				t.Errorf("Holds = %v, want %v", got, tt.want)
			}
		})
	}
}

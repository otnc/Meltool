package selfinstall

import "testing"

func TestPathWith(t *testing.T) {
	const dir = `C:\Users\a\AppData\Local\Programs\vMeltype`
	tests := []struct {
		name, paths, want string
		changed           bool
	}{
		{name: "空なら dir だけにする", paths: "", want: dir, changed: true},
		{name: "末尾に ; を挟んで足す", paths: `%USERPROFILE%\bin`, want: `%USERPROFILE%\bin;` + dir, changed: true},
		{name: "末尾が ; なら、そのまま足す", paths: `C:\bin;`, want: `C:\bin;` + dir, changed: true},
		{name: "既にあれば変えない", paths: `C:\bin;` + dir + `;D:\x`, want: `C:\bin;` + dir + `;D:\x`},
		{name: "大文字と小文字、末尾の \\ と空白の違いは同じとみなす", paths: `C:\bin; c:\users\A\appdata\local\programs\VMELTYPE\ `, want: `C:\bin; c:\users\A\appdata\local\programs\VMELTYPE\ `},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := PathWith(tt.paths, dir)
			if got != tt.want || changed != tt.changed {
				t.Errorf("PathWith(%q) = %q, %t, want %q, %t", tt.paths, got, changed, tt.want, tt.changed)
			}
		})
	}
}

func TestPathWithout(t *testing.T) {
	const dir = `C:\Programs\vMeltype`
	tests := []struct {
		name, paths, want string
		changed           bool
	}{
		{name: "dir だけを外し、ほかの項目の順と書き方を変えない", paths: `%USERPROFILE%\bin;` + dir + `;D:\x`, want: `%USERPROFILE%\bin;D:\x`, changed: true},
		{name: "空の項目は残す", paths: `C:\bin;;` + dir + `;`, want: `C:\bin;;`, changed: true},
		{name: "大文字と小文字、末尾の \\ の違いがあっても外す", paths: `c:\programs\VMELTYPE\;C:\bin`, want: `C:\bin`, changed: true},
		{name: "何度も入っていれば全部外す", paths: dir + `;C:\bin;` + dir, want: `C:\bin`, changed: true},
		{name: "なければ変えない", paths: `C:\bin;D:\x`, want: `C:\bin;D:\x`},
		{name: "dir だけなら空にする", paths: dir, want: "", changed: true},
		{name: "別のフォルダーの下は外さない", paths: dir + `\sub;C:\Programs`, want: dir + `\sub;C:\Programs`},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, changed := PathWithout(tt.paths, dir)
			if got != tt.want || changed != tt.changed {
				t.Errorf("PathWithout(%q) = %q, %t, want %q, %t", tt.paths, got, changed, tt.want, tt.changed)
			}
		})
	}
}

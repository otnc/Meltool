package upstreamreg

import (
	"path/filepath"
	"testing"
)

func TestSameLocation(t *testing.T) {
	appDir := filepath.Join("C:", "Users", "u", "AppData", "Local", "Programs", "Meltype")
	tests := []struct {
		name     string
		location string
		want     bool
	}{
		{name: "末尾に区切りがあっても同じ場所とみなす", location: appDir + string(filepath.Separator), want: true},
		{name: "大文字と小文字の違いは同じ場所とみなす", location: filepath.Join("C:", "users", "u", "appdata", "local", "programs", "MELTYPE"), want: true},
		{name: "別の場所の登録は変えない", location: filepath.Join("D:", "Meltype"), want: false},
		{name: "インストール先の記録がなければ変えない", location: "", want: false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := sameLocation(tt.location, appDir); got != tt.want {
				t.Errorf("sameLocation = %v, want %v", got, tt.want)
			}
		})
	}
}

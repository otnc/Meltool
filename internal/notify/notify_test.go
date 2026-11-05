package notify

import (
	"slices"
	"strings"
	"testing"
)

func TestCommand(t *testing.T) {
	const title, body = `vMeltype "x"`, `v1.5.0 があります; $(rm -rf /)`
	t.Run("Windows は Windows PowerShell で出し、文言はスクリプトに埋め込まず環境変数で渡す", func(t *testing.T) {
		c, err := command("windows", title, body)
		if err != nil || c.name != "powershell.exe" {
			t.Fatalf("c = %+v, err = %v", c, err)
		}
		script := c.args[len(c.args)-1]
		if strings.Contains(script, body) || strings.Contains(script, title) {
			t.Errorf("文言をスクリプトに埋め込んだ: %s", script)
		}
		if !slices.Contains(c.env, "VMELTYPE_NOTIFY_TITLE="+title) || !slices.Contains(c.env, "VMELTYPE_NOTIFY_BODY="+body) {
			t.Errorf("env = %v", c.env)
		}
	})
	t.Run("macOS は osascript の引数で渡す", func(t *testing.T) {
		c, err := command("darwin", title, body)
		if err != nil || c.name != "osascript" || c.args[len(c.args)-2] != title || c.args[len(c.args)-1] != body {
			t.Errorf("c = %+v, err = %v", c, err)
		}
		if strings.Contains(strings.Join(c.args[:len(c.args)-2], " "), body) {
			t.Errorf("文言をスクリプトに埋め込んだ: %v", c.args)
		}
	})
	t.Run("Linux は notify-send の引数で渡す", func(t *testing.T) {
		c, err := command("linux", title, body)
		if err != nil || c.name != "notify-send" || c.args[len(c.args)-2] != title || c.args[len(c.args)-1] != body {
			t.Errorf("c = %+v, err = %v", c, err)
		}
	})
	t.Run("ほかの OS は誤りにする", func(t *testing.T) {
		if _, err := command("plan9", title, body); err == nil {
			t.Error("誤りにしていない")
		}
	})
}

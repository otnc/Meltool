// Package notify は、OS の通知 (push 通知) を出す。Windows はトースト、macOS は通知センター、Linux は notify-send を使う。
// どれも OS のコマンドを引数の配列で呼び、文言をスクリプトやシェルの文字列に埋め込まない。足りないコマンドを入れることはしない。
package notify

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"runtime"
	"strings"
)

// toastScript は、Windows PowerShell 5.1 でトーストを出す。PowerShell 7 からは WinRT の型を読めないので使わない。
// 文言は環境変数で受け取る。通知の送り主には、どの PC にもある Windows PowerShell の AppUserModelID を借りる。
const toastScript = `$ErrorActionPreference = 'Stop'
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$xml = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$texts = $xml.GetElementsByTagName('text')
$texts.Item(0).AppendChild($xml.CreateTextNode($env:VMELTYPE_NOTIFY_TITLE)) | Out-Null
$texts.Item(1).AppendChild($xml.CreateTextNode($env:VMELTYPE_NOTIFY_BODY)) | Out-Null
$app = '{1AC14E77-02E7-4E5D-B744-2EB1AE5198B7}\WindowsPowerShell\v1.0\powershell.exe'
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier($app).Show([Windows.UI.Notifications.ToastNotification]::new($xml))`

type osCommand struct {
	name string
	args []string
	env  []string
}

func command(goos, title, body string) (osCommand, error) {
	switch goos {
	case "windows":
		return osCommand{
			name: "powershell.exe",
			args: []string{"-NoProfile", "-NonInteractive", "-WindowStyle", "Hidden", "-Command", toastScript},
			env:  []string{"VMELTYPE_NOTIFY_TITLE=" + title, "VMELTYPE_NOTIFY_BODY=" + body},
		}, nil
	case "darwin":
		return osCommand{name: "osascript", args: []string{
			"-e", "on run argv", "-e", "display notification (item 2 of argv) with title (item 1 of argv)", "-e", "end run", title, body,
		}}, nil
	case "linux":
		return osCommand{name: "notify-send", args: []string{"--app-name=vMeltype", title, body}}, nil
	}
	return osCommand{}, fmt.Errorf("この OS (%s) では、push 通知を出せません", goos)
}

// Send は、OS の通知を出す。
func Send(title, body string) error {
	c, err := command(runtime.GOOS, title, body)
	if err != nil {
		return err
	}
	if _, err := exec.LookPath(c.name); err != nil {
		if runtime.GOOS == "linux" {
			return errors.New("notify-send がないので通知を出せません。libnotify の notify-send (Debian と Ubuntu なら libnotify-bin) を入れてください")
		}
		return fmt.Errorf("%s がないので通知を出せません: %w", c.name, err)
	}
	cmd := exec.Command(c.name, c.args...)
	cmd.Env = append(os.Environ(), c.env...)
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("通知を出せませんでした: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Request は、通知を出せるかを、試しの通知を 1 つ出して確かめる。macOS では、初めて出すときに OS が許可を求める。
// 利用者が OS の設定で通知を止めていても、ここでは分からないことがある。
func Request() error {
	return Send("vMeltype", "push 通知を有効にしました。新しいバージョンを見つけたときなどに、ここで知らせます。")
}

//go:build windows

package selfinstall

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// uninstallKey は、インストーラーが「設定 → アプリ」に登録するキー。Inno Setup は AppId (vMeltype) に _is1 を付けた名前にする。
const uninstallKey = `Software\Microsoft\Windows\CurrentVersion\Uninstall\vMeltype_is1`

// Find は、このユーザーにインストーラーで入れた vMeltype を、「設定 → アプリ」の登録から読む。登録がなければ false を返す。
// いま動いている vmeltype.exe がそこにあるかは、Installation.Holds で調べる。
func Find() (Installation, bool, error) {
	key, err := registry.OpenKey(registry.CURRENT_USER, uninstallKey, registry.QUERY_VALUE)
	if errors.Is(err, registry.ErrNotExist) {
		return Installation{}, false, nil
	}
	if err != nil {
		return Installation{}, false, fmt.Errorf("vMeltype のインストールの登録を読めませんでした: %w", err)
	}
	defer key.Close()
	location, _, err := key.GetStringValue("InstallLocation")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return Installation{}, false, fmt.Errorf("vMeltype のインストール先を読めませんでした: %w", err)
	}
	uninstaller, _, err := key.GetStringValue("UninstallString")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return Installation{}, false, fmt.Errorf("vMeltype のアンインストーラーを読めませんでした: %w", err)
	}
	inst, ok := parse(location, uninstaller)
	return inst, ok, nil
}

func parse(location, uninstallString string) (Installation, bool) {
	uninstaller := strings.Trim(strings.TrimSpace(uninstallString), `"`)
	if location == "" || uninstaller == "" {
		return Installation{}, false
	}
	return Installation{Dir: filepath.Clean(location), Uninstaller: uninstaller}, true
}

// RunSetup は、setup.exe を画面なしで動かし、終わるまで待つ。前と同じインストール先と選択で入れ直す。
func RunSetup(ctx context.Context, setup string) error {
	cmd := exec.CommandContext(ctx, setup, append(silentArgs, "/SP-")...)
	cmd.Dir = os.TempDir()
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("インストーラーが失敗しました: %w", err)
	}
	return nil
}

// StartUninstaller は、アンインストーラーを画面なしで起動し、終わるのを待たない。
// アンインストーラーが vmeltype.exe を消せるよう、呼び出し側はこのあとすぐに終わる。
func StartUninstaller(inst Installation) error {
	cmd := exec.Command(inst.Uninstaller, silentArgs...)
	// アンインストーラーがインストール先を消せるよう、インストール先の外で動かす
	cmd.Dir = os.TempDir()
	if err := cmd.Start(); err != nil {
		return fmt.Errorf("vMeltype のアンインストーラーを起動できませんでした: %w", err)
	}
	return cmd.Process.Release()
}

// environmentKey は、利用者の環境変数のキー。
const environmentKey = `Environment`

// AddToUserPath は、利用者の PATH に dir を足す。既にあれば何もしない。
// PATH は %USERPROFILE% などを含むことがあるので、展開せずに読み、REG_EXPAND_SZ で書き戻す。動いているアプリへの知らせは、インストーラーに任せる。
func AddToUserPath(dir string) error {
	return changeUserPath(func(paths string) (string, bool) { return PathWith(paths, dir) })
}

// RemoveFromUserPath は、利用者の PATH から dir を外す。なければ何もしない。
func RemoveFromUserPath(dir string) error {
	return changeUserPath(func(paths string) (string, bool) { return PathWithout(paths, dir) })
}

func changeUserPath(change func(string) (string, bool)) error {
	key, _, err := registry.CreateKey(registry.CURRENT_USER, environmentKey, registry.QUERY_VALUE|registry.SET_VALUE)
	if err != nil {
		return fmt.Errorf("利用者の環境変数を開けませんでした: %w", err)
	}
	defer key.Close()
	paths, _, err := key.GetStringValue("Path")
	if err != nil && !errors.Is(err, registry.ErrNotExist) {
		return fmt.Errorf("利用者の PATH を読めませんでした: %w", err)
	}
	changed, ok := change(paths)
	if !ok {
		return nil
	}
	if err := key.SetExpandStringValue("Path", changed); err != nil {
		return fmt.Errorf("利用者の PATH を書き換えられませんでした: %w", err)
	}
	return nil
}

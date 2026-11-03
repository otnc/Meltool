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

// Find は、exe (いま動いている vmeltype.exe) が、このユーザーにインストーラーで入れたものかを、「設定 → アプリ」の登録から調べる。
// 登録があっても、exe がインストール先になければ (ZIP から別の場所に置いたものなら)、インストーラーで入れたものとみなさない。
func Find(exe string) (Installation, bool, error) {
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
	inst, ok := match(location, uninstaller, exe)
	return inst, ok, nil
}

func match(location, uninstallString, exe string) (Installation, bool) {
	uninstaller := strings.Trim(strings.TrimSpace(uninstallString), `"`)
	if location == "" || uninstaller == "" {
		return Installation{}, false
	}
	dir := filepath.Clean(location)
	if !strings.EqualFold(filepath.Dir(filepath.Clean(exe)), dir) {
		return Installation{}, false
	}
	return Installation{Dir: dir, Uninstaller: uninstaller}, true
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

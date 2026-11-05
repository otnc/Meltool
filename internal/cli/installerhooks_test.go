package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestAppAddToPath(t *testing.T) {
	t.Run("いま動いている vmeltype.exe のフォルダーを、利用者の PATH に足す", func(t *testing.T) {
		l := newLocalEnv(t)

		got := l.run(t, false, "", "app", "add-to-path")

		if got.code != ExitOK || len(l.pathAdded) != 1 || l.pathAdded[0] != filepath.Dir(testExecutable) {
			t.Errorf("code = %d, added = %v\n%s", got.code, l.pathAdded, got.stderr)
		}
	})
	t.Run("足せなければ失敗する", func(t *testing.T) {
		l := newLocalEnv(t)
		l.pathFails = true

		if got := l.run(t, false, "", "app", "add-to-path"); got.code != ExitError {
			t.Errorf("code = %d", got.code)
		}
	})
	t.Run("インストーラーのためのコマンドなので、一覧には出さない", func(t *testing.T) {
		help := execute(t, "app", "--help")
		if strings.Contains(help.stdout, "add-to-path") {
			t.Errorf("一覧に出ている:\n%s", help.stdout)
		}
	})
}

func TestAppUninstallFromInstallerRemovesPath(t *testing.T) {
	t.Run("管理ルートを片付けたあと、vmeltype.exe のフォルダーを PATH から外す", func(t *testing.T) {
		l := managedStandard(t)
		l.installed = true

		got := l.run(t, false, "", "app", "uninstall", "--from-installer", "-y")

		if got.code != ExitOK || len(l.pathRemoved) != 1 || l.pathRemoved[0] != filepath.Dir(testExecutable) {
			t.Errorf("code = %d, removed = %v\n%s", got.code, l.pathRemoved, got.stderr)
		}
	})
	t.Run("管理ルートがなくても外す", func(t *testing.T) {
		l := newLocalEnv(t)
		os.Remove(l.root)
		l.installed = true

		got := l.run(t, false, "", "app", "uninstall", "--from-installer", "-y")

		if got.code != ExitOK || len(l.pathRemoved) != 1 {
			t.Errorf("code = %d, removed = %v\n%s", got.code, l.pathRemoved, got.stderr)
		}
	})
	t.Run("外せなくてもアンインストールは止めず、警告する", func(t *testing.T) {
		l := managedStandard(t)
		l.installed = true
		l.pathFails = true

		got := l.run(t, false, "", "app", "uninstall", "--from-installer", "-y")

		if got.code != ExitOK || !strings.Contains(got.stderr, "警告:") {
			t.Errorf("code = %d\n%s", got.code, got.stderr)
		}
	})
	t.Run("アンインストーラーから呼ばれていなければ、PATH はアンインストーラーに任せて触れない", func(t *testing.T) {
		l := managedStandard(t)
		l.installed = true

		l.run(t, false, "", "app", "uninstall", "-y")

		if len(l.pathRemoved) != 0 {
			t.Errorf("removed = %v", l.pathRemoved)
		}
	})
	t.Run("--dry-run では外さない", func(t *testing.T) {
		l := managedStandard(t)
		l.installed = true

		l.run(t, false, "", "app", "uninstall", "--from-installer", "--dry-run")

		if len(l.pathRemoved) != 0 {
			t.Errorf("removed = %v", l.pathRemoved)
		}
	})
}

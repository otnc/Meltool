package cli

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/privatefs"
)

// appDoctorCheck は、app doctor の結果から name の項目を返す。
func appDoctorCheck(t *testing.T, got result, name string) check {
	t.Helper()
	var body struct {
		OK     bool
		Checks []check
	}
	if err := json.Unmarshal([]byte(got.stdout), &body); err != nil {
		t.Fatalf("JSON を読めない: %v\n%s%s", err, got.stdout, got.stderr)
	}
	for _, c := range body.Checks {
		if c.Name == name {
			return c
		}
	}
	t.Fatalf("%s の項目がない: %s", name, got.stdout)
	return check{}
}

// placeExe は、dir に vmeltype.exe を置き、そのパスを返す。
func placeExe(t *testing.T, dir string) string {
	t.Helper()
	os.MkdirAll(dir, 0o700)
	exe := filepath.Join(dir, "vmeltype.exe")
	if err := os.WriteFile(exe, []byte("vmeltype"), 0o700); err != nil {
		t.Fatal(err)
	}
	return exe
}

func TestAppDoctor(t *testing.T) {
	t.Run("問題がなければ終了コード 0 で、self doctor でも同じ", func(t *testing.T) {
		for _, args := range [][]string{{"app", "doctor"}, {"self", "doctor"}} {
			l := newLocalEnv(t)
			// 本物と同じく、本人だけが使える権限で作る。t.TempDir は %TEMP% の権限を継承するので使わない
			l.root = filepath.Join(t.TempDir(), "root")
			if err := privatefs.MkdirAll(l.root); err != nil {
				t.Fatal(err)
			}
			l.exe = placeExe(t, filepath.Join(t.TempDir(), "bin"))
			l.path = filepath.Dir(l.exe)

			got := l.run(t, false, "", args...)

			if got.code != ExitOK || !strings.Contains(got.stdout, "1.2.3") {
				t.Errorf("%v: code = %d\n%s%s", args, got.code, got.stdout, got.stderr)
			}
		}
	})
	t.Run("インストーラーで入れたものか、ZIP から置いたものかを示す", func(t *testing.T) {
		installed := newLocalEnv(t)
		installed.installed = true
		archive := newLocalEnv(t)

		got := installed.run(t, false, "", "app", "doctor", "--json")
		other := archive.run(t, false, "", "app", "doctor", "--json")

		if c := appDoctorCheck(t, got, "install"); c.Level != levelOK || !strings.Contains(c.Message, "インストーラー") {
			t.Errorf("インストーラー: %+v", c)
		}
		if c := appDoctorCheck(t, other, "install"); c.Level != levelOK || !strings.Contains(c.Message, "ZIP") {
			t.Errorf("ZIP: %+v", c)
		}
	})
	t.Run("インストーラーで入れたものとは別の vmeltype.exe を使っていれば警告する", func(t *testing.T) {
		l := newLocalEnv(t)
		l.installed = true
		l.exe = placeExe(t, filepath.Join(t.TempDir(), "other"))

		got := l.run(t, false, "", "app", "doctor", "--json")

		if c := appDoctorCheck(t, got, "install"); c.Level != levelWarn || c.Hint == "" {
			t.Errorf("%+v", c)
		}
	})
	t.Run("PATH の先のほうに別の vmeltype.exe があれば警告する", func(t *testing.T) {
		l := newLocalEnv(t)
		l.exe = placeExe(t, filepath.Join(t.TempDir(), "mine"))
		earlier := filepath.Dir(placeExe(t, filepath.Join(t.TempDir(), "earlier")))
		l.path = strings.Join([]string{earlier, filepath.Dir(l.exe)}, string(os.PathListSeparator))

		got := l.run(t, false, "", "app", "doctor", "--json")

		if c := appDoctorCheck(t, got, "path"); c.Level != levelWarn || !strings.Contains(c.Message, earlier) {
			t.Errorf("%+v", c)
		}
	})
	t.Run("vmeltype.exe のフォルダーが PATH になければ警告する", func(t *testing.T) {
		l := newLocalEnv(t)
		l.exe = placeExe(t, filepath.Join(t.TempDir(), "mine"))

		got := l.run(t, false, "", "app", "doctor", "--json")

		if c := appDoctorCheck(t, got, "path"); c.Level != levelWarn || c.Hint == "" {
			t.Errorf("%+v", c)
		}
	})
	t.Run("中断した変更があれば、app uninstall が止まることを警告する", func(t *testing.T) {
		l := interruptedUse(t, lifecycle.PhaseOldMoved)

		got := l.run(t, false, "", "app", "doctor", "--json")

		if c := appDoctorCheck(t, got, "uninstall"); c.Level != levelWarn || !strings.Contains(c.Hint, "recover") {
			t.Errorf("%+v", c)
		}
	})
	t.Run("管理ルートがなければ作らない", func(t *testing.T) {
		l := newLocalEnv(t)
		os.Remove(l.root)

		got := l.run(t, false, "", "app", "doctor")

		if got.code != ExitOK {
			t.Errorf("code = %d\n%s%s", got.code, got.stdout, got.stderr)
		}
		if _, err := os.Stat(l.root); !os.IsNotExist(err) {
			t.Errorf("管理ルートを作った")
		}
	})
	t.Run("ほかの利用者も管理ルートを使えるなら要対応にする", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("ACL での判定は privatefs で確かめる。ここでは権限の数字で変えられる OS だけで確かめる")
		}
		l := newLocalEnv(t)
		os.Chmod(l.root, 0o755)

		got := l.run(t, false, "", "app", "doctor", "--json")

		if got.code != ExitNeedsAction {
			t.Errorf("code = %d", got.code)
		}
		if c := appDoctorCheck(t, got, "root-access"); c.Level != levelFail || c.Hint == "" {
			t.Errorf("%+v", c)
		}
	})
}

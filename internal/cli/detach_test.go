package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/snapshot"
)

func (l *localEnv) state(t *testing.T) lifecycle.State {
	t.Helper()
	s, err := lifecycle.LoadState(l.root)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func TestDetachStandard(t *testing.T) {
	t.Run("実行先とデータを残し、本家の自動更新を戻して、管理の記録を外す", func(t *testing.T) {
		l := managedStandard(t)
		l.run(t, false, "", "pin")
		snapshots, _, _ := snapshot.List(l.root)

		got := l.run(t, false, "", "detach", "-y")

		if got.code != ExitOK {
			t.Fatalf("code = %d\n%s%s", got.code, got.stdout, got.stderr)
		}
		if s := l.state(t); s.Active != nil || s.Previous != nil || s.Pin != nil {
			t.Errorf("管理の記録が残っている: %+v", s)
		}
		if l.autoUpdate(t) != true {
			t.Errorf("本家の自動更新を戻していない: %v", l.autoUpdate(t))
		}
		if readFile(filepath.Join(l.standardAppDir(), "Meltype.exe")) != "exe v1.4.0" || readFile(filepath.Join(l.standardDataDir(), "userdict.txt")) != "辞書" {
			t.Errorf("実行先かデータを残していない")
		}
		if ids := storedIDs(t, l.root); len(ids) != 1 {
			t.Errorf("保持バージョンを残していない: %v", ids)
		}
		if after, _, _ := snapshot.List(l.root); len(after) != len(snapshots) {
			t.Errorf("スナップショットの数が変わった: %d → %d", len(snapshots), len(after))
		}
	})
	t.Run("管理をやめたあとも、use でまた管理を始められる", func(t *testing.T) {
		l := managedStandard(t)
		l.run(t, false, "", "detach", "-y")

		got := l.run(t, false, "", "use", "1.4.0", "-y")

		if got.code != ExitOK || l.state(t).Active == nil || l.autoUpdate(t) != false {
			t.Errorf("code = %d, autoUpdate = %v\n%s", got.code, l.autoUpdate(t), got.stderr)
		}
	})
	t.Run("--dry-run は何も変えない", func(t *testing.T) {
		l := managedStandard(t)

		got := l.run(t, false, "", "detach", "--dry-run")

		if got.code != ExitOK || l.state(t).Active == nil || l.autoUpdate(t) != false {
			t.Errorf("code = %d, autoUpdate = %v\n%s", got.code, l.autoUpdate(t), got.stderr)
		}
		if !strings.Contains(got.stdout, l.standardAppDir()) {
			t.Errorf("残す実行先を示していない:\n%s", got.stdout)
		}
	})
	t.Run("確認できない環境で -y がなければ、何も変えずに止まる", func(t *testing.T) {
		l := managedStandard(t)

		got := l.run(t, false, "", "detach")

		if got.code != ExitNeedsAction || l.state(t).Active == nil {
			t.Errorf("code = %d\n%s", got.code, got.stderr)
		}
	})
	t.Run("対話で既定 (Enter) を選んだら、何も変えない", func(t *testing.T) {
		l := managedStandard(t)

		got := l.run(t, true, "\n", "detach")

		if got.code != ExitCanceled || l.state(t).Active == nil {
			t.Errorf("code = %d\n%s", got.code, got.stderr)
		}
	})
	t.Run("動いている Meltype は終了させ、終わったら起動し直す", func(t *testing.T) {
		l := managedStandard(t)
		l.running = []string{filepath.Join(l.standardAppDir(), "Meltype.exe")}

		got := l.run(t, false, "", "detach", "-y")

		if got.code != ExitOK || l.stopped != 1 || l.started != 1 {
			t.Errorf("code = %d, stopped = %d, started = %d\n%s", got.code, l.stopped, l.started, got.stderr)
		}
	})
}

func TestDetachPortable(t *testing.T) {
	t.Run("--to の先へ app と data を写し、管理ルートの中のものを外す", func(t *testing.T) {
		l := newPortableWithData(t)
		dest := filepath.Join(t.TempDir(), "Meltype")

		got := l.run(t, false, "", "detach", "--to", dest, "-y")

		if got.code != ExitOK {
			t.Fatalf("code = %d\n%s%s", got.code, got.stdout, got.stderr)
		}
		if readFile(filepath.Join(dest, "Meltype.exe")) != "exe v1.4.0" || readFile(filepath.Join(dest, "data", "userdict.txt")) != "保存した辞書" {
			t.Errorf("写していない")
		}
		if _, err := os.Stat(l.appDir()); !os.IsNotExist(err) {
			t.Errorf("管理ルートの中の実行先が残っている")
		}
		if l.state(t).Active != nil {
			t.Errorf("管理の記録が残っている")
		}
	})
	t.Run("終了させた Meltype は、写した先で起動し直す", func(t *testing.T) {
		l := newPortableWithData(t)
		l.running = []string{filepath.Join(l.appDir(), "Meltype.exe")}
		dest := filepath.Join(t.TempDir(), "Meltype")
		var startedIn []string
		l.onStart = func(dir string) { startedIn = append(startedIn, dir) }

		got := l.run(t, false, "", "detach", "--to", dest, "-y")

		if got.code != ExitOK || l.stopped != 1 || len(startedIn) != 1 || startedIn[0] != dest {
			t.Errorf("code = %d, stopped = %d, startedIn = %v\n%s", got.code, l.stopped, startedIn, got.stderr)
		}
	})
	for _, tt := range []struct {
		name     string
		args     func(l *localEnv, t *testing.T) []string
		wantCode int
	}{
		{name: "--to がなければ、何も変えずに --to を求める", wantCode: ExitUsage, args: func(*localEnv, *testing.T) []string { return []string{"detach", "-y"} }},
		{name: "--to の先が空でなければ、何も変えない", wantCode: ExitError, args: func(_ *localEnv, t *testing.T) []string {
			dest := t.TempDir()
			os.WriteFile(filepath.Join(dest, "other.txt"), []byte("x"), 0o600)
			return []string{"detach", "--to", dest, "-y"}
		}},
		{name: "--to の先が管理ルートの中なら、何も変えない", wantCode: ExitError, args: func(l *localEnv, _ *testing.T) []string {
			return []string{"detach", "--to", filepath.Join(l.root, "moved"), "-y"}
		}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			l := newPortableWithData(t)

			got := l.run(t, false, "", tt.args(l, t)...)

			if got.code != tt.wantCode || l.state(t).Active == nil || l.read(t, "data/userdict.txt") != "保存した辞書" {
				t.Errorf("code = %d, want %d\n%s", got.code, tt.wantCode, got.stderr)
			}
			if !strings.Contains(got.stderr, "--to") {
				t.Errorf("--to のことを示していない: %s", got.stderr)
			}
		})
	}
}

func TestDetachWithoutActiveVersion(t *testing.T) {
	got := newLocalEnv(t).run(t, false, "", "detach", "-y")

	if got.code != ExitError || !strings.Contains(got.stderr, "有効なバージョンがない") {
		t.Errorf("code = %d, stderr = %s", got.code, got.stderr)
	}
}

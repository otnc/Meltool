package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/store"
	"github.com/otnc/vmeltype/internal/winime"
)

func (l *localEnv) deployedExe() string {
	return readFile(filepath.Join(l.standardAppDir(), "Meltype.exe"))
}

func TestApplyRepairsActiveVersion(t *testing.T) {
	t.Run("すべて一致していれば何もしない", func(t *testing.T) {
		l := managedStandard(t)
		l.stopped = 0

		got := l.run(t, false, "", "apply", "-y")

		if got.code != ExitOK || !strings.Contains(got.stdout, "直すことはありません") {
			t.Errorf("code = %d\n%s%s", got.code, got.stdout, got.stderr)
		}
	})
	t.Run("実行先が保持バージョンと違えば置き直す", func(t *testing.T) {
		l := managedStandard(t)
		os.WriteFile(filepath.Join(l.standardAppDir(), "Meltype.exe"), []byte("壊れた"), 0o600)

		got := l.run(t, false, "", "apply", "-y")

		if got.code != ExitOK || l.deployedExe() != "exe v1.4.0" || !strings.Contains(got.stdout, "置き直しました") {
			t.Errorf("code = %d, exe = %q\n%s%s", got.code, l.deployedExe(), got.stdout, got.stderr)
		}
	})
	t.Run("本家の自動更新が動いていれば、置き直して止める", func(t *testing.T) {
		l := managedStandard(t)
		os.WriteFile(filepath.Join(l.standardDataDir(), "config.json"), []byte(`{"AutoUpdate": true}`), 0o600)

		got := l.run(t, false, "", "apply", "-y")

		if got.code != ExitOK || l.autoUpdate(t) != false {
			t.Errorf("code = %d, AutoUpdate = %v\n%s%s", got.code, l.autoUpdate(t), got.stdout, got.stderr)
		}
	})
	t.Run("登録済みの IME の DLL が違えば、置き直して入れ替える", func(t *testing.T) {
		l := managedStandard(t)
		l.imeRegistered = true
		l.imeResult = winime.ResultReplaced
		l.installedTip = map[string]string{"x64": "OLD", "x86": "OLD"}
		l.imeCalls = nil

		got := l.run(t, false, "", "apply", "-y")

		if got.code != ExitOK || len(l.imeCalls) != 1 {
			t.Errorf("code = %d, calls = %+v\n%s%s", got.code, l.imeCalls, got.stdout, got.stderr)
		}
	})
	t.Run("--force なら、一致していても置き直す", func(t *testing.T) {
		l := managedStandard(t)

		got := l.run(t, false, "", "apply", "--force", "-y")

		if got.code != ExitOK || !strings.Contains(got.stdout, "置き直しました") {
			t.Errorf("code = %d\n%s%s", got.code, got.stdout, got.stderr)
		}
	})
	t.Run("固定していても、同じバージョンなので置き直せる", func(t *testing.T) {
		l := managedStandard(t)
		l.run(t, false, "", "pin")
		os.WriteFile(filepath.Join(l.standardAppDir(), "Meltype.exe"), []byte("壊れた"), 0o600)

		got := l.run(t, false, "", "apply", "-y")

		if got.code != ExitOK || l.deployedExe() != "exe v1.4.0" {
			t.Errorf("code = %d\n%s", got.code, got.stderr)
		}
	})
	t.Run("--dry-run は何も変えない", func(t *testing.T) {
		l := managedStandard(t)
		os.WriteFile(filepath.Join(l.standardAppDir(), "Meltype.exe"), []byte("壊れた"), 0o600)

		got := l.run(t, false, "", "apply", "--ime", "--dry-run")

		if got.code != ExitOK || l.deployedExe() != "壊れた" || len(l.registerCalls) != 0 {
			t.Errorf("code = %d, exe = %q, register = %v\n%s%s", got.code, l.deployedExe(), l.registerCalls, got.stdout, got.stderr)
		}
	})
}

func TestApplyRegistersIME(t *testing.T) {
	t.Run("--ime で、登録していなければ、有効なバージョンの資材で登録する", func(t *testing.T) {
		l := managedStandard(t)
		l.registerResult = winime.ResultRegistered

		got := l.run(t, false, "", "apply", "--ime", "-y")

		want := filepath.Join(store.Store{Root: l.root}.Path(store.ID("v1.4.0", "windows", "amd64", "standard")), "files", "app")
		if got.code != ExitOK || len(l.registerCalls) != 1 || l.registerCalls[0] != want {
			t.Fatalf("code = %d, calls = %v\n%s%s", got.code, l.registerCalls, got.stdout, got.stderr)
		}
		if !strings.Contains(got.stdout+got.stderr, "既定の入力方式は変えません") {
			t.Errorf("既定を変えないことを示していない:\n%s%s", got.stdout, got.stderr)
		}
	})
	t.Run("--ime でも、登録済みなら登録し直さない", func(t *testing.T) {
		l := managedStandard(t)
		l.imeRegistered = true
		l.imeResult = winime.ResultCurrent

		got := l.run(t, false, "", "apply", "--ime", "-y")

		if got.code != ExitOK || len(l.registerCalls) != 0 {
			t.Errorf("code = %d, calls = %v\n%s", got.code, l.registerCalls, got.stderr)
		}
	})
	t.Run("一覧に足せなければ、登録はしたので成功にし、足し方を示す", func(t *testing.T) {
		l := managedStandard(t)
		l.registerResult = winime.ResultUnlisted

		got := l.run(t, false, "", "apply", "--ime", "-y")

		if got.code != ExitOK || !strings.Contains(got.stderr, "警告:") {
			t.Errorf("code = %d\n%s%s", got.code, got.stdout, got.stderr)
		}
	})
	t.Run("UAC で断られたら、対処が要ることを返す", func(t *testing.T) {
		l := managedStandard(t)
		l.registerResult = winime.ResultCanceled

		got := l.run(t, false, "", "apply", "--ime", "-y")

		if got.code != ExitNeedsAction {
			t.Errorf("code = %d\n%s", got.code, got.stderr)
		}
	})
}

func TestApplyStopsBeforeChanging(t *testing.T) {
	tests := []struct {
		name     string
		prepare  func(t *testing.T) *localEnv
		args     []string
		wantCode int
		wantText string
	}{
		{
			name:     "有効なバージョンがなければ、use を案内する",
			prepare:  newLocalEnv,
			args:     []string{"apply", "-y"},
			wantCode: ExitError,
			wantText: "use",
		},
		{
			name:     "中断した変更があれば、recover を案内する",
			prepare:  func(t *testing.T) *localEnv { return interruptedUse(t, lifecycle.PhaseOldMoved) },
			args:     []string{"apply", "-y"},
			wantCode: ExitNeedsAction,
			wantText: "recover",
		},
		{
			name: "portable 版で --ime なら、通常版の取得と use を案内する",
			prepare: func(t *testing.T) *localEnv {
				l := newLocalEnv(t)
				l.storePortable(t, "v1.4.0", nil)
				l.run(t, false, "", "use", "1.4.0", "--portable", "-y")
				return l
			},
			args:     []string{"apply", "--ime", "-y"},
			wantCode: ExitError,
			wantText: "通常版",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			l := tt.prepare(t)
			before := rootEntries(l.root)

			got := l.run(t, false, "", tt.args...)

			if got.code != tt.wantCode || !strings.Contains(got.stdout+got.stderr, tt.wantText) {
				t.Errorf("code = %d, want %d\n%s%s", got.code, tt.wantCode, got.stdout, got.stderr)
			}
			if strings.Join(rootEntries(l.root), ",") != strings.Join(before, ",") || len(l.registerCalls) != 0 {
				t.Errorf("止めるべきなのに変えた: %v", rootEntries(l.root))
			}
		})
	}
}

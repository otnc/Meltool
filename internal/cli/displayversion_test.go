package cli

import (
	"strings"
	"testing"

	"github.com/otnc/vmeltype/internal/lifecycle"
)

type displayVersionCall struct {
	appDir, version string
}

func (l *localEnv) lastDisplayVersion(t *testing.T) displayVersionCall {
	t.Helper()
	if len(l.displayVersions) == 0 {
		t.Fatal("「設定 → アプリ」のバージョンを書き換えていない")
	}
	return l.displayVersions[len(l.displayVersions)-1]
}

func TestDisplayVersion(t *testing.T) {
	t.Run("通常版を切り替えたら、実行先の登録のバージョンを書き換える", func(t *testing.T) {
		l := newLocalEnv(t)
		l.storeStandard(t, "v1.3.0")
		l.storeStandard(t, "v1.4.0")
		l.installUpstream(t)
		l.run(t, false, "", "use", "1.3.0", "-y")

		got := l.run(t, false, "", "use", "1.4.0", "-y")

		if got.code != ExitOK {
			t.Fatalf("code = %d\n%s", got.code, got.stderr)
		}
		if call := l.lastDisplayVersion(t); call.appDir != l.standardAppDir() || call.version != "1.4.0" {
			t.Errorf("%+v", call)
		}
	})
	t.Run("portable 版では書き換えない", func(t *testing.T) {
		l := newLocalEnv(t)
		l.storePortable(t, "v1.4.0", nil)

		l.run(t, false, "", "use", "1.4.0", "--portable", "-y")

		if len(l.displayVersions) != 0 {
			t.Errorf("%+v", l.displayVersions)
		}
	})
	t.Run("書き換えられなくても切り替えは成功にし、警告する", func(t *testing.T) {
		l := newLocalEnv(t)
		l.storeStandard(t, "v1.4.0")
		l.installUpstream(t)
		l.displayFails = true

		got := l.run(t, false, "", "use", "1.4.0", "-y")

		if got.code != ExitOK || !strings.Contains(got.stderr, "警告:") {
			t.Errorf("code = %d\n%s", got.code, got.stderr)
		}
	})
	t.Run("recover では、実行先に残ったバージョンにする", func(t *testing.T) {
		for phase, want := range map[string]string{lifecycle.PhaseOldMoved: "1.3.0", lifecycle.PhaseNewPlaced: "1.4.0"} {
			l := interruptedStandardUse(t, phase)
			l.displayVersions = nil

			got := l.run(t, false, "", "recover", "-y")

			if got.code != ExitOK {
				t.Fatalf("%s: code = %d\n%s", phase, got.code, got.stderr)
			}
			if call := l.lastDisplayVersion(t); call.version != want {
				t.Errorf("%s: %+v, want %s", phase, call, want)
			}
		}
	})
}

package cli

import (
	"encoding/json"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/store"
)

// withPending は、通常版の v1.4.0 が有効で、v1.5.0 を保持し、v1.5.0 への適用待ちがある管理ルートを作る。
func withPending(t *testing.T) *localEnv {
	t.Helper()
	l := managedStandard(t)
	l.storeStandard(t, "v1.5.0")
	l.savePending(t, "v1.5.0")
	return l
}

func (l *localEnv) savePending(t *testing.T, tag string) {
	t.Helper()
	p := lifecycle.Pending{StoreID: store.ID(tag, "windows", "amd64", "standard"), Tag: tag, Variant: "standard", Reason: "Meltype が動いていた", CreatedAt: time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)}
	if err := lifecycle.SavePending(l.root, p); err != nil {
		t.Fatal(err)
	}
}

func (l *localEnv) pending(t *testing.T) *lifecycle.Pending {
	t.Helper()
	p, err := lifecycle.LoadPending(l.root)
	if err != nil {
		t.Fatal(err)
	}
	return p
}

func (l *localEnv) deployed() string {
	return readFile(filepath.Join(l.standardAppDir(), "Meltype.exe"))
}

func TestApplyPending(t *testing.T) {
	t.Run("適用待ちのバージョンに切り替え、適用待ちを消す", func(t *testing.T) {
		l := withPending(t)

		got := l.run(t, false, "", "apply", "-y")

		if got.code != ExitOK || l.deployed() != "exe v1.5.0" || l.pending(t) != nil {
			t.Errorf("code = %d, deployed = %q, pending = %+v\n%s%s", got.code, l.deployed(), l.pending(t), got.stdout, got.stderr)
		}
	})
	t.Run("--dry-run は対象と理由を示し、何も変えない", func(t *testing.T) {
		l := withPending(t)

		got := l.run(t, false, "", "apply", "--dry-run")

		if got.code != ExitOK || l.deployed() != "exe v1.4.0" || l.pending(t) == nil {
			t.Errorf("code = %d, deployed = %q\n%s", got.code, l.deployed(), got.stderr)
		}
		if !strings.Contains(got.stdout, "v1.5.0") || !strings.Contains(got.stdout, "Meltype が動いていた") {
			t.Errorf("対象か理由を示していない:\n%s", got.stdout)
		}
	})
	t.Run("切り替えられなければ、適用待ちを残す", func(t *testing.T) {
		l := withPending(t)

		got := l.run(t, false, "", "apply")

		if got.code != ExitNeedsAction || l.deployed() != "exe v1.4.0" || l.pending(t) == nil {
			t.Errorf("code = %d, deployed = %q, pending = %+v", got.code, l.deployed(), l.pending(t))
		}
	})
	t.Run("固定していれば止め、--override-pin なら切り替えて固定を移す", func(t *testing.T) {
		l := withPending(t)
		l.run(t, false, "", "pin")

		pinned := l.run(t, false, "", "apply", "-y")
		if pinned.code != ExitNeedsAction || l.deployed() != "exe v1.4.0" || l.pending(t) == nil {
			t.Fatalf("code = %d, deployed = %q\n%s", pinned.code, l.deployed(), pinned.stderr)
		}
		overridden := l.run(t, false, "", "apply", "--override-pin", "-y")
		if overridden.code != ExitOK || l.deployed() != "exe v1.5.0" || l.state(t).Pin == nil || l.state(t).Pin.Tag != "v1.5.0" {
			t.Errorf("code = %d, deployed = %q, pin = %+v\n%s", overridden.code, l.deployed(), l.state(t).Pin, overridden.stderr)
		}
	})
	t.Run("適用待ちの保持バージョンがなければ、何も変えずに --discard を示す", func(t *testing.T) {
		l := managedStandard(t)
		l.savePending(t, "v1.6.0")

		got := l.run(t, false, "", "apply", "-y")

		if got.code != ExitError || !strings.Contains(got.stderr, "--discard") || l.pending(t) == nil || l.deployed() != "exe v1.4.0" {
			t.Errorf("code = %d\n%s", got.code, got.stderr)
		}
	})
	t.Run("適用待ちが有効なバージョンと同じなら、消して修復だけをする", func(t *testing.T) {
		l := managedStandard(t)
		l.savePending(t, "v1.4.0")

		got := l.run(t, false, "", "apply", "-y")

		if got.code != ExitOK || l.pending(t) != nil || !strings.Contains(got.stdout, "直すことはありません") {
			t.Errorf("code = %d, pending = %+v\n%s%s", got.code, l.pending(t), got.stdout, got.stderr)
		}
	})
}

func TestApplyDiscard(t *testing.T) {
	t.Run("適用待ちを取り消し、有効なバージョンは変えない", func(t *testing.T) {
		l := withPending(t)

		got := l.run(t, false, "", "apply", "--discard")

		if got.code != ExitOK || l.pending(t) != nil || l.deployed() != "exe v1.4.0" || !strings.Contains(got.stdout, "v1.5.0") {
			t.Errorf("code = %d, pending = %+v\n%s", got.code, l.pending(t), got.stdout)
		}
	})
	t.Run("適用待ちがなければ、そう示す", func(t *testing.T) {
		got := managedStandard(t).run(t, false, "", "apply", "--discard")

		if got.code != ExitOK || !strings.Contains(got.stdout, "適用待ちの要求はありません") {
			t.Errorf("code = %d\n%s", got.code, got.stdout)
		}
	})
}

func TestPendingIsShownAndProtected(t *testing.T) {
	t.Run("status で適用待ちを示す", func(t *testing.T) {
		l := withPending(t)

		got := l.run(t, false, "", "status")
		asJSON := l.run(t, false, "", "status", "--json")

		if !strings.Contains(got.stdout, "適用待ち:") || !strings.Contains(got.stdout, "v1.5.0") || !strings.Contains(got.stdout, "vmeltype apply") {
			t.Errorf("status:\n%s", got.stdout)
		}
		var s struct {
			PendingRequest *lifecycle.Pending `json:"pendingRequest"`
		}
		if err := json.Unmarshal([]byte(asJSON.stdout), &s); err != nil || s.PendingRequest == nil || s.PendingRequest.Tag != "v1.5.0" {
			t.Errorf("json = %s, err = %v", asJSON.stdout, err)
		}
	})
	t.Run("doctor で適用待ちを警告する", func(t *testing.T) {
		got := withPending(t).run(t, false, "", "doctor")

		if !strings.Contains(got.stdout, "[警告]") || !strings.Contains(got.stdout, "適用待ち") {
			t.Errorf("doctor:\n%s", got.stdout)
		}
	})
	t.Run("適用待ちの保持バージョンは uninstall で消さない", func(t *testing.T) {
		l := withPending(t)

		got := l.run(t, false, "", "uninstall", "v1.5.0", "-y")

		if got.code != ExitError || !strings.Contains(got.stderr, "適用待ち") || len(storedIDs(t, l.root)) != 2 {
			t.Errorf("code = %d, stored = %v\n%s", got.code, storedIDs(t, l.root), got.stderr)
		}
	})
	t.Run("use で適用待ちのバージョンに切り替えたら、適用待ちを消す", func(t *testing.T) {
		l := withPending(t)

		got := l.run(t, false, "", "use", "1.5.0", "-y")

		if got.code != ExitOK || l.pending(t) != nil {
			t.Errorf("code = %d, pending = %+v\n%s", got.code, l.pending(t), got.stderr)
		}
	})
	t.Run("use で別のバージョンに切り替えても、適用待ちは残す", func(t *testing.T) {
		l := withPending(t)
		l.storeStandard(t, "v1.3.0")

		l.run(t, false, "", "use", "1.3.0", "-y")

		if l.pending(t) == nil {
			t.Errorf("適用待ちを消した")
		}
	})
}

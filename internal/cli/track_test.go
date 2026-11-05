package cli

import (
	"encoding/json"
	"strings"
	"testing"
	"time"

	"github.com/otnc/vmeltype/internal/lifecycle"
)

type trackRegisterCall struct {
	exe, root string
	interval  time.Duration
}

// standardZip は、storeStandard と同じ中身の、本家の通常版の ZIP を作る。
func standardZip(t *testing.T, tag string) []byte {
	t.Helper()
	return makeZip(t, map[string]string{
		"app/Meltype.exe":                   "exe " + tag,
		"app/dotnet/host.dll":               "dll " + tag,
		"app/tip/x64/MeltypeTip.dll":        "tip " + tag,
		"app/tip/x64/MeltypeTip.dll.sha256": "hash-x64-" + tag,
		"app/tip/x86/MeltypeTip.dll.sha256": "hash-x86-" + tag,
		"install.ps1":                       "install " + tag,
		"uninstall.ps1":                     "uninstall " + tag,
	})
}

// tracking は、通常版の v1.4.0 が有効で、公式一覧に正式版の v1.5.0 とプレリリースの v1.6.0-pre.1 がある環境で、追従を有効にする。
func tracking(t *testing.T, args ...string) *localEnv {
	t.Helper()
	l := managedStandard(t)
	stable, pre := standardZip(t, "v1.5.0"), standardZip(t, "v1.6.0-pre.1")
	l.upstream = newFakeUpstream(t,
		fakeRelease{tag: "v1.4.0", zip: standardZip(t, "v1.4.0"), digest: digestOf(standardZip(t, "v1.4.0"))},
		fakeRelease{tag: "v1.5.0", zip: stable, digest: digestOf(stable)},
		fakeRelease{tag: "v1.6.0-pre.1", prerelease: true, zip: pre, digest: digestOf(pre)},
	)
	if got := l.run(t, false, "", append([]string{"track", "enable"}, args...)...); got.code != ExitOK {
		t.Fatalf("track enable: %d\n%s", got.code, got.stderr)
	}
	return l
}

func (l *localEnv) trackState(t *testing.T) (lifecycle.TrackSettings, *lifecycle.TrackRun) {
	t.Helper()
	s, last, err := lifecycle.LoadTrack(l.root)
	if err != nil {
		t.Fatal(err)
	}
	return s, last
}

func TestTrackEnable(t *testing.T) {
	t.Run("既定は正式版、notify、1 日ごとで、OS のスケジューラーに登録する", func(t *testing.T) {
		l := tracking(t)

		s, _ := l.trackState(t)
		if !s.Enabled || s.Channel != lifecycle.TrackStable || s.Mode != lifecycle.TrackNotify || s.Interval() != 24*time.Hour || s.Push {
			t.Errorf("settings = %+v", s)
		}
		if len(l.registrations) != 1 || l.registrations[0].exe != testExecutable || l.registrations[0].root != l.root || l.registrations[0].interval != 24*time.Hour {
			t.Errorf("registrations = %+v", l.registrations)
		}
		if s.Registration == nil || s.Registration.ID != "vMeltype-track" {
			t.Errorf("登録を記録していない: %+v", s.Registration)
		}
		if l.pushAsked != 0 {
			t.Errorf("--push がないのに通知の権限を求めた")
		}
	})
	t.Run("channel、mode、間隔を指定できる", func(t *testing.T) {
		l := tracking(t, "--channel", "prerelease", "--mode", "apply", "--interval", "6h")

		s, _ := l.trackState(t)
		if s.Channel != lifecycle.TrackPrerelease || s.Mode != lifecycle.TrackApply || s.Interval() != 6*time.Hour {
			t.Errorf("settings = %+v", s)
		}
	})
	t.Run("もう一度 enable すると、前の登録を外して登録し直す", func(t *testing.T) {
		l := tracking(t)

		l.run(t, false, "", "track", "enable", "--mode", "download")

		if len(l.registrations) != 2 || len(l.unregistrations) != 1 {
			t.Errorf("registrations = %d, unregistrations = %d", len(l.registrations), len(l.unregistrations))
		}
	})
	for _, tt := range []struct{ name, flag, value string }{
		{"未知の channel", "--channel", "nightly"},
		{"未知の mode", "--mode", "force"},
		{"読めない間隔", "--interval", "毎日"},
		{"1 時間より短い間隔", "--interval", "30m"},
	} {
		t.Run(tt.name+"は引数の誤りにし、登録しない", func(t *testing.T) {
			l := managedStandard(t)

			got := l.run(t, false, "", "track", "enable", tt.flag, tt.value)

			if s, _ := l.trackState(t); got.code != ExitUsage || s.Enabled || len(l.registrations) != 0 {
				t.Errorf("code = %d, enabled = %t, registrations = %d", got.code, s.Enabled, len(l.registrations))
			}
		})
	}
	t.Run("登録できなければ、有効にしない", func(t *testing.T) {
		l := managedStandard(t)
		l.registerFails = true

		got := l.run(t, false, "", "track", "enable")

		if s, _ := l.trackState(t); got.code != ExitError || s.Enabled {
			t.Errorf("code = %d, enabled = %t", got.code, s.Enabled)
		}
	})
}

func TestTrackPushNotifications(t *testing.T) {
	t.Run("--push で、OS に通知の権限を求めてから有効にする", func(t *testing.T) {
		l := tracking(t, "--push")

		if s, _ := l.trackState(t); !s.Push || l.pushAsked != 1 {
			t.Errorf("push = %t, asked = %d", s.Push, l.pushAsked)
		}
	})
	t.Run("権限を断られたら、追従も push 通知も有効にしない", func(t *testing.T) {
		l := managedStandard(t)
		l.pushDenied = true

		got := l.run(t, false, "", "track", "enable", "--push")

		if s, _ := l.trackState(t); got.code != ExitError || s.Enabled || s.Push || len(l.registrations) != 0 {
			t.Errorf("code = %d, settings = %+v, registrations = %d\n%s", got.code, s, len(l.registrations), got.stderr)
		}
	})
	t.Run("有効なら、新しいバージョンを見つけたときに通知する", func(t *testing.T) {
		l := tracking(t, "--push")

		l.run(t, false, "", "track", "run")

		if len(l.pushes) != 1 || !strings.Contains(l.pushes[0], "v1.5.0") {
			t.Errorf("pushes = %v", l.pushes)
		}
	})
	t.Run("無効なら通知しない", func(t *testing.T) {
		l := tracking(t)

		l.run(t, false, "", "track", "run")

		if len(l.pushes) != 0 {
			t.Errorf("pushes = %v", l.pushes)
		}
	})
	t.Run("最新なら通知しない", func(t *testing.T) {
		l := tracking(t, "--push")
		l.upstream.releases = l.upstream.releases[:1]

		l.run(t, false, "", "track", "run")

		if len(l.pushes) != 0 {
			t.Errorf("pushes = %v", l.pushes)
		}
	})
}

func TestTrackRun(t *testing.T) {
	t.Run("notify は見つけたことを記録するだけで、取得も切り替えもしない", func(t *testing.T) {
		l := tracking(t)

		got := l.run(t, false, "", "track", "run")

		_, last := l.trackState(t)
		if got.code != ExitOK || last == nil || last.Found != "v1.5.0" || l.upstream.downloads.Load() != 0 || l.deployed() != "exe v1.4.0" || l.pending(t) != nil {
			t.Errorf("code = %d, last = %+v, downloads = %d\n%s", got.code, last, l.upstream.downloads.Load(), got.stderr)
		}
	})
	t.Run("prerelease ではプレリリースも追う", func(t *testing.T) {
		l := tracking(t, "--channel", "prerelease")

		l.run(t, false, "", "track", "run")

		if _, last := l.trackState(t); last == nil || last.Found != "v1.6.0-pre.1" {
			t.Errorf("last = %+v", last)
		}
	})
	t.Run("download は取得して保持し、適用待ちにする", func(t *testing.T) {
		l := tracking(t, "--mode", "download")

		l.run(t, false, "", "track", "run")

		if p := l.pending(t); p == nil || p.Tag != "v1.5.0" || len(storedIDs(t, l.root)) != 2 || l.deployed() != "exe v1.4.0" {
			t.Errorf("pending = %+v, stored = %v, deployed = %q", p, storedIDs(t, l.root), l.deployed())
		}
	})
	t.Run("apply は条件がそろえば切り替える", func(t *testing.T) {
		l := tracking(t, "--mode", "apply")

		got := l.run(t, false, "", "track", "run")

		if _, last := l.trackState(t); l.deployed() != "exe v1.5.0" || l.pending(t) != nil || last == nil || last.Outcome != "applied" {
			t.Errorf("deployed = %q, pending = %+v, last = %+v\n%s", l.deployed(), l.pending(t), last, got.stderr)
		}
	})
	for _, tt := range []struct {
		name   string
		setup  func(l *localEnv, t *testing.T)
		reason string
	}{
		{name: "固定していれば", setup: func(l *localEnv, t *testing.T) { l.run(t, false, "", "pin") }, reason: "固定"},
		{name: "Meltype が動いていれば", setup: func(l *localEnv, _ *testing.T) { l.running = []string{l.standardAppDir() + `\Meltype.exe`} }, reason: "動いて"},
		{name: "IME の DLL の入れ替えに管理者権限が要るなら", setup: func(l *localEnv, _ *testing.T) {
			l.imeRegistered = true
			l.installedTip = map[string]string{"x64": "hash-x64-v1.4.0", "x86": "hash-x86-v1.4.0"}
		}, reason: "管理者権限"},
	} {
		t.Run("apply でも、"+tt.name+"切り替えずに理由を付けて適用待ちにする", func(t *testing.T) {
			l := tracking(t, "--mode", "apply")
			tt.setup(l, t)

			l.run(t, false, "", "track", "run")

			p := l.pending(t)
			if l.deployed() != "exe v1.4.0" || p == nil || p.Tag != "v1.5.0" || !strings.Contains(p.Reason, tt.reason) || l.stopped != 0 {
				t.Errorf("deployed = %q, pending = %+v, stopped = %d", l.deployed(), p, l.stopped)
			}
		})
	}
	t.Run("通信できなければ、静かに次の回に回す", func(t *testing.T) {
		l := tracking(t)
		l.upstream.server.Close()

		got := l.run(t, false, "", "track", "run")

		if _, last := l.trackState(t); got.code != ExitOK || last == nil || last.Outcome != "deferred" {
			t.Errorf("code = %d, last = %+v", got.code, last)
		}
	})
	t.Run("無効なら何もしない", func(t *testing.T) {
		l := managedStandard(t)

		got := l.run(t, false, "", "track", "run")

		if _, last := l.trackState(t); got.code != ExitOK || last != nil {
			t.Errorf("code = %d, last = %+v", got.code, last)
		}
	})
	t.Run("別の追従の処理が動いていれば、何もせずに終わる", func(t *testing.T) {
		l := tracking(t)
		lock, err := lifecycle.AcquireTrackLock(l.root)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Release()

		got := l.run(t, false, "", "track", "run")

		if _, last := l.trackState(t); got.code != ExitOK || last != nil || l.upstream.apiCalls.Load() != 0 {
			t.Errorf("code = %d, last = %+v", got.code, last)
		}
	})
	t.Run("一覧には出さない", func(t *testing.T) {
		help := execute(t, "track", "--help")
		if strings.Contains(help.stdout, "\n  run ") {
			t.Errorf("一覧に出ている:\n%s", help.stdout)
		}
	})
}

func TestTrackStatusAndDisable(t *testing.T) {
	t.Run("status で設定と最後の結果を示す", func(t *testing.T) {
		l := tracking(t, "--mode", "download", "--push")
		l.run(t, false, "", "track", "run")

		got := l.run(t, false, "", "track", "status")
		asJSON := l.run(t, false, "", "track", "status", "--json")

		for _, want := range []string{"有効", "download", "正式版", "24h", "push 通知", "v1.5.0"} {
			if !strings.Contains(got.stdout, want) {
				t.Errorf("%q がない:\n%s", want, got.stdout)
			}
		}
		var s struct {
			Settings lifecycle.TrackSettings `json:"settings"`
			Last     *lifecycle.TrackRun     `json:"last"`
		}
		if err := json.Unmarshal([]byte(asJSON.stdout), &s); err != nil || !s.Settings.Enabled || s.Last == nil {
			t.Errorf("json = %s, err = %v", asJSON.stdout, err)
		}
	})
	t.Run("見つけた新しいバージョンを、次に status を見たときに知らせる", func(t *testing.T) {
		l := tracking(t)
		l.run(t, false, "", "track", "run")

		got := l.run(t, false, "", "status")

		if !strings.Contains(got.stdout, "追従:") || !strings.Contains(got.stdout, "v1.5.0") {
			t.Errorf("status:\n%s", got.stdout)
		}
	})
	t.Run("disable で登録を外し、無効にする", func(t *testing.T) {
		l := tracking(t, "--push")

		got := l.run(t, false, "", "track", "disable")

		s, _ := l.trackState(t)
		if got.code != ExitOK || s.Enabled || s.Push || s.Registration != nil || len(l.unregistrations) != 1 {
			t.Errorf("code = %d, settings = %+v, unregistrations = %d", got.code, s, len(l.unregistrations))
		}
	})
	t.Run("detach と app uninstall でも登録を外す", func(t *testing.T) {
		detached := tracking(t)
		uninstalled := tracking(t)

		detached.run(t, false, "", "detach", "-y")
		uninstalled.run(t, false, "", "app", "uninstall", "-y")

		if len(detached.unregistrations) != 1 || len(uninstalled.unregistrations) != 1 {
			t.Errorf("unregistrations = %d, %d", len(detached.unregistrations), len(uninstalled.unregistrations))
		}
	})
}

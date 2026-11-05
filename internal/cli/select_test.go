package cli

import (
	"errors"
	"strings"
	"testing"

	"github.com/otnc/vmeltype/internal/prompt"
)

// fakeChooser は、選択画面の代わり。ラベルに pick を含む候補を選び、pick が空なら中止する。
type fakeChooser struct {
	pick    string
	calls   int
	title   string
	options []prompt.Option
}

func (f *fakeChooser) choose(title string, options []prompt.Option) (int, error) {
	f.calls++
	f.title, f.options = title, options
	if f.pick == "" {
		return 0, prompt.ErrCanceled
	}
	for i, o := range options {
		if strings.Contains(o.Label, f.pick) {
			if o.Disabled != "" {
				return 0, errors.New("選べない候補を選んだ")
			}
			return i, nil
		}
	}
	return 0, errors.New("選ぶ候補がない: " + f.pick)
}

func (f *fakeChooser) labels() []string {
	var labels []string
	for _, o := range f.options {
		labels = append(labels, o.Label)
	}
	return labels
}

func (f *fakeChooser) option(t *testing.T, tag string) prompt.Option {
	t.Helper()
	for _, o := range f.options {
		if strings.HasPrefix(o.Label, tag+" ") {
			return o
		}
	}
	t.Fatalf("%s の候補がない: %v", tag, f.labels())
	return prompt.Option{}
}

func selectUpstream(t *testing.T) *fakeUpstream {
	t.Helper()
	data := makeZip(t, map[string]string{"a": "b"})
	return newFakeUpstream(t,
		fakeRelease{tag: "v1.3.0", zip: data, digest: digestOf(data)},
		fakeRelease{tag: "v1.4.0", zip: data, digest: digestOf(data)},
		fakeRelease{tag: "v1.5.0-pre.1", prerelease: true, zip: data, digest: digestOf(data)},
		fakeRelease{tag: "v1.5.0-pre.2", prerelease: true, noAssets: true},
	)
}

func TestInstallWithoutVersionOnTerminalOpensSelection(t *testing.T) {
	t.Run("選んだバージョンを取得する", func(t *testing.T) {
		upstream := selectUpstream(t)
		upstream.chooser = &fakeChooser{pick: "v1.3.0 "}
		root := t.TempDir()

		got := upstream.run(t, root, true, "\n", "install")

		if tags := storedTags(t, root); got.code != ExitOK || len(tags) != 1 || tags[0] != "v1.3.0" {
			t.Errorf("code = %d, tags = %v\n%s%s", got.code, tags, got.stdout, got.stderr)
		}
	})
	t.Run("正式版とプレリリースを新しい順に並べ、区分と公開日を示す", func(t *testing.T) {
		upstream := selectUpstream(t)
		chooser := &fakeChooser{pick: "v1.4.0 "}
		upstream.chooser = chooser

		upstream.run(t, t.TempDir(), true, "\n", "install")

		labels := chooser.labels()
		order := []string{"v1.5.0-pre.2 ", "v1.5.0-pre.1 ", "v1.4.0 ", "v1.3.0 "}
		if len(labels) != len(order) {
			t.Fatalf("候補 = %v", labels)
		}
		for i, prefix := range order {
			if !strings.HasPrefix(labels[i], prefix) {
				t.Errorf("%d 番目 = %q, want %q で始まる", i, labels[i], prefix)
			}
		}
		if o := chooser.option(t, "v1.5.0-pre.1"); !strings.Contains(o.Label, "プレリリース") || !strings.Contains(o.Label, "2026-10-03") {
			t.Errorf("プレリリースの区分か公開日がない: %q", o.Label)
		}
		if o := chooser.option(t, "v1.4.0"); !strings.Contains(o.Label, "正式版") {
			t.Errorf("正式版の区分がない: %q", o.Label)
		}
	})
	t.Run("この PC 向けの配布物がないリリースは、理由を示して選ばせない", func(t *testing.T) {
		upstream := selectUpstream(t)
		chooser := &fakeChooser{pick: "v1.4.0 "}
		upstream.chooser = chooser

		upstream.run(t, t.TempDir(), true, "\n", "install")

		if o := chooser.option(t, "v1.5.0-pre.2"); o.Disabled == "" {
			t.Errorf("配布物のないリリースを選べる: %+v", o)
		}
		if o := chooser.option(t, "v1.4.0"); o.Disabled != "" {
			t.Errorf("配布物のあるリリースを選べない: %+v", o)
		}
	})
	t.Run("保持しているバージョンを示す", func(t *testing.T) {
		upstream := selectUpstream(t)
		root := t.TempDir()
		upstream.run(t, root, false, "", "install", "v1.3.0", "-y")
		chooser := &fakeChooser{pick: "v1.4.0 "}
		upstream.chooser = chooser

		upstream.run(t, root, true, "\n", "install")

		if o := chooser.option(t, "v1.3.0"); !strings.Contains(o.Label, "保持済み") {
			t.Errorf("保持済みを示していない: %q", o.Label)
		}
		if o := chooser.option(t, "v1.4.0"); strings.Contains(o.Label, "保持済み") {
			t.Errorf("保持していないのに保持済みと示した: %q", o.Label)
		}
	})
	t.Run("中止したら何も取得せず、中止の終了コードで終わる", func(t *testing.T) {
		upstream := selectUpstream(t)
		upstream.chooser = &fakeChooser{}
		root := t.TempDir()

		got := upstream.run(t, root, true, "\n", "install")

		if got.code != ExitCanceled || len(storedTags(t, root)) != 0 || upstream.downloads.Load() != 0 {
			t.Errorf("code = %d, tags = %v, downloads = %d", got.code, storedTags(t, root), upstream.downloads.Load())
		}
	})
	t.Run("-y を付けても選択は省かず、選んだあとの確認だけを省く", func(t *testing.T) {
		upstream := selectUpstream(t)
		chooser := &fakeChooser{pick: "v1.3.0 "}
		upstream.chooser = chooser
		root := t.TempDir()

		got := upstream.run(t, root, true, "", "install", "-y")

		if tags := storedTags(t, root); got.code != ExitOK || chooser.calls != 1 || len(tags) != 1 || tags[0] != "v1.3.0" {
			t.Errorf("code = %d, calls = %d, tags = %v\n%s", got.code, chooser.calls, tags, got.stderr)
		}
	})
	for _, tt := range []struct {
		name        string
		interactive bool
		args        []string
		want        string
	}{
		{name: "端末でなければ選ばず、正式版の最新を取得する", args: []string{"install", "-y"}, want: "v1.4.0"},
		{name: "--json では選ばない", interactive: true, args: []string{"install", "-y", "--json"}, want: "v1.4.0"},
		{name: "latest と書けば選ばない", interactive: true, args: []string{"install", "latest", "-y"}, want: "v1.4.0"},
		{name: "--pre を付ければ選ばず、プレリリースの最新を取得する", interactive: true, args: []string{"install", "--pre", "-y"}, want: "v1.5.0-pre.1"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			upstream := newFakeUpstream(t, selectUpstream(t).releases[:3]...)
			chooser := &fakeChooser{pick: "v1.3.0 "}
			upstream.chooser = chooser
			root := t.TempDir()

			got := upstream.run(t, root, tt.interactive, "", tt.args...)

			if tags := storedTags(t, root); got.code != ExitOK || chooser.calls != 0 || len(tags) != 1 || tags[0] != tt.want {
				t.Errorf("code = %d, calls = %d, tags = %v, want %s\n%s", got.code, chooser.calls, tags, tt.want, got.stderr)
			}
		})
	}
}

func TestUseWithoutVersionOnTerminalOpensSelection(t *testing.T) {
	stored := func(t *testing.T) *localEnv {
		t.Helper()
		l := newLocalEnv(t)
		for _, tag := range []string{"v1.3.0", "v1.4.0"} {
			l.storePortable(t, tag, nil)
		}
		return l
	}

	t.Run("保持バージョンから選んで切り替える", func(t *testing.T) {
		l := stored(t)
		l.chooser = &fakeChooser{pick: "v1.3.0 "}

		got := l.run(t, true, "\n", "use", "--portable")

		if got.code != ExitOK || l.read(t, "Meltype.exe") != "exe v1.3.0" {
			t.Errorf("code = %d, Meltype.exe = %q\n%s", got.code, l.read(t, "Meltype.exe"), got.stderr)
		}
	})
	t.Run("--portable の指定に合う保持バージョンだけを並べ、有効なバージョンを示す", func(t *testing.T) {
		l := stored(t)
		l.storeFiles(t, "v1.5.0", "standard", map[string]string{"Meltype.exe": "exe v1.5.0"})
		l.run(t, false, "", "use", "1.4.0", "--portable", "-y")
		chooser := &fakeChooser{pick: "v1.3.0 "}
		l.chooser = chooser

		l.run(t, true, "\n", "use", "--portable")

		labels := chooser.labels()
		if len(labels) != 2 || !strings.HasPrefix(labels[0], "v1.4.0 ") || !strings.HasPrefix(labels[1], "v1.3.0 ") {
			t.Fatalf("候補 = %v", labels)
		}
		if !strings.Contains(labels[0], "有効") || strings.Contains(labels[1], "有効") {
			t.Errorf("有効なバージョンの印が違う: %v", labels)
		}
	})
	t.Run("中止したら切り替えず、中止の終了コードで終わる", func(t *testing.T) {
		l := stored(t)
		l.chooser = &fakeChooser{}

		got := l.run(t, true, "\n", "use", "--portable")

		if got.code != ExitCanceled || l.read(t, "Meltype.exe") != "<なし>" {
			t.Errorf("code = %d, Meltype.exe = %q", got.code, l.read(t, "Meltype.exe"))
		}
	})
	t.Run("合う保持バージョンがなければ、選ばずに install を示す", func(t *testing.T) {
		l := newLocalEnv(t)
		chooser := &fakeChooser{pick: "v1.3.0 "}
		l.chooser = chooser

		got := l.run(t, true, "\n", "use", "--portable")

		if got.code != ExitError || chooser.calls != 0 || !strings.Contains(got.stderr, "vmeltype install") {
			t.Errorf("code = %d, calls = %d, stderr = %s", got.code, chooser.calls, got.stderr)
		}
	})
	t.Run("端末でなければ選ばず、保持している正式版の最新にする", func(t *testing.T) {
		l := stored(t)
		chooser := &fakeChooser{pick: "v1.3.0 "}
		l.chooser = chooser

		got := l.run(t, false, "", "use", "--portable", "-y")

		if got.code != ExitOK || chooser.calls != 0 || l.read(t, "Meltype.exe") != "exe v1.4.0" {
			t.Errorf("code = %d, calls = %d, Meltype.exe = %q", got.code, chooser.calls, l.read(t, "Meltype.exe"))
		}
	})
}

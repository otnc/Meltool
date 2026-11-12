package cli

import (
	"slices"
	"strings"
	"testing"
)

// completions は、シェルの補完が呼ぶ __complete を動かし、候補の語を返す。説明と、最後の指示の行は除く。
func (l *localEnv) completions(t *testing.T, args ...string) []string {
	t.Helper()
	got := l.run(t, false, "", append([]string{"__complete"}, args...)...)
	if got.code != ExitOK {
		t.Fatalf("補完に失敗した: %s", got.stderr)
	}
	var words []string
	for line := range strings.Lines(got.stdout) {
		line = strings.TrimRight(line, "\r\n")
		if strings.HasPrefix(line, ":") {
			break
		}
		word, _, _ := strings.Cut(line, "\t")
		words = append(words, word)
	}
	return words
}

func TestCompletionScript(t *testing.T) {
	for _, shell := range []string{"powershell", "bash", "zsh", "fish"} {
		t.Run(shell+" の補完のスクリプトを出す", func(t *testing.T) {
			got := newLocalEnv(t).run(t, false, "", "completion", shell)

			if got.code != ExitOK || !strings.Contains(got.stdout, "vmeltype") {
				t.Errorf("code = %d, stdout = %q, stderr = %q", got.code, got.stdout, got.stderr)
			}
		})
	}
	t.Run("知らないシェルは引数の誤りにする", func(t *testing.T) {
		got := newLocalEnv(t).run(t, false, "", "completion", "cmd")

		if got.code != ExitUsage {
			t.Errorf("code = %d, stderr = %q", got.code, got.stderr)
		}
	})
}

func TestCompleteStoredVersions(t *testing.T) {
	t.Run("use は保持バージョンのタグを新しい順に出す", func(t *testing.T) {
		l := newLocalEnv(t)
		for _, tag := range []string{"1.3.0", "1.5.0", "1.4.1-pre.6"} {
			l.storeStandard(t, tag)
		}
		l.storePortable(t, "1.2.0", nil)

		got := l.completions(t, "use", "")

		if want := []string{"1.5.0", "1.4.1-pre.6", "1.3.0"}; !slices.Equal(got, want) {
			t.Errorf("候補 = %v, want %v", got, want)
		}
	})
	t.Run("--portable のときは portable 版のタグを出す", func(t *testing.T) {
		l := newLocalEnv(t)
		l.storeStandard(t, "1.5.0")
		l.storePortable(t, "1.2.0", nil)

		got := l.completions(t, "use", "--portable", "")

		if want := []string{"1.2.0"}; !slices.Equal(got, want) {
			t.Errorf("候補 = %v, want %v", got, want)
		}
	})
	t.Run("uninstall は有効なバージョンを出さない", func(t *testing.T) {
		l := newLocalEnv(t)
		l.storeStandard(t, "1.4.0")
		l.storeStandard(t, "1.5.0")
		if got := l.run(t, false, "", "use", "1.5.0", "-y"); got.code != ExitOK {
			t.Fatal(got.stderr)
		}

		got := l.completions(t, "uninstall", "")

		if want := []string{"1.4.0"}; !slices.Equal(got, want) {
			t.Errorf("候補 = %v, want %v", got, want)
		}
	})
	t.Run("補完の途中で標準エラーに何も出さない", func(t *testing.T) {
		// PowerShell の補完は、呼んだコマンドの標準エラーを捨てきれず、入力中の画面に出てしまう
		for _, args := range [][]string{{"__complete", "use", ""}, {"__completeNoDesc", "use", ""}} {
			got := newLocalEnv(t).run(t, false, "", args...)

			if got.stderr != "" {
				t.Errorf("%v: stderr = %q", args, got.stderr)
			}
		}
	})
	t.Run("バージョンを指定したあとは候補を出さない", func(t *testing.T) {
		l := newLocalEnv(t)
		l.storeStandard(t, "1.5.0")

		if got := l.completions(t, "use", "1.5.0", ""); len(got) != 0 {
			t.Errorf("候補 = %v", got)
		}
	})
}

package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/otnc/vmeltype/internal/assets"
	"github.com/otnc/vmeltype/internal/selfinstall"
)

// selfRelease は、vMeltype 自身のリリース。
type selfRelease struct {
	tag    string
	pre    bool
	zip    []byte
	digest string
	// setup は、インストーラー。nil ならリリースに置かない。
	setup       []byte
	setupDigest string
}

// selfUpdateEnv は、vMeltype のリリースを返す偽の GitHub と、置き換えられる vmeltype.exe を用意する。
type selfUpdateEnv struct {
	server  *httptest.Server
	exe     string
	version string
	// installed なら、exe をインストーラーで入れたものとして扱う。RunSetup は、取得した setup.exe の中身を exe に書く。
	installed bool
}

func newSelfUpdateEnv(t *testing.T, version string, releases ...selfRelease) *selfUpdateEnv {
	t.Helper()
	e := &selfUpdateEnv{exe: filepath.Join(t.TempDir(), "vmeltype.exe"), version: version}
	if err := os.WriteFile(e.exe, []byte("vmeltype "+version), 0o700); err != nil {
		t.Fatal(err)
	}
	e.server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/repos/otnc/vMeltype/releases" {
			var body []map[string]any
			for i, rel := range releases {
				version := strings.TrimPrefix(rel.tag, "v")
				assets := []map[string]any{{"id": 10 + i, "name": "vMeltype-" + version + "-windows.zip", "size": len(rel.zip), "digest": rel.digest,
					"browser_download_url": e.server.URL + "/download/" + rel.tag}}
				if rel.setup != nil {
					assets = append(assets, map[string]any{"id": 100 + i, "name": "vMeltype-" + version + "-setup.exe", "size": len(rel.setup), "digest": rel.setupDigest,
						"browser_download_url": e.server.URL + "/download/" + rel.tag + "/setup"})
				}
				body = append(body, map[string]any{
					"id": i + 1, "tag_name": rel.tag, "prerelease": rel.pre,
					"published_at": time.Date(2026, 10, 1+i, 0, 0, 0, 0, time.UTC),
					"assets":       assets,
				})
			}
			json.NewEncoder(w).Encode(body)
			return
		}
		for _, rel := range releases {
			if r.URL.Path == "/download/"+rel.tag {
				w.Write(rel.zip)
				return
			}
			if r.URL.Path == "/download/"+rel.tag+"/setup" {
				w.Write(rel.setup)
				return
			}
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(e.server.Close)
	return e
}

func (e *selfUpdateEnv) run(t *testing.T, args ...string) result {
	t.Helper()
	var stdout, stderr bytes.Buffer
	root := t.TempDir()
	env := Env{
		In: strings.NewReader(""), Out: &stdout, Err: &stderr,
		Version: e.version, Now: time.Now, Getenv: func(string) string { return "" },
		DefaultRoot:  func() (string, error) { return root, nil },
		HTTPClient:   e.server.Client(),
		GitHubAPIURL: e.server.URL,
		Platform:     assets.Platform{OS: "windows", Arch: "amd64"},
		Executable:   func() (string, error) { return e.exe, nil },
		FindInstallation: func() (selfinstall.Installation, bool, error) {
			return selfinstall.Installation{Dir: filepath.Dir(e.exe), Uninstaller: filepath.Join(filepath.Dir(e.exe), "unins000.exe")}, e.installed, nil
		},
		RunSetup: func(_ context.Context, setup string) error {
			data, err := os.ReadFile(setup)
			if err != nil {
				return err
			}
			version, ok := strings.CutPrefix(string(data), "setup ")
			if !ok {
				return errors.New("setup.exe ではない")
			}
			return os.WriteFile(e.exe, []byte("vmeltype "+version), 0o700)
		},
	}
	return result{code: Run(args, env), stdout: stdout.String(), stderr: stderr.String()}
}

func selfZip(t *testing.T, tag string) selfRelease {
	t.Helper()
	zip := makeZip(t, map[string]string{"vmeltype.exe": "vmeltype " + strings.TrimPrefix(tag, "v"), "LICENSE.txt": "license"})
	return selfRelease{tag: tag, pre: strings.Contains(tag, "-"), zip: zip, digest: digestOf(zip)}
}

// selfSetup は、ZIP とインストーラーの両方を置いたリリース。
func selfSetup(t *testing.T, tag string) selfRelease {
	t.Helper()
	rel := selfZip(t, tag)
	rel.setup = []byte("setup " + strings.TrimPrefix(tag, "v"))
	rel.setupDigest = digestOf(rel.setup)
	return rel
}

func TestAppUpdate(t *testing.T) {
	t.Run("正式版の最新に置き換える", func(t *testing.T) {
		for _, args := range [][]string{{"app", "update"}, {"self", "upgrade"}} {
			e := newSelfUpdateEnv(t, "0.1.0-pre.2", selfZip(t, "v0.1.0-pre.2"), selfZip(t, "v0.1.0"), selfZip(t, "v0.2.0-pre.1"))

			got := e.run(t, append(args, "-y")...)

			if got.code != ExitOK || readFile(e.exe) != "vmeltype 0.1.0" {
				t.Errorf("%v: code = %d, exe = %q\n%s", args, got.code, readFile(e.exe), got.stderr)
			}
		}
	})
	t.Run("--pre ではプレリリースも含める", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "0.1.0", selfZip(t, "v0.1.0"), selfZip(t, "v0.2.0-pre.1"))

		got := e.run(t, "app", "update", "--pre", "-y")

		if got.code != ExitOK || readFile(e.exe) != "vmeltype 0.2.0-pre.1" {
			t.Errorf("code = %d, exe = %q\n%s", got.code, readFile(e.exe), got.stderr)
		}
	})
	t.Run("最新なら何もしない", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "0.1.0-pre.2", selfZip(t, "v0.1.0-pre.2"))

		got := e.run(t, "app", "update", "-y")

		if got.code != ExitOK || !strings.Contains(got.stdout, "最新") || readFile(e.exe) != "vmeltype 0.1.0-pre.2" {
			t.Errorf("code = %d\n%s%s", got.code, got.stdout, got.stderr)
		}
	})
	t.Run("プレリリースを使っていて --pre がなければ、--pre を案内する", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "0.1.0-pre.2", selfZip(t, "v0.1.0-pre.2"))

		got := e.run(t, "app", "update", "-y")

		if !strings.Contains(got.stdout, "--pre") {
			t.Errorf("--pre を案内していない: %s", got.stdout)
		}
		if stable := newSelfUpdateEnv(t, "0.1.0", selfZip(t, "v0.1.0")).run(t, "app", "update", "-y"); strings.Contains(stable.stdout, "--pre") {
			t.Errorf("正式版を使っているのに --pre を案内した: %s", stable.stdout)
		}
	})
	t.Run("SHA-256 が合わなければ置き換えない", func(t *testing.T) {
		broken := selfZip(t, "v0.1.0")
		broken.digest = digestOf([]byte("別のもの"))
		e := newSelfUpdateEnv(t, "0.1.0-pre.2", broken)

		got := e.run(t, "app", "update", "-y")

		if got.code != ExitError || !strings.Contains(got.stderr, "SHA-256") || readFile(e.exe) != "vmeltype 0.1.0-pre.2" {
			t.Errorf("code = %d, exe = %q\n%s", got.code, readFile(e.exe), got.stderr)
		}
	})
	t.Run("期待値がなければ置き換えない", func(t *testing.T) {
		unverified := selfZip(t, "v0.1.0")
		unverified.digest = ""
		e := newSelfUpdateEnv(t, "0.1.0-pre.2", unverified)

		got := e.run(t, "app", "update", "-y")

		if got.code != ExitError || readFile(e.exe) != "vmeltype 0.1.0-pre.2" {
			t.Errorf("code = %d, exe = %q\n%s", got.code, readFile(e.exe), got.stderr)
		}
	})
	t.Run("--dry-run と、確認できないときは置き換えない", func(t *testing.T) {
		for _, args := range [][]string{{"app", "update", "--dry-run"}, {"app", "update"}} {
			e := newSelfUpdateEnv(t, "0.1.0-pre.2", selfZip(t, "v0.1.0"))

			got := e.run(t, args...)

			if readFile(e.exe) != "vmeltype 0.1.0-pre.2" || !strings.Contains(got.stdout+got.stderr, "v0.1.0") {
				t.Errorf("%v: code = %d, exe = %q\n%s%s", args, got.code, readFile(e.exe), got.stdout, got.stderr)
			}
		}
	})
	t.Run("インストーラーで入れたものは、setup.exe で入れ直す", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "0.1.0", selfSetup(t, "v0.1.0"), selfSetup(t, "v0.2.0"))
		e.installed = true

		got := e.run(t, "app", "update", "-y")

		if got.code != ExitOK || readFile(e.exe) != "vmeltype 0.2.0" || !strings.Contains(got.stderr, "setup.exe") {
			t.Errorf("code = %d, exe = %q\n%s", got.code, readFile(e.exe), got.stderr)
		}
	})
	t.Run("インストーラーで入れたものは、setup.exe のないリリースを選ばない", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "0.1.0", selfSetup(t, "v0.1.0"), selfZip(t, "v0.2.0"))
		e.installed = true

		got := e.run(t, "app", "update", "-y")

		if got.code != ExitOK || readFile(e.exe) != "vmeltype 0.1.0" || !strings.Contains(got.stdout, "最新") {
			t.Errorf("code = %d, exe = %q\n%s%s", got.code, readFile(e.exe), got.stdout, got.stderr)
		}
	})
	t.Run("setup.exe の SHA-256 が合わなければ動かさない", func(t *testing.T) {
		broken := selfSetup(t, "v0.2.0")
		broken.setupDigest = digestOf([]byte("別のもの"))
		e := newSelfUpdateEnv(t, "0.1.0", broken)
		e.installed = true

		got := e.run(t, "app", "update", "-y")

		if got.code != ExitError || !strings.Contains(got.stderr, "SHA-256") || readFile(e.exe) != "vmeltype 0.1.0" {
			t.Errorf("code = %d, exe = %q\n%s", got.code, readFile(e.exe), got.stderr)
		}
	})
	t.Run("開発中のビルドは更新しない", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "dev", selfZip(t, "v0.1.0"))

		got := e.run(t, "app", "update", "-y")

		if got.code != ExitError || readFile(e.exe) != "vmeltype dev" {
			t.Errorf("code = %d\n%s", got.code, got.stderr)
		}
	})
}

func TestAppCheck(t *testing.T) {
	t.Run("新しい正式版があれば、終了コード 10 で示し、置き換えない", func(t *testing.T) {
		for _, args := range [][]string{{"app", "check"}, {"self", "check"}} {
			e := newSelfUpdateEnv(t, "0.1.0-pre.2", selfZip(t, "v0.1.0-pre.2"), selfZip(t, "v0.1.0"))

			got := e.run(t, args...)

			if got.code != ExitUpdateAvailable || !strings.Contains(got.stdout, "v0.1.0") || !strings.Contains(got.stdout, "app update") || readFile(e.exe) != "vmeltype 0.1.0-pre.2" {
				t.Errorf("%v: code = %d, exe = %q\n%s%s", args, got.code, readFile(e.exe), got.stdout, got.stderr)
			}
		}
	})
	t.Run("--pre ではプレリリースも含め、--json で結果を返す", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "0.1.0", selfZip(t, "v0.1.0"), selfZip(t, "v0.2.0-pre.1"))

		got := e.run(t, "app", "check", "--pre", "--json")

		var body struct {
			Current, Latest string
			UpdateAvailable bool
		}
		json.Unmarshal([]byte(got.stdout), &body)
		if got.code != ExitUpdateAvailable || body.Current != "0.1.0" || body.Latest != "v0.2.0-pre.1" || !body.UpdateAvailable {
			t.Errorf("code = %d, body = %+v\n%s", got.code, body, got.stderr)
		}
	})
	t.Run("最新なら終了コード 0 で、プレリリースを使っていれば --pre を案内する", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "0.1.0-pre.2", selfZip(t, "v0.1.0-pre.2"))

		got := e.run(t, "app", "check")

		if got.code != ExitOK || !strings.Contains(got.stdout, "最新") || !strings.Contains(got.stdout, "--pre") {
			t.Errorf("code = %d\n%s%s", got.code, got.stdout, got.stderr)
		}
	})
}

func TestSelfUpdateCleanup(t *testing.T) {
	t.Run("前の更新で退避した vmeltype.exe.old を、次のコマンドで消す", func(t *testing.T) {
		e := newSelfUpdateEnv(t, "0.1.0", selfZip(t, "v0.1.0"))
		os.WriteFile(e.exe+".old", []byte("vmeltype 0.0.9"), 0o700)

		got := e.run(t, "--version")

		if got.code != ExitOK {
			t.Fatalf("code = %d\n%s", got.code, got.stderr)
		}
		if _, err := os.Stat(e.exe + ".old"); !os.IsNotExist(err) {
			t.Errorf("vmeltype.exe.old が残っている")
		}
		if readFile(e.exe) != "vmeltype 0.1.0" {
			t.Errorf("いまの vmeltype.exe を変えた: %q", readFile(e.exe))
		}
	})
}

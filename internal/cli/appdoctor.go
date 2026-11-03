package cli

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/privatefs"
)

const appDoctorLong = `vMeltype 自身と管理ルートを診断し、直し方を示します。通信せず、何も変えません。

vMeltype のバージョン、入れ方 (インストーラーか ZIP か)、PATH、管理ルートの権限と書き込み、app uninstall でやめられるかを調べます。
Meltype のバージョンや配置の診断は "vmeltype doctor" で行います。要対応の項目があれば、終了コード 3 を返します。`

func runAppDoctor(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		exe := ""
		if env.Executable != nil {
			exe, _ = env.Executable()
		}
		checks := []check{versionCheck(env.Version), installCheck(env, exe), pathCheck(env, exe)}
		checks = append(checks, rootChecks(root)...)
		checks = append(checks, uninstallCheck(root))
		ok := true
		for _, c := range checks {
			if c.Level == levelFail {
				ok = false
			}
		}
		out := cmd.OutOrStdout()
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			if err := writeJSON(out, map[string]any{"root": root, "executable": exe, "ok": ok, "checks": checks}); err != nil {
				return err
			}
		} else {
			writeChecks(out, env.outStyle(), root, checks)
		}
		if !ok {
			return errDoctorFailed
		}
		return nil
	}
}

func versionCheck(version string) check {
	if isDevBuild(version) {
		return check{Name: "version", Level: levelWarn, Message: "開発中のビルドなので、app check と app update でリリースと比べられません", Hint: "リリースの vmeltype.exe を使ってください"}
	}
	return check{Name: "version", Level: levelOK, Message: fmt.Sprintf("vMeltype %s です", version)}
}

// installCheck は、いまの vmeltype.exe の入れ方を示す。app update と app uninstall は、入れ方で動きを変える。
func installCheck(env *Env, exe string) check {
	if exe == "" {
		return check{Name: "install", Level: levelWarn, Message: "vmeltype.exe の場所が分かりません"}
	}
	var inst struct {
		dir        string
		registered bool
	}
	if env.FindInstallation != nil {
		found, ok, err := env.FindInstallation()
		if err != nil {
			return check{Name: "install", Level: levelWarn, Message: err.Error()}
		}
		if ok && found.Holds(exe) {
			return check{Name: "install", Level: levelOK, Message: fmt.Sprintf("インストーラーで入れたものです (%s)。app update はインストーラーで入れ直します", found.Dir)}
		}
		inst.dir, inst.registered = found.Dir, ok
	}
	if inst.registered {
		return check{Name: "install", Level: levelWarn,
			Message: fmt.Sprintf("インストーラーで入れた vMeltype (%s) とは別の vmeltype.exe (%s) を使っています", inst.dir, exe),
			Hint:    "どちらを使うかを決め、使わないほうは「設定 → アプリ」から削除するか、手で消してください"}
	}
	return check{Name: "install", Level: levelOK, Message: fmt.Sprintf("ZIP から置いたものです (%s)。app update は vmeltype.exe を置き換えます", exe)}
}

// pathCheck は、PATH から vmeltype と打ったときに、いまの vmeltype.exe が使われるかを調べる。
func pathCheck(env *Env, exe string) check {
	if exe == "" || env.Getenv == nil {
		return check{Name: "path", Level: levelWarn, Message: "PATH を調べられません"}
	}
	dir := filepath.Dir(exe)
	for _, entry := range filepath.SplitList(env.Getenv("PATH")) {
		if entry == "" {
			continue
		}
		if sameDir(entry, dir) {
			return check{Name: "path", Level: levelOK, Message: fmt.Sprintf("PATH から、この vmeltype.exe を使います (%s)", dir)}
		}
		if _, err := os.Stat(filepath.Join(entry, filepath.Base(exe))); err == nil {
			return check{Name: "path", Level: levelWarn,
				Message: fmt.Sprintf("PATH では、この vmeltype.exe より先に %s の %s が見つかります", entry, filepath.Base(exe)),
				Hint:    "PATH の順を変えるか、使わないほうを消してください"}
		}
	}
	return check{Name: "path", Level: levelWarn, Message: fmt.Sprintf("vmeltype.exe のフォルダー (%s) は PATH にありません", dir),
		Hint: "どのフォルダーからでも vmeltype で使うなら、このフォルダーを PATH に足してください"}
}

func sameDir(a, b string) bool {
	return strings.EqualFold(filepath.Clean(a), filepath.Clean(b))
}

// rootChecks は、管理ルートの権限と書き込みを調べる。管理ルートには Meltype の学習データのスナップショットも置くので、本人だけが使えるようにする。
func rootChecks(root string) []check {
	if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
		return []check{{Name: "root-access", Level: levelOK, Message: "管理ルートはまだありません。初めて変更するときに、本人だけが使える権限で作ります"}}
	}
	access := check{Name: "root-access", Level: levelOK, Message: "管理ルートは本人だけが使えます"}
	if others, err := privatefs.OthersCanAccess(root); err != nil {
		access = check{Name: "root-access", Level: levelWarn, Message: err.Error()}
	} else if others != "" {
		access = check{Name: "root-access", Level: levelFail, Message: "ほかの利用者も管理ルートを使えます",
			Detail: others,
			Hint:   "管理ルートには学習データのスナップショットも置くので、本人 (と SYSTEM、Administrators) だけが使える権限にしてください。Windows なら icacls で権限を確かめられます"}
	}
	write := check{Name: "root-write", Level: levelOK, Message: "管理ルートに書き込めます"}
	// 書き込めるかは、権限の表示からは分からない (読み取り専用のドライブなど) ので、空のファイルを作って消して確かめる
	probe, err := os.CreateTemp(root, ".doctor-*")
	if err != nil {
		write = check{Name: "root-write", Level: levelFail, Message: fmt.Sprintf("管理ルートに書き込めません: %v", err), Hint: "管理ルートの権限か、置いたドライブを確かめてください"}
	} else {
		probe.Close()
		os.Remove(probe.Name())
	}
	return []check{access, write}
}

// uninstallCheck は、app uninstall でやめられるかを調べる。
func uninstallCheck(root string) check {
	journal, err := lifecycle.LoadJournal(root)
	if err != nil {
		return check{Name: "uninstall", Level: levelWarn, Message: fmt.Sprintf("処理記録を読めないので、app uninstall は止まります: %v", err), Hint: "記録のファイルを手で確かめてください"}
	}
	if journal != nil {
		return check{Name: "uninstall", Level: levelWarn, Message: "中断した変更があるので、app uninstall は止まります", Hint: "\"vmeltype recover\" で復旧してください"}
	}
	return check{Name: "uninstall", Level: levelOK, Message: "app uninstall で vMeltype をやめられます"}
}

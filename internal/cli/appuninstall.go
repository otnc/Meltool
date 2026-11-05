package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"

	"github.com/otnc/vmeltype/internal/assets"
	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/prompt"
	"github.com/otnc/vmeltype/internal/selfinstall"
	"github.com/otnc/vmeltype/internal/store"
	"github.com/otnc/vmeltype/internal/upstreamconf"
)

const appUninstallLong = `vMeltype をやめ、管理ルートを消します。

Meltype は既定で残します。有効なバージョンをそのまま使える状態で残し、止めていた本家の自動更新を管理前の値に戻します。
Meltype も削除するなら、対話で答えるか --remove-meltype を付けます。本家のアンインストーラーで削除するので、IME の登録、自動起動、ショートカットも外れます。本家の setup.exe で入れていれば設定と学習データを消すかを本家が聞きますが、Install.cmd で入れたものや vMeltype だけで入れたものは、本家のアンインストーラーが設定と学習データも消します。
スナップショットは既定で消します。残すなら、対話で答えるか --keep-snapshots を付けます。管理ルートの snapshots だけが残ります。
インストーラーで入れたものは、終わったあとに vMeltype のアンインストーラーを画面なしで起動し、vmeltype.exe、PATH、「設定 → アプリ」の登録も外します。
ZIP から入れたものは、いま動いている vmeltype.exe を消せないので、終わったあとに手で消してください。`

func setupAppUninstallFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("remove-meltype", false, "Meltype も本家のアンインストーラーで削除する")
	cmd.Flags().Bool("keep-snapshots", false, "スナップショットを残す")
	// vMeltype のアンインストーラーが、ファイルを消す前に呼ぶ。アンインストーラーを起動し返さず、本家のアンインストーラーは画面ありで動かす
	cmd.Flags().Bool("from-installer", false, "vMeltype のアンインストーラーから呼ばれた")
	cmd.Flags().MarkHidden("from-installer")
}

func runAppUninstall(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		flags := cmd.Flags()
		dryRun, _ := flags.GetBool("dry-run")
		yes, _ := flags.GetBool("yes")
		asJSON, _ := flags.GetBool("json")
		removeMeltype, _ := flags.GetBool("remove-meltype")
		keepSnapshots, _ := flags.GetBool("keep-snapshots")
		fromInstaller, _ := flags.GetBool("from-installer")
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		exe := ""
		if env.Executable != nil {
			exe, _ = env.Executable()
		}
		inst, installed, err := env.installation(exe)
		if err != nil {
			return err
		}
		self := selfRemoval{exe: exe, byInstaller: installed || fromInstaller}
		if installed && !fromInstaller {
			self.uninstaller = &inst
		}
		out := cmd.OutOrStdout()
		if _, err := os.Stat(root); errors.Is(err, os.ErrNotExist) {
			fmt.Fprintf(out, "管理ルート (%s) はないので、消すものはありません。\n", root)
			if dryRun {
				return nil
			}
			if err := startSelfUninstaller(env, self); err != nil {
				return err
			}
			removeSelfFromPath(env, exe, fromInstaller)
			writeSelfRemovalNote(out, self)
			return nil
		}

		lock, err := lifecycle.AcquireLock(root)
		if err != nil {
			return err
		}
		locked := true
		defer func() {
			if locked {
				lock.Release()
			}
		}()
		if journal, err := lifecycle.LoadJournal(root); err != nil {
			return err
		} else if journal != nil {
			return lifecycle.ErrPendingJournal
		}
		state, err := lifecycle.LoadState(root)
		if err != nil {
			return err
		}
		baseline, err := lifecycle.LoadBaseline(root)
		if err != nil {
			return err
		}

		interactive := env.Interactive && !asJSON
		asker := env.confirmer(cmd, interactive, false)
		if !removeMeltype && interactive && !dryRun {
			if removeMeltype, err = asker.Ask("Meltype も削除しますか？", prompt.DefaultNo); err != nil {
				return err
			}
		}
		if !keepSnapshots && interactive && !dryRun {
			removeSnapshots, err := asker.Ask("スナップショットも削除しますか？", prompt.DefaultYes)
			if err != nil {
				return err
			}
			keepSnapshots = !removeSnapshots
		}

		portableDir := lifecycle.PortableAppDir(root)
		hasPortable := exists(portableDir)
		if hasPortable && !removeMeltype {
			return fmt.Errorf("portable 版の Meltype は管理ルートの中 (%s) にあり、管理ルートを消すと一緒に消えます。残すなら別の場所へ写してから実行するか、Meltype も削除するなら --remove-meltype を付けてください", portableDir)
		}
		var standard *layout
		if (state.Active != nil && state.Active.Variant == string(assets.Standard)) || baseline.StandardConfig != nil {
			place, err := layoutFor(env, root, string(assets.Standard))
			if err != nil {
				return err
			}
			standard = &place
		}

		planOut := out
		if asJSON {
			planOut = cmd.ErrOrStderr()
		}
		writeUninstallPlan(planOut, root, self, standard, hasPortable, removeMeltype, keepSnapshots)
		if dryRun {
			if asJSON {
				return writeJSON(out, map[string]any{"dryRun": true, "root": root, "removeMeltype": removeMeltype, "keepSnapshots": keepSnapshots})
			}
			return nil
		}
		confirmer := env.confirmer(cmd, interactive, yes)
		if err := confirmer.Confirm("vMeltype を削除しますか？", prompt.DefaultNo); err != nil {
			return err
		}

		// 書き換えや削除の間に Meltype が設定を保存し直さないよう、止めておく
		var stopped []string
		defer func() {
			for _, dir := range stopped {
				restartApp(cmd, env, dir, nil)
			}
		}()
		for _, dir := range []string{portableDir, standardAppDir(standard)} {
			if dir == "" {
				continue
			}
			running, err := env.RunningUnder(dir)
			if err != nil {
				return fmt.Errorf("Meltype が動いているかを確かめられませんでした: %w", err)
			}
			if len(running) == 0 {
				continue
			}
			if err := env.StopApp(dir); err != nil {
				return err
			}
			if !removeMeltype {
				stopped = append(stopped, dir)
			}
		}

		if standard != nil && removeMeltype && exists(standard.appDir) {
			fallback := ""
			if state.Active != nil && state.Active.Variant == string(assets.Standard) {
				fallback = filepath.Join(store.Store{Root: root}.Path(state.Active.StoreID), "files", "uninstall.ps1")
			}
			// アンインストーラーから呼ばれたときは画面のある削除の途中なので、本家のアンインストーラーにも確認と、設定と学習データを消すかを聞かせる
			if err := env.RemoveMeltype(standard.appDir, fallback, interactive || fromInstaller); err != nil {
				return fmt.Errorf("%w。管理ルートは消していません", err)
			}
		}
		if standard != nil && !removeMeltype && baseline.StandardConfig != nil {
			if err := upstreamconf.RestoreAutoUpdate(filepath.Join(standard.dataDir, "config.json"), *baseline.StandardConfig); err != nil {
				return fmt.Errorf("本家の自動更新を戻せなかったので、管理ルートは消していません: %w", err)
			}
		}

		if _, err := disableTrack(env, root); err != nil {
			return fmt.Errorf("%w。管理ルートは消していません", err)
		}

		// 排他のファイルも管理ルートにあり、開いたままでは消せない
		lock.Release()
		locked = false
		if err := removeRoot(root, keepSnapshots); err != nil {
			return err
		}
		if err := startSelfUninstaller(env, self); err != nil {
			return fmt.Errorf("管理ルートは消しました。%w", err)
		}
		removeSelfFromPath(env, exe, fromInstaller)
		if asJSON {
			return writeJSON(out, map[string]any{"root": root, "removedMeltype": removeMeltype, "keptSnapshots": keepSnapshots, "executable": exe, "uninstallerStarted": self.uninstaller != nil})
		}
		fmt.Fprintln(out, "vMeltype の管理ルートを消しました。")
		writeSelfRemovalNote(out, self)
		return nil
	}
}

// selfRemoval は、管理ルートを消したあとの vmeltype.exe の消し方。
type selfRemoval struct {
	exe string
	// byInstaller なら、vmeltype.exe は vMeltype のアンインストーラーが消す。
	byInstaller bool
	// uninstaller は、管理ルートを消したあとに起動する vMeltype のアンインストーラー。アンインストーラーから呼ばれたときは nil。
	uninstaller *selfinstall.Installation
}

func startSelfUninstaller(env *Env, self selfRemoval) error {
	if self.uninstaller == nil {
		return nil
	}
	if err := env.StartUninstaller(*self.uninstaller); err != nil {
		return fmt.Errorf("%w。「設定 → アプリ」から vMeltype を削除してください", err)
	}
	return nil
}

// removeSelfFromPath は、アンインストーラーから呼ばれたときに、vmeltype.exe のフォルダーを利用者の PATH から外す。
// 管理ルートは片付け終わっているので、外せなくても失敗にはせず、警告する。
func removeSelfFromPath(env *Env, exe string, fromInstaller bool) {
	if !fromInstaller || exe == "" || env.RemoveFromUserPath == nil {
		return
	}
	dir := filepath.Dir(exe)
	if err := env.RemoveFromUserPath(dir); err != nil {
		env.warnf("PATH から %s を外せませんでした。手で外してください: %v", dir, err)
	}
}

func writeSelfRemovalNote(out io.Writer, self selfRemoval) {
	switch {
	case self.uninstaller != nil:
		fmt.Fprintln(out, "vMeltype のアンインストーラーを起動しました。vmeltype.exe、PATH、「設定 → アプリ」の登録を外します。")
	case !self.byInstaller && self.exe != "":
		fmt.Fprintf(out, "vmeltype.exe は動いているので消せません。終わったら %s を手で消してください。\n", self.exe)
	}
}

func standardAppDir(l *layout) string {
	if l == nil {
		return ""
	}
	return l.appDir
}

// removeRoot は、管理ルートを消す。keepSnapshots なら snapshots だけを残す。
func removeRoot(root string, keepSnapshots bool) error {
	if !keepSnapshots {
		return os.RemoveAll(root)
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.Name() == "snapshots" {
			continue
		}
		if err := os.RemoveAll(filepath.Join(root, e.Name())); err != nil {
			return fmt.Errorf("%s を消せませんでした: %w", filepath.Join(root, e.Name()), err)
		}
	}
	return nil
}

func writeUninstallPlan(w io.Writer, root string, self selfRemoval, standard *layout, hasPortable, removeMeltype, keepSnapshots bool) {
	fmt.Fprintf(w, "管理ルート:       %s (消します)\n", root)
	switch {
	case removeMeltype && standard != nil:
		fmt.Fprintf(w, "Meltype:          本家のアンインストーラーで削除します (%s)。IME の登録、自動起動、ショートカットも外れます。\n", standard.appDir)
		fmt.Fprintln(w, "                  Install.cmd や vMeltype だけで入れたものは、設定と学習データも消えます。")
	case standard != nil:
		fmt.Fprintf(w, "Meltype:          いまのバージョンのまま残し (%s)、本家の自動更新を管理前の値に戻します。\n", standard.appDir)
	}
	if hasPortable && removeMeltype {
		fmt.Fprintf(w, "portable 版:      管理ルートの中にあるので、data と一緒に消えます (%s)\n", lifecycle.PortableAppDir(root))
	}
	if keepSnapshots {
		fmt.Fprintf(w, "スナップショット: 残します (%s)\n", filepath.Join(root, "snapshots"))
	} else {
		fmt.Fprintln(w, "スナップショット: 消します")
	}
	switch {
	case self.byInstaller:
		fmt.Fprintf(w, "vmeltype.exe:     vMeltype のアンインストーラーが、PATH と「設定 → アプリ」の登録と一緒に消します (%s)\n", self.exe)
	case self.exe != "":
		fmt.Fprintf(w, "vmeltype.exe:     動いているので消せません。終わったあとに手で消してください (%s)\n", self.exe)
	}
}

func exists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}

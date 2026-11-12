package cli

import (
	"errors"
	"fmt"
	"io"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/otnc/vmeltype/internal/assets"
	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/prompt"
	"github.com/otnc/vmeltype/internal/store"
	"github.com/otnc/vmeltype/internal/upstreamconf"
	"github.com/otnc/vmeltype/internal/winime"
)

const applyLong = `適用待ちの要求があれば適用し、なければ有効なバージョンを、保持したものから修復します。通信しません。

適用待ちの要求は、条件がそろわずに切り替えられなかったバージョンの記録です。apply は、use と同じ処理でそのバージョンに切り替え、終わったら記録を消します。固定 (pin) と違うバージョンなら、--override-pin がなければ切り替えません。--discard で、適用せずに取り消します。

実行先が保持バージョンと違う、本家の自動更新が動いている、登録済みの Meltype IME の DLL が有効なバージョンのものと違う、のどれかなら、"use --force" と同じ処理で置き直します。どれもなければ何もしません。--force を付けると、一致していても置き直します。
--ime を付けると、通常版で Meltype IME を登録していなければ登録します。管理者権限の確認 (UAC) が出ます。日本語のキーボードの一覧の末尾に Meltype を足しますが、既定の入力方式は変えません。前に登録を断っていても登録します。`

func setupApplyFlags(cmd *cobra.Command) {
	cmd.Flags().Bool("ime", false, "通常版で Meltype IME を登録していなければ登録する。管理者権限の確認 (UAC) が出る")
	cmd.Flags().Bool("discard", false, "適用待ちの要求を、適用せずに取り消す")
	addOverridePinFlag(cmd)
	addStopAppFlag(cmd)
}

func runApply(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		flags := cmd.Flags()
		force, _ := flags.GetBool("force")
		ime, _ := flags.GetBool("ime")
		dryRun, _ := flags.GetBool("dry-run")
		asJSON, _ := flags.GetBool("json")
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		if journal, err := lifecycle.LoadJournal(root); err != nil {
			return err
		} else if journal != nil {
			return lifecycle.ErrPendingJournal
		}
		if discard, _ := flags.GetBool("discard"); discard {
			return discardPending(cmd, root, dryRun, asJSON)
		}
		state, err := lifecycle.LoadState(root)
		if err != nil {
			return err
		}
		active := state.Active
		pending, err := lifecycle.LoadPending(root)
		if err != nil {
			return err
		}
		if pending != nil && (active == nil || pending.StoreID != active.StoreID) {
			return applyPending(cmd, env, root, *pending, ime)
		}
		if pending != nil && !dryRun {
			// 適用待ちのバージョンは既に有効なので、記録だけを消し、いつもの修復に進む
			discardAppliedPending(env, root, pending.StoreID)
		}
		if active == nil {
			return errors.New("有効なバージョンがないので、修復するものがありません。\"vmeltype use <tag>\" で有効にしてください")
		}
		standard := active.Variant == string(assets.Standard)
		if ime && !standard {
			return fmt.Errorf("portable 版には Meltype IME を登録できません。通常版を \"vmeltype install %s\" で取得し、\"vmeltype use %s\" で有効にしてから、もう一度実行してください", active.Tag, active.Tag)
		}
		st := store.Store{Root: root}
		target, err := st.Get(active.StoreID)
		if err != nil {
			return fmt.Errorf("有効なバージョン %s を保持していないので、修復できません。\"%s --reinstall\" で取得し直してください: %w", active.Tag, installCommand(active.Tag, active.Variant), err)
		}
		reasons, err := applyReasons(env, root, standard, force)
		if err != nil {
			return err
		}

		out := cmd.OutOrStdout()
		planOut := out
		if asJSON {
			planOut = cmd.ErrOrStderr()
		}
		if len(reasons) > 0 {
			fmt.Fprintf(planOut, "直すこと:     %s\n", strings.Join(reasons, "、"))
			// --json では、use の結果ではなく apply の結果を返す
			if asJSON {
				cmd.SetOut(io.Discard)
			}
			err := switchVersion(cmd, env, root, st, target, true, "%s を置き直しますか？", "%s を置き直しました")
			cmd.SetOut(out)
			if err != nil {
				return err
			}
		}

		var registered winime.Result
		if ime {
			if registered, err = registerIME(cmd, env, filepath.Join(st.Path(target.ID), "files", "app"), target.Tag); err != nil {
				return err
			}
		}
		if asJSON {
			return writeJSON(out, map[string]any{"dryRun": dryRun, "active": target.ID, "reasons": nonNil(reasons), "ime": string(registered)})
		}
		if len(reasons) == 0 && registered == "" {
			fmt.Fprintf(out, "%s は保持したものと一致しているので、直すことはありません。\n", target.Tag)
		}
		return nil
	}
}

// applyReasons は、有効なバージョンを置き直す理由を返す。なければ空を返す。
func applyReasons(env *Env, root string, standard, force bool) ([]string, error) {
	var reasons []string
	if force {
		reasons = append(reasons, "--force が付いている")
	}
	s, err := inspectLocal(env, root)
	if err != nil {
		return nil, err
	}
	if s.Deployment != lifecycle.DeploymentMatch {
		reasons = append(reasons, "実行先が保持バージョンと違う ("+describeDeployment(s.Status)+")")
	}
	if !standard {
		return reasons, nil
	}
	disabled, err := upstreamconf.AutoUpdateDisabled(filepath.Join(s.DataDir, "config.json"))
	if err != nil {
		return nil, err
	}
	if !disabled {
		reasons = append(reasons, "本家の自動更新が動いている")
	}
	if c, ok := imeCheck(env, root, s); ok && c.Level == levelFail {
		reasons = append(reasons, "登録済みの Meltype IME の DLL が有効なバージョンのものと違う")
	}
	return reasons, nil
}

// registerIME は、Meltype IME を登録していなければ、有効なバージョンの資材 (appDir) で登録する。登録済みなら何もせず、空の結果を返す。
func registerIME(cmd *cobra.Command, env *Env, appDir, tag string) (winime.Result, error) {
	flags := cmd.Flags()
	dryRun, _ := flags.GetBool("dry-run")
	yes, _ := flags.GetBool("yes")
	asJSON, _ := flags.GetBool("json")
	out := cmd.OutOrStdout()
	planOut := out
	if asJSON {
		planOut = cmd.ErrOrStderr()
	}
	registered, err := env.IMERegistered()
	if err != nil {
		return "", err
	}
	if registered {
		fmt.Fprintln(planOut, "Meltype IME は登録済みです。")
		return "", nil
	}
	fmt.Fprintf(planOut, "Meltype IME:  %s のものを登録します。管理者権限の確認 (UAC) が出ます。日本語のキーボードの一覧の末尾に Meltype を足しますが、既定の入力方式は変えません。\n", tag)
	if dryRun {
		return "", nil
	}
	confirmer := env.confirmer(cmd, env.Interactive && !asJSON, yes)
	if err := confirmer.Confirm("Meltype IME を登録しますか？", prompt.DefaultYes); err != nil {
		return "", err
	}
	result, err := env.RegisterIME(appDir)
	if err != nil {
		return "", err
	}
	switch result {
	case winime.ResultRegistered:
		if !asJSON {
			fmt.Fprintln(out, "Meltype IME を登録しました。Win + Space で Meltype を選ぶと、入力欄に直接入力できます。")
		}
	case winime.ResultUnlisted:
		env.warnf("Meltype IME を登録しましたが、日本語のキーボードの一覧に足せませんでした。「設定」→「時刻と言語」→「言語と地域」→ 日本語 の「…」→「言語のオプション」→「キーボードの追加」で Meltype を選んでください")
	default:
		return result, imeResultError(result, tag)
	}
	return result, nil
}

// applyPending は、適用待ちのバージョンに、use と同じ処理で切り替える。切り替え終えたら、switchVersion が記録を消す。
func applyPending(cmd *cobra.Command, env *Env, root string, pending lifecycle.Pending, ime bool) error {
	dryRun, _ := cmd.Flags().GetBool("dry-run")
	asJSON, _ := cmd.Flags().GetBool("json")
	st := store.Store{Root: root}
	target, err := st.Get(pending.StoreID)
	if err != nil {
		return fmt.Errorf("適用待ちの %s を保持していないので、適用できません。取得し直すなら \"%s\"、取り消すなら \"vmeltype apply --discard\" を実行してください: %w", pending.Tag, installCommand(pending.Tag, pending.Variant), err)
	}
	standard := target.Variant == string(assets.Standard)
	if ime && !standard {
		return fmt.Errorf("適用待ちの %s は portable 版なので、Meltype IME を登録できません。--ime を外して実行してください", target.Tag)
	}
	out := cmd.OutOrStdout()
	planOut := out
	if asJSON {
		planOut = cmd.ErrOrStderr()
		cmd.SetOut(io.Discard)
	}
	fmt.Fprintf(planOut, "適用待ち:     %s\n", describePending(&pending))
	err = switchVersion(cmd, env, root, st, target, false, "%s を適用しますか？", "%s を適用しました")
	cmd.SetOut(out)
	if err != nil {
		return err
	}
	var registered winime.Result
	if ime {
		if registered, err = registerIME(cmd, env, filepath.Join(st.Path(target.ID), "files", "app"), target.Tag); err != nil {
			return err
		}
	}
	if asJSON {
		return writeJSON(out, map[string]any{"dryRun": dryRun, "active": target.ID, "applied": pending, "ime": string(registered)})
	}
	return nil
}

func discardPending(cmd *cobra.Command, root string, dryRun, asJSON bool) error {
	lock, err := lifecycle.AcquireLockIfManaged(root)
	if err != nil {
		return err
	}
	var pending *lifecycle.Pending
	if lock != nil {
		defer lock.Release()
		if pending, err = lifecycle.LoadPending(root); err != nil {
			return err
		}
	}
	out := cmd.OutOrStdout()
	if pending != nil && !dryRun {
		if _, err := lifecycle.DiscardPending(root); err != nil {
			return err
		}
	}
	if asJSON {
		return writeJSON(out, map[string]any{"dryRun": dryRun, "discarded": pending})
	}
	switch {
	case pending == nil:
		fmt.Fprintln(out, "適用待ちの要求はありません。")
	case dryRun:
		fmt.Fprintf(out, "適用待ちの %s を取り消します。\n", describePending(pending))
	default:
		fmt.Fprintf(out, "適用待ちの %s を取り消しました。有効なバージョンは変えていません。\n", describePending(pending))
	}
	return nil
}

// discardAppliedPending は、storeID に切り替え終えたときに、そのバージョンの適用待ちを消す。切り替えは終わっているので、消せなくても警告だけにする。
func discardAppliedPending(env *Env, root, storeID string) {
	if err := lifecycle.DiscardPendingFor(root, storeID); err != nil {
		env.warnf("適用待ちの記録を消せませんでした。\"vmeltype apply --discard\" で取り消してください: %v", err)
	}
}

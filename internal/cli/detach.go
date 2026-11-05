package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/otnc/vmeltype/internal/assets"
	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/prompt"
)

const detachLong = `いまの Meltype を、vMeltype に頼らずに動く状態で残し、管理をやめます。vMeltype 自身、保持バージョン、スナップショットは残します。通信しません。

通常版は、実行先 (%LOCALAPPDATA%\Programs\Meltype) とデータ先をそのまま残し、本家の自動更新を管理前の値に戻します。
portable 版の実行先は管理ルートの中にあるので、--to で管理ルートの外の空のフォルダーを指定します。app と data をそこへ写し、中身が一致することを確かめてから、管理ルートの中のものを消します。
有効なバージョン、直前のバージョン、固定、管理前の記録を外します。あとで use すれば、また管理を始められます。

実行先の Meltype が動いていれば、use と同じ条件で正常に終了させ、終わったら起動し直します。`

func setupDetachFlags(cmd *cobra.Command) {
	cmd.Flags().String("to", "", "portable 版の app と data を写す先。管理ルートの外の、ないか空のフォルダー")
	addStopAppFlag(cmd)
}

func runDetach(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		flags := cmd.Flags()
		dryRun, _ := flags.GetBool("dry-run")
		yes, _ := flags.GetBool("yes")
		asJSON, _ := flags.GetBool("json")
		to, _ := flags.GetString("to")
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		state, err := lifecycle.LoadState(root)
		if err != nil {
			return err
		}
		if state.Active == nil {
			return errors.New("有効なバージョンがないので、やめる管理がありません")
		}
		place, err := activeLayout(env, root)
		if err != nil {
			return err
		}
		portable := state.Active.Variant == string(assets.Portable)
		dest := ""
		switch {
		case portable && to == "":
			return usageError{fmt.Errorf("portable 版の実行先は管理ルートの中 (%s) にあるので、--to で管理ルートの外の写す先を指定してください", place.appDir)}
		case portable:
			if dest, err = detachDestination(root, to); err != nil {
				return err
			}
		case to != "":
			return usageError{errors.New("--to は portable 版のときだけ使います。通常版の実行先はそのまま残します")}
		}

		app, err := inspectApp(cmd, env, root, place.appDir)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		planOut := out
		if asJSON {
			planOut = cmd.ErrOrStderr()
		}
		writeDetachPlan(planOut, state.Active, place, dest)
		app.describe(planOut)
		if dryRun {
			if asJSON {
				return writeJSON(out, map[string]any{"dryRun": true, "tag": state.Active.Tag, "variant": state.Active.Variant, "appDir": place.appDir, "to": dest})
			}
			return nil
		}
		interactive := env.Interactive && !asJSON
		if err := app.confirmable(interactive); err != nil {
			return err
		}
		confirmer := env.confirmer(cmd, interactive, app.assumeYes(yes))
		if err := confirmer.Confirm(app.question("管理をやめますか？"), prompt.DefaultNo); err != nil {
			return err
		}

		lock, err := lockForDataChange(root)
		if err != nil {
			return err
		}
		defer lock.Release()
		stopped, err := app.stop(env)
		restartDir := place.appDir
		if stopped {
			defer func() { restartApp(cmd, env, restartDir, err) }()
		}
		if err != nil {
			return err
		}
		if portable {
			if err = lifecycle.CopyPortable(root, dest); err != nil {
				return fmt.Errorf("%w。管理はやめていません", err)
			}
			if err = lifecycle.Detach(root, ""); err != nil {
				os.RemoveAll(dest)
				return err
			}
			restartDir = dest
			if removeErr := os.RemoveAll(place.appDir); removeErr != nil {
				env.warnf("管理ルートの中の実行先 (%s) を消せませんでした。手で消してください: %v", place.appDir, removeErr)
			}
		} else if err = lifecycle.Detach(root, place.dataDir); err != nil {
			return err
		}

		if asJSON {
			return writeJSON(out, map[string]any{"tag": state.Active.Tag, "variant": state.Active.Variant, "appDir": restartDir})
		}
		fmt.Fprintf(out, "管理をやめました。Meltype %s は %s にあります。\n", state.Active.Tag, restartDir)
		return nil
	}
}

// detachDestination は、portable 版を写す先を確かめて、絶対パスにする。ないか空のフォルダーで、管理ルートと重ならないものだけを受け付ける。
func detachDestination(root, to string) (string, error) {
	dest, err := filepath.Abs(to)
	if err != nil {
		return "", err
	}
	absRoot, err := filepath.Abs(root)
	if err != nil {
		return "", err
	}
	if within(dest, absRoot) || within(absRoot, dest) {
		return "", fmt.Errorf("--to の先 (%s) が管理ルート (%s) と重なっています。管理ルートの外を指定してください", dest, absRoot)
	}
	entries, err := os.ReadDir(dest)
	switch {
	case errors.Is(err, os.ErrNotExist):
		return dest, nil
	case err != nil:
		return "", fmt.Errorf("--to の先 (%s) を確かめられませんでした: %w", dest, err)
	case len(entries) > 0:
		return "", fmt.Errorf("--to の先 (%s) が空ではありません。ないか空のフォルダーを指定してください", dest)
	}
	return dest, nil
}

// within は、path が dir 自身か、その下にあるかを返す。
func within(path, dir string) bool {
	rel, err := filepath.Rel(dir, path)
	return err == nil && rel != ".." && !strings.HasPrefix(rel, ".."+string(filepath.Separator))
}

func writeDetachPlan(w io.Writer, active *lifecycle.Activation, place layout, dest string) {
	fmt.Fprintf(w, "やめる管理:   %s (%s)\n", active.Tag, variantLabels[active.Variant])
	if dest != "" {
		fmt.Fprintf(w, "写す先:       %s (app と data を写して確かめてから、%s を消します)\n", dest, place.appDir)
	} else {
		fmt.Fprintf(w, "実行先:       %s (そのまま残します)\n", place.appDir)
		fmt.Fprintf(w, "データ先:     %s (そのまま残します)\n", place.dataDir)
		fmt.Fprintln(w, "本家の自動更新: 管理前の値に戻します")
	}
	fmt.Fprintln(w, "保持バージョンとスナップショットは残します。")
}

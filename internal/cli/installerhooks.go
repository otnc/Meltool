package cli

import (
	"errors"
	"fmt"
	"path/filepath"

	"github.com/spf13/cobra"
)

// runAppAddToPath は、vMeltype のインストーラーが、入れたあとに呼ぶ。PATH の扱いを Go に置き、インストーラーの Pascal Script を増やさないため。
// 外すほうは、アンインストーラーが呼ぶ app uninstall --from-installer が行う。
func runAppAddToPath(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		if env.Executable == nil || env.AddToUserPath == nil {
			return errors.New("この環境では PATH に足せません")
		}
		exe, err := env.Executable()
		if err != nil {
			return err
		}
		dir := filepath.Dir(exe)
		if err := env.AddToUserPath(dir); err != nil {
			return err
		}
		fmt.Fprintf(cmd.OutOrStdout(), "%s を PATH に足しました。\n", dir)
		return nil
	}
}

// hideCommand は、利用者が直接使わないコマンドを、ヘルプの一覧から隠す。
func hideCommand(cmd *cobra.Command) {
	cmd.Hidden = true
}

package cli

import (
	"fmt"
	"slices"

	"github.com/spf13/cobra"

	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/store"
	"github.com/otnc/vmeltype/internal/versions"
)

const completionLong = `シェルの補完のスクリプトを標準出力に出します。通信しません。use と uninstall のバージョンは、保持バージョンから補います。

入れ方:
  PowerShell  vmeltype completion powershell | Out-String | Invoke-Expression を $PROFILE に書く
  bash        source <(vmeltype completion bash) を ~/.bashrc に書く
  zsh         source <(vmeltype completion zsh) を ~/.zshrc の compinit より後に書く
  fish        vmeltype completion fish | source を ~/.config/fish/config.fish に書く`

var completionShells = []string{"powershell", "bash", "zsh", "fish"}

func completionArgs(cmd *cobra.Command, args []string) error {
	if err := exactArgs(1)(cmd, args); err != nil {
		return err
	}
	if !slices.Contains(completionShells, args[0]) {
		return usageError{fmt.Errorf("補完を出せるシェルは %v です: %q", completionShells, args[0])}
	}
	return nil
}

func runCompletion(cmd *cobra.Command, args []string) error {
	root, out := cmd.Root(), cmd.OutOrStdout()
	switch args[0] {
	case "powershell":
		return root.GenPowerShellCompletionWithDesc(out)
	case "bash":
		return root.GenBashCompletionV2(out, true)
	case "zsh":
		return root.GenZshCompletion(out)
	default:
		return root.GenFishCompletion(out, true)
	}
}

func completeShells(_ *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
	if len(args) > 0 {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	return completionShells, cobra.ShellCompDirectiveNoFileComp
}

// completeStoredTags は、--portable の指定に合う保持バージョンのタグを、新しい順に補う。管理ルートを読むだけで、通信も変更もしない。
// withoutActive なら、有効なバージョンを除く (uninstall は有効なバージョンを消せない)。
func completeStoredTags(env *Env, withoutActive bool) cobra.CompletionFunc {
	return func(cmd *cobra.Command, args []string, _ string) ([]cobra.Completion, cobra.ShellCompDirective) {
		const directive = cobra.ShellCompDirectiveNoFileComp | cobra.ShellCompDirectiveKeepOrder
		if len(args) > 0 {
			return nil, directive
		}
		root, err := managementRoot(cmd, env)
		if err != nil {
			return nil, directive
		}
		stored, _, err := store.Store{Root: root}.List()
		if err != nil {
			return nil, directive
		}
		activeID := ""
		if state, err := lifecycle.LoadState(root); withoutActive && err == nil && state.Active != nil {
			activeID = state.Active.StoreID
		}

		variant := string(selectedVariant(cmd))
		var candidates []store.Stored
		for _, s := range stored {
			if s.OS == env.Platform.OS && s.Arch == env.Platform.Arch && s.Variant == variant && s.ID != activeID {
				candidates = append(candidates, s)
			}
		}
		slices.SortFunc(candidates, func(a, b store.Stored) int { return versions.Compare(b.Tag, a.Tag) })
		completions := make([]cobra.Completion, 0, len(candidates))
		for _, s := range candidates {
			kind := "正式版"
			if s.Prerelease {
				kind = "プレリリース"
			}
			completions = append(completions, cobra.CompletionWithDesc(s.Tag, kind))
		}
		return completions, directive
	}
}

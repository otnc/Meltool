package cli

import (
	"fmt"
	"io"
	"os"
	"strings"

	"github.com/spf13/cobra"

	"github.com/otnc/vmeltype/internal/lifecycle"
)

// localStatus は、status と doctor が使う、管理ルートと実行先のいまの状態。
type localStatus struct {
	Root string `json:"root"`
	lifecycle.Status
	Running    []string `json:"running"`
	DataDir    string   `json:"dataDir"`
	DataExists bool     `json:"dataExists"`
	// PendingRequest は、適用待ちの要求。lifecycle.Status の Pending (中断した処理記録) とは別のもの。
	PendingRequest *lifecycle.Pending `json:"pendingRequest,omitempty"`
	// TrackNotice は、追従の処理が見つけた、まだ有効にしていない新しいバージョンの知らせ。
	TrackNotice string `json:"trackNotice,omitempty"`
}

func inspectLocal(env *Env, root string) (localStatus, error) {
	status, err := lifecycle.Inspect(root)
	if err != nil {
		return localStatus{}, err
	}
	place, err := activeLayout(env, root)
	if err != nil {
		return localStatus{}, err
	}
	s := localStatus{Root: root, Status: status, Running: []string{}, DataDir: place.dataDir}
	running, err := env.RunningUnder(place.appDir)
	if err != nil {
		return localStatus{}, fmt.Errorf("実行先の Meltype が動いているかを確かめられませんでした: %w", err)
	}
	if running != nil {
		s.Running = running
	}
	if info, err := os.Stat(s.DataDir); err == nil && info.IsDir() {
		s.DataExists = true
	}
	if s.PendingRequest, err = lifecycle.LoadPending(root); err != nil {
		return localStatus{}, err
	}
	if s.Active != nil && s.PendingRequest == nil {
		s.TrackNotice = trackNotice(root, s.Active.Tag)
	}
	return s, nil
}

func runStatus(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		s, err := inspectLocal(env, root)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if asJSON, _ := cmd.Flags().GetBool("json"); asJSON {
			return writeJSON(out, s)
		}
		writeStatus(out, s)
		return nil
	}
}

func writeStatus(w io.Writer, s localStatus) {
	fmt.Fprintf(w, "管理ルート:       %s\n", s.Root)
	if s.Pending != nil {
		fmt.Fprintf(w, "未完了の処理:     %s。\"vmeltype recover\" で復旧してください\n", describeJournal(s.Pending))
	}
	if s.PendingRequest != nil {
		fmt.Fprintf(w, "適用待ち:         %s。\"vmeltype apply\" で適用します\n", describePending(s.PendingRequest))
	}
	if s.TrackNotice != "" {
		fmt.Fprintf(w, "追従:             %s\n", s.TrackNotice)
	}
	if s.Active == nil {
		fmt.Fprintln(w, "有効なバージョン: 有効なバージョンはありません。\"vmeltype use <tag>\" で有効にします")
		return
	}
	fmt.Fprintf(w, "有効なバージョン: %s (%s、保持 ID: %s)\n", s.Active.Tag, variantLabels[s.Active.Variant], s.Active.StoreID)
	previous := "なし"
	if s.Previous != nil {
		previous = s.Previous.Tag
	}
	fmt.Fprintf(w, "直前のバージョン: %s\n", previous)
	pin := "なし"
	if s.Pin != nil {
		pin = s.Pin.Tag + " に固定しています"
	}
	fmt.Fprintf(w, "固定:             %s\n", pin)
	fmt.Fprintf(w, "実行先:           %s\n", s.Active.AppDir)
	fmt.Fprintf(w, "配置:             %s\n", describeDeployment(s.Status))
	running := "止まっています"
	if len(s.Running) > 0 {
		running = "動いています (" + strings.Join(s.Running, "、") + ")"
	}
	fmt.Fprintf(w, "稼働:             %s\n", running)
	data := "まだありません"
	if s.DataExists {
		data = "あります"
	}
	fmt.Fprintf(w, "データ先:         %s (%s)\n", s.DataDir, data)
}

func describePending(p *lifecycle.Pending) string {
	return fmt.Sprintf("%s (%s、%s に記録。理由: %s)", p.Tag, variantLabels[p.Variant], p.CreatedAt.Local().Format("2006-01-02 15:04"), p.Reason)
}

func describeJournal(j *lifecycle.Journal) string {
	from := "なし"
	if j.From != nil {
		from = j.From.Tag
	}
	return fmt.Sprintf("%s から %s への切り替えが中断しています", from, j.To.Tag)
}

func describeDeployment(s lifecycle.Status) string {
	switch s.Deployment {
	case lifecycle.DeploymentMatch:
		return "保持バージョンと一致しています"
	case lifecycle.DeploymentMismatch:
		return fmt.Sprintf("保持バージョンと一致しません (%s)。\"%s\" で置き直せます", summarize(s.Mismatched), reapplyCommand(s.Active))
	case lifecycle.DeploymentPending:
		return "未完了の処理があるので調べていません"
	default:
		return fmt.Sprintf("保持バージョン %s を読めないので比べられません", s.Active.StoreID)
	}
}

// summarize は、長い一覧を、先頭のいくつかと残りの件数にまとめる。
func summarize(names []string) string {
	const shown = 3
	if len(names) <= shown {
		return strings.Join(names, "、")
	}
	return fmt.Sprintf("%s ほか %d 件", strings.Join(names[:shown], "、"), len(names)-shown)
}

// writeLocalStatus は、引数なしの vmeltype で、ヘルプの前に status と同じ内容を表示する。
func writeLocalStatus(cmd *cobra.Command, env *Env) error {
	root, err := managementRoot(cmd, env)
	if err != nil {
		return err
	}
	s, err := inspectLocal(env, root)
	if err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	writeStatus(out, s)
	fmt.Fprintln(out)
	return nil
}

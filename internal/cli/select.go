package cli

import (
	"sort"
	"strings"

	"github.com/mattn/go-runewidth"

	"github.com/otnc/vmeltype/internal/assets"
	"github.com/otnc/vmeltype/internal/catalog"
	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/prompt"
	"github.com/otnc/vmeltype/internal/store"
	"github.com/otnc/vmeltype/internal/versions"
)

const (
	chooseInstallTitle = "取得するバージョンを選んでください"
	chooseUseTitle     = "切り替えるバージョンを選んでください"
)

// chooseRelease は、公式一覧のリリースを新しい順に並べて選ばせる。この PC 向けの配布物がないリリースは、理由を示して選ばせない。
func chooseRelease(choose prompt.Choose, releases []catalog.Release, st store.Store, root string, platform assets.Platform, variant assets.Variant) (catalog.Release, error) {
	sorted := append([]catalog.Release(nil), releases...)
	sort.SliceStable(sorted, func(i, j int) bool { return versions.Compare(sorted[i].Tag, sorted[j].Tag) > 0 })
	active := activeTagOf(root, variant)

	rows := make([][]string, len(sorted))
	options := make([]prompt.Option, len(sorted))
	for i, r := range sorted {
		var marks []string
		if _, err := st.Get(store.ID(r.Tag, platform.OS, platform.Arch, string(variant))); err == nil {
			marks = append(marks, "保持済み")
		}
		if r.Tag == active {
			marks = append(marks, "有効")
		}
		published := ""
		if !r.PublishedAt.IsZero() {
			published = r.PublishedAt.UTC().Format("2006-01-02")
		}
		rows[i] = []string{r.Tag, releaseKind(r.Prerelease), published, strings.Join(marks, "、")}
		if _, err := assets.Select(r, platform, variant); err != nil {
			options[i].Disabled = err.Error()
		}
	}
	for i, label := range alignColumns(rows) {
		options[i].Label = label
	}
	chosen, err := choose(chooseInstallTitle, options)
	if err != nil {
		return catalog.Release{}, err
	}
	return sorted[chosen], nil
}

// chooseStored は、この PC と variant に合う保持バージョンを新しい順に並べて選ばせる。保持バージョンの manifest だけを使い、通信しない。
// 合うものがなければ、選ばずに resolveStored と同じ理由を返す。
func chooseStored(choose prompt.Choose, st store.Store, root string, q storedQuery, platform assets.Platform) (store.Stored, error) {
	list, _, err := st.List()
	if err != nil {
		return store.Stored{}, err
	}
	var candidates []store.Stored
	for _, s := range list {
		if s.OS == platform.OS && s.Arch == platform.Arch && s.Variant == string(q.variant) {
			candidates = append(candidates, s)
		}
	}
	if len(candidates) == 0 {
		return resolveStored(st, q, platform)
	}
	sort.SliceStable(candidates, func(i, j int) bool { return versions.Compare(candidates[i].Tag, candidates[j].Tag) > 0 })
	active := activeTagOf(root, q.variant)

	rows := make([][]string, len(candidates))
	for i, s := range candidates {
		mark := ""
		if s.Tag == active {
			mark = "有効"
		}
		rows[i] = []string{s.Tag, releaseKind(s.Prerelease), mark}
	}
	options := make([]prompt.Option, len(candidates))
	for i, label := range alignColumns(rows) {
		options[i].Label = label
	}
	chosen, err := choose(chooseUseTitle, options)
	if err != nil {
		return store.Stored{}, err
	}
	return candidates[chosen], nil
}

// activeTagOf は、variant の有効なバージョンのタグを返す。候補の印に使うだけなので、記録を読めなければ空にする。
func activeTagOf(root string, variant assets.Variant) string {
	state, err := lifecycle.LoadState(root)
	if err != nil || state.Active == nil || state.Active.Variant != string(variant) {
		return ""
	}
	return state.Active.Tag
}

func releaseKind(prerelease bool) string {
	if prerelease {
		return "プレリリース"
	}
	return "正式版"
}

// alignColumns は、列を表示の幅でそろえた行を返す。選択画面の候補を、表のように読めるようにする。
func alignColumns(rows [][]string) []string {
	var widths []int
	for _, row := range rows {
		for i, cell := range row {
			if i == len(widths) {
				widths = append(widths, 0)
			}
			widths[i] = max(widths[i], runewidth.StringWidth(cell))
		}
	}
	lines := make([]string, len(rows))
	for n, row := range rows {
		var line strings.Builder
		for i, cell := range row {
			line.WriteString(cell)
			if i < len(row)-1 {
				line.WriteString(strings.Repeat(" ", widths[i]-runewidth.StringWidth(cell)+2))
			}
		}
		lines[n] = strings.TrimRight(line.String(), " ")
	}
	return lines
}

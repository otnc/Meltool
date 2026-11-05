// Package choose は、端末で候補を 1 つ選ばせる画面を、Huh (charm.land/huh/v2) で作る。cli は prompt.Choose として受け取るので、Huh に依存しない。
package choose

import (
	"errors"
	"io"

	"charm.land/bubbles/v2/key"
	"charm.land/huh/v2"

	"github.com/otnc/vmeltype/internal/prompt"
)

const help = "↑↓ で移動、/ で絞り込み、Enter で決定、Esc で中止"

// New は、in から読み、out に描く選択画面を返す。in と out は端末にする。
// 結果を出す標準出力を汚さないよう、out には標準エラーを渡す。
//
//	env.Choose = choose.New(os.Stdin, os.Stderr)
func New(in io.Reader, out io.Writer) prompt.Choose {
	return func(title string, options []prompt.Option) (int, error) {
		huhOptions := make([]huh.Option[int], len(options))
		for i, o := range options {
			label := o.Label
			if o.Disabled != "" {
				label += " (選べません)"
			}
			huhOptions[i] = huh.NewOption(label, i)
		}
		chosen := firstEnabled(options)
		field := huh.NewSelect[int]().
			Title(title).
			Description(help).
			Options(huhOptions...).
			Value(&chosen).
			Height(min(len(options), 15) + 2).
			Validate(func(i int) error {
				if reason := options[i].Disabled; reason != "" {
					return errors.New(reason)
				}
				return nil
			})
		err := huh.NewForm(huh.NewGroup(field)).
			WithInput(in).
			WithOutput(out).
			WithShowHelp(false).
			WithKeyMap(keyMap()).
			Run()
		if errors.Is(err, huh.ErrUserAborted) {
			return 0, prompt.ErrCanceled
		}
		return chosen, err
	}
}

// keyMap は、y/n の確認と同じく、Esc、Ctrl+C、Ctrl+D、Ctrl+Z で中止できるようにする。
// Huh の既定では、中止は Ctrl+C だけで、Esc は絞り込みの確定と解除、Ctrl+D は半ページ送りに使う。
// 絞り込みは Enter でそのまま選ぶことにして、これらのキーを中止に回す。
func keyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	km.Quit = key.NewBinding(key.WithKeys("ctrl+c", "esc", "ctrl+d", "ctrl+z"))
	km.Select.HalfPageDown = key.NewBinding(key.WithKeys("pgdown"))
	km.Select.HalfPageUp = key.NewBinding(key.WithKeys("pgup"))
	km.Select.SetFilter = key.NewBinding(key.WithDisabled())
	km.Select.ClearFilter = key.NewBinding(key.WithDisabled())
	return km
}

func firstEnabled(options []prompt.Option) int {
	for i, o := range options {
		if o.Disabled == "" {
			return i
		}
	}
	return 0
}

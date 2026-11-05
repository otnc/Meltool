package choose

import (
	"errors"
	"io"
	"testing"
	"time"

	"github.com/otnc/vmeltype/internal/prompt"
)

const (
	keyCtrlD = "\x04"
	keyCtrlZ = "\x1a"
	keyDown  = "\x1b[B"
	keyEnter = "\r"
	keyEsc   = "\x1b"
	keyCtrlC = "\x03"
)

var testOptions = []prompt.Option{{Label: "v1.5.0"}, {Label: "v1.4.1-pre.1", Disabled: "配布物がありません"}, {Label: "v1.4.0"}}

// keys は、キーを 1 つずつ間を空けて渡す。まとめて渡すと、Esc が続くキーと 1 つの並びに読まれるため。
type keys struct {
	parts []string
}

func (k *keys) Read(p []byte) (int, error) {
	if len(k.parts) == 0 {
		time.Sleep(10 * time.Millisecond)
		return 0, io.EOF
	}
	time.Sleep(50 * time.Millisecond)
	n := copy(p, k.parts[0])
	k.parts = k.parts[1:]
	return n, nil
}

func run(t *testing.T, input ...string) (int, error) {
	t.Helper()
	done := make(chan struct{})
	var chosen int
	var err error
	go func() {
		defer close(done)
		chosen, err = New(&keys{parts: input}, io.Discard)("選んでください", testOptions)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("選択画面が終わらない")
	}
	return chosen, err
}

func TestChoose(t *testing.T) {
	t.Run("矢印キーで動かし、Enter で選ぶ", func(t *testing.T) {
		got, err := run(t, keyDown, keyDown, keyEnter)
		if err != nil || got != 2 {
			t.Errorf("got = %d, err = %v", got, err)
		}
	})
	t.Run("選べない候補では Enter を受け付けない", func(t *testing.T) {
		got, err := run(t, keyDown, keyEnter, keyDown, keyEnter)
		if err != nil || got != 2 {
			t.Errorf("got = %d, err = %v", got, err)
		}
	})
	t.Run("文字で絞り込んで選ぶ", func(t *testing.T) {
		got, err := run(t, "/", "1", ".", "4", ".", "0", keyEnter)
		if err != nil || got != 2 {
			t.Errorf("got = %d, err = %v", got, err)
		}
	})
	for _, tt := range []struct{ name, key string }{{"Esc", keyEsc}, {"Ctrl+C", keyCtrlC}, {"Ctrl+D", keyCtrlD}, {"Ctrl+Z", keyCtrlZ}} {
		t.Run(tt.name+" で中止する", func(t *testing.T) {
			_, err := run(t, tt.key)
			if !errors.Is(err, prompt.ErrCanceled) {
				t.Errorf("err = %v", err)
			}
		})
	}
	t.Run("最初は選べる候補にいる", func(t *testing.T) {
		options := []prompt.Option{{Label: "a", Disabled: "x"}, {Label: "b"}}
		got, err := New(&keys{parts: []string{keyEnter}}, io.Discard)("選んでください", options)
		if err != nil || got != 1 {
			t.Errorf("got = %d, err = %v", got, err)
		}
	})
}

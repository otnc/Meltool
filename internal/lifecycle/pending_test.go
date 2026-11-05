package lifecycle

import (
	"testing"
	"time"
)

func TestPending(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	t.Run("なければ nil を返す", func(t *testing.T) {
		p, err := LoadPending(t.TempDir())
		if err != nil || p != nil {
			t.Errorf("p = %+v, err = %v", p, err)
		}
	})
	t.Run("保存したものを読める", func(t *testing.T) {
		root := t.TempDir()
		want := Pending{StoreID: "v1.5.0_windows_amd64_standard", Tag: "v1.5.0", Variant: "standard", Reason: "Meltype が動いていた", CreatedAt: now}

		if err := SavePending(root, want); err != nil {
			t.Fatal(err)
		}
		got, err := LoadPending(root)
		if err != nil || got == nil || *got != want {
			t.Errorf("got = %+v, err = %v", got, err)
		}
	})
	t.Run("新しく保存すると、前のものと置き換わる", func(t *testing.T) {
		root := t.TempDir()
		SavePending(root, Pending{StoreID: "a", Tag: "v1.4.0", CreatedAt: now})
		SavePending(root, Pending{StoreID: "b", Tag: "v1.5.0", CreatedAt: now})

		got, _ := LoadPending(root)
		if got == nil || got.StoreID != "b" {
			t.Errorf("got = %+v", got)
		}
	})
	t.Run("取り消すと消え、取り消したかを返す", func(t *testing.T) {
		root := t.TempDir()
		SavePending(root, Pending{StoreID: "a", CreatedAt: now})

		removed, err := DiscardPending(root)
		again, _ := DiscardPending(root)
		got, _ := LoadPending(root)

		if err != nil || !removed || again || got != nil {
			t.Errorf("removed = %t, again = %t, got = %+v, err = %v", removed, again, got, err)
		}
	})
	t.Run("指定した保持 ID のものだけを消す", func(t *testing.T) {
		root := t.TempDir()
		SavePending(root, Pending{StoreID: "a", CreatedAt: now})

		DiscardPendingFor(root, "b")
		if got, _ := LoadPending(root); got == nil {
			t.Fatalf("違う保持 ID で消した")
		}
		DiscardPendingFor(root, "a")
		if got, _ := LoadPending(root); got != nil {
			t.Errorf("同じ保持 ID で消していない: %+v", got)
		}
	})
	t.Run("管理をやめると消える", func(t *testing.T) {
		root := t.TempDir()
		SavePending(root, Pending{StoreID: "a", CreatedAt: now})

		if err := Detach(root, ""); err != nil {
			t.Fatal(err)
		}
		if got, _ := LoadPending(root); got != nil {
			t.Errorf("got = %+v", got)
		}
	})
}

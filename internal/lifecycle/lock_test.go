package lifecycle

import (
	"errors"
	"os"
	"path/filepath"
	"testing"
)

func TestAcquireLock(t *testing.T) {
	root := t.TempDir()
	first, err := AcquireLock(root)
	if err != nil {
		t.Fatal(err)
	}

	if _, err := AcquireLock(root); !errors.Is(err, ErrLocked) {
		t.Errorf("取得中の排他をもう一度取れた: %v", err)
	}
	if err := first.Release(); err != nil {
		t.Fatal(err)
	}
	again, err := AcquireLock(root)
	if err != nil {
		t.Errorf("解いたあとに取れない: %v", err)
	} else {
		again.Release()
	}
}

func TestAcquireLockIfManaged(t *testing.T) {
	t.Run("state がなければ何も作らずに nil を返す", func(t *testing.T) {
		root := filepath.Join(t.TempDir(), "root")

		lock, err := AcquireLockIfManaged(root)
		if err != nil || lock != nil {
			t.Fatalf("lock = %v, err = %v", lock, err)
		}
		if _, err := os.Stat(root); !os.IsNotExist(err) {
			t.Errorf("管理ルートを作った: %v", err)
		}
	})
	t.Run("state があれば排他を取る", func(t *testing.T) {
		root := t.TempDir()
		first, err := AcquireLock(root)
		if err != nil {
			t.Fatal(err)
		}
		defer first.Release()

		if _, err := AcquireLockIfManaged(root); !errors.Is(err, ErrLocked) {
			t.Errorf("取得中の排他をもう一度取れた: %v", err)
		}
	})
}

package lifecycle

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/gofrs/flock"

	"github.com/otnc/vmeltype/internal/privatefs"
)

// ErrLocked は、別の vMeltype が同じ管理ルートを変更中であることを表す。
var ErrLocked = errors.New("別の vMeltype が同じ管理ルートを変更中です。終わってから実行してください")

// Lock は、管理ルートの変更の排他。OS のファイルロックなので、異常終了してもロックは残らない。
type Lock struct {
	file *flock.Flock
}

// AcquireLock は、管理ルートの排他を待たずに取る。取れなければ ErrLocked を返す。
func AcquireLock(root string) (*Lock, error) {
	dir := stateDir(root)
	if err := privatefs.MkdirAll(dir); err != nil {
		return nil, err
	}
	f := flock.New(filepath.Join(dir, "lock"), flock.SetPermissions(0o600))
	locked, err := f.TryLock()
	if err != nil {
		return nil, fmt.Errorf("排他のファイルを開けませんでした: %w", err)
	}
	if !locked {
		return nil, ErrLocked
	}
	return &Lock{file: f}, nil
}

// AcquireLockIfManaged は、管理ルートに state があれば AcquireLock と同じく排他を取る。state がなければ、変えるものがないので何も作らずに nil を返す。
func AcquireLockIfManaged(root string) (*Lock, error) {
	if _, err := os.Stat(stateDir(root)); errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	} else if err != nil {
		return nil, err
	}
	return AcquireLock(root)
}

// Release は、排他を解く。
func (l *Lock) Release() error {
	return l.file.Unlock()
}

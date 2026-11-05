package lifecycle

import (
	"bytes"
	"crypto/sha256"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"github.com/otnc/vmeltype/internal/upstreamconf"
)

// Detach は、管理の記録 (有効なバージョン、直前のバージョン、固定、適用待ち、管理前の記録) を外す。保持バージョンとスナップショットは残す。
// standardDataDir が空でなければ、先に本家の自動更新を管理前の値に戻し、戻せなければ何も外さない。
// 管理前の記録も外すので、あとで use したときは、その時点の環境を管理前として記録し直す。
// 呼び出し側は、管理ルートの排他を取り、未完了の処理記録がないことを確かめ、Meltype を止めておく。
func Detach(root, standardDataDir string) error {
	baseline, err := LoadBaseline(root)
	if err != nil {
		return err
	}
	if standardDataDir != "" && baseline.StandardConfig != nil {
		if err := upstreamconf.RestoreAutoUpdate(filepath.Join(standardDataDir, "config.json"), *baseline.StandardConfig); err != nil {
			return fmt.Errorf("本家の自動更新を戻せなかったので、管理をやめていません: %w", err)
		}
	}
	if err := saveState(root, State{}); err != nil {
		return err
	}
	if _, err := DiscardPending(root); err != nil {
		return err
	}
	if err := os.Remove(baselinePath(root)); err != nil && !errors.Is(err, os.ErrNotExist) {
		return err
	}
	return os.RemoveAll(filepath.Join(root, "baseline"))
}

// CopyPortable は、portable 版の実行先 (app と、その中の data) を dest へ写し、中身が一致することを確かめる。
// dest は、ないか空のフォルダーにする。一致しなければ、写したものを消してエラーを返す。管理ルートの中のものには触れない。
func CopyPortable(root, dest string) error {
	src := PortableAppDir(root)
	if err := copyTree(src, dest); err != nil {
		os.RemoveAll(dest)
		return err
	}
	if err := sameTree(src, dest); err != nil {
		os.RemoveAll(dest)
		return err
	}
	return nil
}

// sameTree は、b に a と同じファイルが同じ中身でそろっていることを確かめる。
func sameTree(a, b string) error {
	count := 0
	err := filepath.WalkDir(a, func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(a, path)
		if err != nil {
			return err
		}
		count++
		same, err := sameFile(path, filepath.Join(b, rel))
		if err != nil {
			return err
		}
		if !same {
			return fmt.Errorf("写した %s の中身が元と一致しません", rel)
		}
		return nil
	})
	if err != nil {
		return err
	}
	copied := 0
	err = filepath.WalkDir(b, func(_ string, d os.DirEntry, err error) error {
		if err == nil && !d.IsDir() {
			copied++
		}
		return err
	})
	if err == nil && copied != count {
		err = fmt.Errorf("写した先のファイルの数 (%d) が元 (%d) と一致しません", copied, count)
	}
	return err
}

func sameFile(a, b string) (bool, error) {
	ha, err := hashFile(a)
	if err != nil {
		return false, err
	}
	hb, err := hashFile(b)
	if err != nil {
		return false, err
	}
	return bytes.Equal(ha, hb), nil
}

func hashFile(path string) ([]byte, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	h := sha256.New()
	if _, err := io.Copy(h, f); err != nil {
		return nil, err
	}
	return h.Sum(nil), nil
}

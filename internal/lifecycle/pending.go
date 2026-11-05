package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"
)

// Pending は、条件がそろわず、まだ適用していない要求 (適用待ち)。apply で適用する。1 つだけ持ち、新しいものは前のものと置き換わる。
type Pending struct {
	StoreID string `json:"storeId"`
	Tag     string `json:"tag"`
	Variant string `json:"variant"`
	// Reason は、適用できなかった理由。利用者に示す。
	Reason    string    `json:"reason"`
	CreatedAt time.Time `json:"createdAt"`
}

type pendingFile struct {
	SchemaVersion int     `json:"schemaVersion"`
	Pending       Pending `json:"pending"`
}

func pendingPath(root string) string {
	return filepath.Join(stateDir(root), "pending.json")
}

// LoadPending は、適用待ちの要求を読む。なければ nil を返す。
func LoadPending(root string) (*Pending, error) {
	var f pendingFile
	if err := readJSON(pendingPath(root), &f); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return nil, nil
		}
		return nil, fmt.Errorf("適用待ちの記録を読めませんでした: %w", err)
	}
	if f.SchemaVersion > SchemaVersion {
		return nil, errNewerSchema
	}
	return &f.Pending, nil
}

// SavePending は、適用待ちの要求を記録する。呼び出し側は、管理ルートの排他を取っておく。
func SavePending(root string, p Pending) error {
	p.CreatedAt = p.CreatedAt.UTC()
	return writeJSON(pendingPath(root), &pendingFile{SchemaVersion: SchemaVersion, Pending: p})
}

// DiscardPending は、適用待ちの要求を取り消す。取り消したかを返す。呼び出し側は、管理ルートの排他を取っておく。
func DiscardPending(root string) (bool, error) {
	err := os.Remove(pendingPath(root))
	if errors.Is(err, os.ErrNotExist) {
		return false, nil
	}
	return err == nil, err
}

// DiscardPendingFor は、適用待ちの要求が storeID のものなら取り消す。そのバージョンに切り替え終えたときに呼ぶ。
func DiscardPendingFor(root, storeID string) error {
	p, err := LoadPending(root)
	if err != nil || p == nil || p.StoreID != storeID {
		return err
	}
	_, err = DiscardPending(root)
	return err
}

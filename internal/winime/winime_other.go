//go:build !windows

package winime

import (
	"context"
	"errors"
)

// Replace は、Windows 以外では使わない。
func Replace(context.Context, string) (Result, error) {
	return "", errors.New("Meltype IME の入れ替えは、Windows でだけ行えます")
}

// Register は、Windows 以外では使わない。
func Register(context.Context, string) (Result, error) {
	return "", errors.New("Meltype IME の登録は、Windows でだけ行えます")
}

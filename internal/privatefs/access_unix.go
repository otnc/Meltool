//go:build !windows

package privatefs

import (
	"fmt"
	"os"
)

// OthersCanAccess は、所有者のほかに path を使える権限があれば、その権限を返す。なければ空を返す。
func OthersCanAccess(path string) (string, error) {
	info, err := os.Stat(path)
	if err != nil {
		return "", err
	}
	if perm := info.Mode().Perm(); perm&0o077 != 0 {
		return fmt.Sprintf("権限 %o", perm), nil
	}
	return "", nil
}

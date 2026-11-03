//go:build !windows

package privatefs

import (
	"os"
	"testing"
)

func othersCanAccess(t *testing.T, path string) string {
	t.Helper()
	others, err := OthersCanAccess(path)
	if err != nil {
		t.Fatal(err)
	}
	return others
}

// mkdirShared は、ほかの利用者も読めるディレクトリを作る。
func mkdirShared(t *testing.T, path string) {
	t.Helper()
	if err := os.Mkdir(path, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(path, 0o755); err != nil {
		t.Fatal(err)
	}
}

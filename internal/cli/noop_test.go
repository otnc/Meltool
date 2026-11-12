package cli

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestCommandsThatChangeNothingDoNotCreateRoot(t *testing.T) {
	for _, args := range [][]string{
		{"current"}, {"list"}, {"status"}, {"doctor"}, {"snapshot", "list"}, {"app", "config", "show"}, {"track", "status"},
		{"pin"}, {"unpin"}, {"__complete", "use", ""}, {"__complete", "uninstall", ""}, {"recover"}, {"recover", "-n"}, {"rollback", "-y"}, {"apply", "-y"}, {"apply", "--discard"},
		{"snapshot", "create"}, {"snapshot", "prune", "-y"}, {"track", "disable"}, {"detach", "-y"}, {"use", "-y"}, {"uninstall", "1.4.0", "-y"},
	} {
		t.Run(strings.Join(args, " ")+" は管理ルートを作らない", func(t *testing.T) {
			l := newLocalEnv(t)
			l.root = filepath.Join(l.root, "root")

			l.run(t, false, "", args...)

			if _, err := os.Stat(l.root); !os.IsNotExist(err) {
				t.Errorf("管理ルートを作った: %v", rootEntries(l.root))
			}
		})
	}
}

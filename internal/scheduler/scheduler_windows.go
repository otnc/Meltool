//go:build windows

package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/otnc/vmeltype/internal/lifecycle"
)

func register(job Job) (lifecycle.TrackRegistration, error) {
	// schtasks の /TR は 261 文字までなので、長いパスでも収まるよう XML で渡す
	dir, err := os.MkdirTemp("", "vmeltype-track-")
	if err != nil {
		return lifecycle.TrackRegistration{}, err
	}
	defer os.RemoveAll(dir)
	path := filepath.Join(dir, "task.xml")
	if err := os.WriteFile(path, taskXML(job, time.Now().Add(job.Interval)), 0o600); err != nil {
		return lifecycle.TrackRegistration{}, err
	}
	if err := schtasks("/Create", "/TN", taskName, "/XML", path, "/F"); err != nil {
		return lifecycle.TrackRegistration{}, err
	}
	return lifecycle.TrackRegistration{Kind: kindTaskScheduler, ID: taskName}, nil
}

func unregister(reg lifecycle.TrackRegistration) error {
	if err := schtasks("/Query", "/TN", reg.ID); err != nil {
		// もうない
		return nil
	}
	return schtasks("/Delete", "/TN", reg.ID, "/F")
}

func schtasks(args ...string) error {
	out, err := exec.Command("schtasks.exe", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("schtasks %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

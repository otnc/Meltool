//go:build linux

package scheduler

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/otnc/vmeltype/internal/lifecycle"
)

func register(job Job) (lifecycle.TrackRegistration, error) {
	dir, err := unitDir()
	if err != nil {
		return lifecycle.TrackRegistration{}, err
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return lifecycle.TrackRegistration{}, err
	}
	service, timer := systemdUnits(job)
	files := []string{filepath.Join(dir, systemdName+".service"), filepath.Join(dir, systemdName+".timer")}
	for i, body := range []string{service, timer} {
		if err := os.WriteFile(files[i], []byte(body), 0o644); err != nil {
			return lifecycle.TrackRegistration{}, err
		}
	}
	reg := lifecycle.TrackRegistration{Kind: kindSystemdTimer, ID: systemdName + ".timer", Files: files}
	if err := systemctl("daemon-reload"); err != nil {
		unregister(reg)
		return lifecycle.TrackRegistration{}, err
	}
	if err := systemctl("enable", "--now", reg.ID); err != nil {
		unregister(reg)
		return lifecycle.TrackRegistration{}, err
	}
	return reg, nil
}

func unregister(reg lifecycle.TrackRegistration) error {
	systemctl("disable", "--now", reg.ID)
	for _, f := range reg.Files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return systemctl("daemon-reload")
}

func unitDir() (string, error) {
	if base := os.Getenv("XDG_CONFIG_HOME"); filepath.IsAbs(base) {
		return filepath.Join(base, "systemd", "user"), nil
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".config", "systemd", "user"), nil
}

func systemctl(args ...string) error {
	out, err := exec.Command("systemctl", append([]string{"--user"}, args...)...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("systemctl --user %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

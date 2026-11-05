//go:build darwin

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
	home, err := os.UserHomeDir()
	if err != nil {
		return lifecycle.TrackRegistration{}, err
	}
	path := filepath.Join(home, "Library", "LaunchAgents", launchAgentLabel+".plist")
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return lifecycle.TrackRegistration{}, err
	}
	// 読み込み済みなら、新しい定義で読み込み直す
	launchctl("bootout", guiDomain()+"/"+launchAgentLabel)
	if err := os.WriteFile(path, launchAgentPlist(job), 0o644); err != nil {
		return lifecycle.TrackRegistration{}, err
	}
	if err := launchctl("bootstrap", guiDomain(), path); err != nil {
		os.Remove(path)
		return lifecycle.TrackRegistration{}, err
	}
	return lifecycle.TrackRegistration{Kind: kindLaunchAgent, ID: launchAgentLabel, Files: []string{path}}, nil
}

func unregister(reg lifecycle.TrackRegistration) error {
	launchctl("bootout", guiDomain()+"/"+reg.ID)
	for _, f := range reg.Files {
		if err := os.Remove(f); err != nil && !os.IsNotExist(err) {
			return err
		}
	}
	return nil
}

func guiDomain() string {
	return fmt.Sprintf("gui/%d", os.Getuid())
}

func launchctl(args ...string) error {
	out, err := exec.Command("launchctl", args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("launchctl %s: %w: %s", args[0], err, strings.TrimSpace(string(out)))
	}
	return nil
}

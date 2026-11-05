//go:build !windows && !darwin && !linux

package scheduler

import (
	"errors"

	"github.com/otnc/vmeltype/internal/lifecycle"
)

func register(Job) (lifecycle.TrackRegistration, error) {
	return lifecycle.TrackRegistration{}, errors.New("この OS では、追従の処理を登録できません")
}

func unregister(lifecycle.TrackRegistration) error {
	return errors.New("この OS では、追従の処理の登録を外せません")
}

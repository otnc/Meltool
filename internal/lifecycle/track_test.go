package lifecycle

import (
	"errors"
	"testing"
	"time"
)

func TestTrack(t *testing.T) {
	now := time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC)
	t.Run("設定がなければ無効", func(t *testing.T) {
		s, last, err := LoadTrack(t.TempDir())
		if err != nil || s.Enabled || last != nil {
			t.Errorf("s = %+v, last = %+v, err = %v", s, last, err)
		}
	})
	t.Run("設定と結果は、互いを消さずに記録できる", func(t *testing.T) {
		root := t.TempDir()
		settings := TrackSettings{Enabled: true, Channel: TrackStable, Mode: TrackDownload, IntervalSeconds: 86400, Registration: &TrackRegistration{Kind: "task-scheduler", ID: `vMeltype\track`}, ChangedAt: now}

		SaveTrackSettings(root, settings)
		SaveTrackRun(root, TrackRun{At: now, Outcome: "found", Found: "v1.5.0"})
		settings.Mode = TrackApply
		SaveTrackSettings(root, settings)

		s, last, err := LoadTrack(root)
		if err != nil || s.Mode != TrackApply || s.Interval() != 24*time.Hour || s.Registration == nil || last == nil || last.Found != "v1.5.0" {
			t.Errorf("s = %+v, last = %+v, err = %v", s, last, err)
		}
	})
	t.Run("追従の処理は同時に 2 つ動かない", func(t *testing.T) {
		root := t.TempDir()
		lock, err := AcquireTrackLock(root)
		if err != nil {
			t.Fatal(err)
		}
		defer lock.Release()

		if _, err := AcquireTrackLock(root); !errors.Is(err, ErrTrackRunning) {
			t.Errorf("err = %v", err)
		}
		other, err := AcquireLock(root)
		if err != nil {
			t.Errorf("管理ルートの排他まで取れなくなった: %v", err)
		} else {
			other.Release()
		}
	})
}

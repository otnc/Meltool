package lifecycle

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"

	"github.com/otnc/vmeltype/internal/privatefs"
)

// TrackChannel は、追従するバージョンの範囲。
type TrackChannel string

// TrackMode は、新しいバージョンを見つけたときにすること。
type TrackMode string

const (
	// TrackStable は、正式版だけを追う。
	TrackStable TrackChannel = "stable"
	// TrackPrerelease は、プレリリースと正式版を合わせた最大を追う。
	TrackPrerelease TrackChannel = "prerelease"

	// TrackNotify は、見つけたことを記録するだけで、保持バージョンも有効なバージョンも変えない。
	TrackNotify TrackMode = "notify"
	// TrackDownload は、取得して保持し、適用待ちにする。有効なバージョンは変えない。
	TrackDownload TrackMode = "download"
	// TrackApply は、取得したあと、安全な条件がそろったときだけ切り替える。そろわなければ適用待ちにする。
	TrackApply TrackMode = "apply"
)

// ErrTrackRunning は、別の追従の処理が動いていることを表す。
var ErrTrackRunning = errors.New("別の追従の処理が動いています")

// TrackRegistration は、追従の処理を OS のスケジューラーに登録した記録。外すときに使う。
type TrackRegistration struct {
	// Kind は、登録先の種類 (task-scheduler、launch-agent、systemd-timer)。
	Kind string `json:"kind"`
	// ID は、登録先での名前 (タスクの名前、LaunchAgent のラベル、タイマーの名前)。
	ID string `json:"id"`
	// Files は、登録のために vMeltype が置いたファイル。外すときに消す。
	Files []string `json:"files,omitempty"`
}

// TrackSettings は、新しいバージョンへの自動の追従の設定。既定は無効。
type TrackSettings struct {
	Enabled         bool               `json:"enabled"`
	Channel         TrackChannel       `json:"channel,omitempty"`
	Mode            TrackMode          `json:"mode,omitempty"`
	IntervalSeconds int64              `json:"intervalSeconds,omitempty"`
	Registration    *TrackRegistration `json:"registration,omitempty"`
	// Push は、見つけたことなどを OS の通知でも知らせるか。既定は無効で、有効にするときに OS の権限を求める。
	Push      bool      `json:"push,omitempty"`
	ChangedAt time.Time `json:"changedAt,omitzero"`
}

// Interval は、追従の処理を動かす間隔。
func (s TrackSettings) Interval() time.Duration {
	return time.Duration(s.IntervalSeconds) * time.Second
}

// TrackRun は、追従の処理を最後に動かした結果。
type TrackRun struct {
	At time.Time `json:"at"`
	// Outcome は、結果の種類 (latest、found、downloaded、applied、pending、deferred、failed)。
	Outcome string `json:"outcome"`
	// Found は、見つけた新しいバージョンのタグ。
	Found   string `json:"found,omitempty"`
	Message string `json:"message"`
}

type trackFile struct {
	SchemaVersion int           `json:"schemaVersion"`
	Settings      TrackSettings `json:"settings"`
	Last          *TrackRun     `json:"last,omitempty"`
}

func trackPath(root string) string {
	return filepath.Join(stateDir(root), "track.json")
}

func loadTrackFile(root string) (trackFile, error) {
	var f trackFile
	if err := readJSON(trackPath(root), &f); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return trackFile{}, nil
		}
		return trackFile{}, fmt.Errorf("追従の設定を読めませんでした: %w", err)
	}
	if f.SchemaVersion > SchemaVersion {
		return trackFile{}, errNewerSchema
	}
	return f, nil
}

func saveTrackFile(root string, f trackFile) error {
	f.SchemaVersion = SchemaVersion
	return writeJSON(trackPath(root), &f)
}

// LoadTrack は、追従の設定と、最後に動かした結果 (なければ nil) を読む。設定がなければ、無効の設定を返す。
func LoadTrack(root string) (TrackSettings, *TrackRun, error) {
	f, err := loadTrackFile(root)
	return f.Settings, f.Last, err
}

// SaveTrackSettings は、追従の設定を記録する。最後に動かした結果は残す。呼び出し側は、管理ルートの排他を取っておく。
func SaveTrackSettings(root string, s TrackSettings) error {
	f, err := loadTrackFile(root)
	if err != nil {
		return err
	}
	f.Settings = s
	return saveTrackFile(root, f)
}

// SaveTrackRun は、追従の処理を動かした結果を記録する。呼び出し側は、AcquireTrackLock で追従の排他を取っておく。
func SaveTrackRun(root string, run TrackRun) error {
	f, err := loadTrackFile(root)
	if err != nil {
		return err
	}
	run.At = run.At.UTC()
	f.Last = &run
	return saveTrackFile(root, f)
}

// AcquireTrackLock は、追従の処理の排他を待たずに取る。管理ルートの排他とは別で、取れなければ ErrTrackRunning を返す。
// 追従の処理は、切り替えるときに管理ルートの排他も取るので、こちらは同じ処理が重なって動かないためだけに使う。
func AcquireTrackLock(root string) (*Lock, error) {
	dir := stateDir(root)
	if err := privatefs.MkdirAll(dir); err != nil {
		return nil, err
	}
	f := flock.New(filepath.Join(dir, "track.lock"), flock.SetPermissions(0o600))
	locked, err := f.TryLock()
	if err != nil {
		return nil, fmt.Errorf("追従の排他のファイルを開けませんでした: %w", err)
	}
	if !locked {
		return nil, ErrTrackRunning
	}
	return &Lock{file: f}, nil
}

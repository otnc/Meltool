package cli

import (
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"

	"github.com/otnc/vmeltype/internal/assets"
	"github.com/otnc/vmeltype/internal/lifecycle"
	"github.com/otnc/vmeltype/internal/store"
	"github.com/otnc/vmeltype/internal/versions"
	"github.com/otnc/vmeltype/internal/winsys"
)

const trackEnableLong = `新しいバージョンへの自動の追従を有効にし、OS のスケジューラーに、追従の処理 (vmeltype track run) を登録します。既定は無効です。

--channel stable (既定) は正式版だけ、prerelease はプレリリースと正式版を合わせた最大を追います。有効なバージョンより古いバージョンには下げません。
--mode notify (既定) は、見つけたことを記録するだけです。download は、取得して保持し、適用待ちにします。"vmeltype apply" で適用します。apply は、固定していない、Meltype が止まっている、IME の DLL の入れ替えに管理者権限が要らない、のすべてがそろったときだけ切り替え、そろわなければ理由を付けて適用待ちにします。
--interval は確かめる間隔で、既定は 24h です。1h より短くはできません。
見つけたことなどは、次に "vmeltype status" などを使ったときに示します。--push を付けると、OS の通知でも知らせます。有効にするときに、OS が通知の許可を求めることがあります。
もう一度実行すると、前の設定と登録を置き換えます。`

const (
	defaultTrackInterval = 24 * time.Hour
	minTrackInterval     = time.Hour
)

// 追従の処理の結果の種類。
const (
	trackLatest     = "latest"
	trackFound      = "found"
	trackDownloaded = "downloaded"
	trackApplied    = "applied"
	trackPending    = "pending"
	trackDeferred   = "deferred"
	trackFailed     = "failed"
)

var trackChannelLabels = map[lifecycle.TrackChannel]string{lifecycle.TrackStable: "正式版", lifecycle.TrackPrerelease: "プレリリースと正式版"}

func setupTrackEnableFlags(cmd *cobra.Command) {
	cmd.Flags().String("channel", string(lifecycle.TrackStable), "追うバージョンの範囲 (stable か prerelease)")
	cmd.Flags().String("mode", string(lifecycle.TrackNotify), "見つけたときにすること (notify、download、apply)")
	cmd.Flags().Duration("interval", defaultTrackInterval, "確かめる間隔 (1h 以上)")
	cmd.Flags().Bool("push", false, "OS の通知でも知らせる。有効にするときに OS が許可を求めることがある")
}

func runTrackEnable(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		flags := cmd.Flags()
		channelText, _ := flags.GetString("channel")
		modeText, _ := flags.GetString("mode")
		interval, _ := flags.GetDuration("interval")
		push, _ := flags.GetBool("push")
		asJSON, _ := flags.GetBool("json")
		channel := lifecycle.TrackChannel(channelText)
		mode := lifecycle.TrackMode(modeText)
		if _, ok := trackChannelLabels[channel]; !ok {
			return usageError{fmt.Errorf("--channel は stable か prerelease です: %q", channelText)}
		}
		if mode != lifecycle.TrackNotify && mode != lifecycle.TrackDownload && mode != lifecycle.TrackApply {
			return usageError{fmt.Errorf("--mode は notify、download、apply のどれかです: %q", modeText)}
		}
		if interval < minTrackInterval {
			return usageError{fmt.Errorf("--interval は %s 以上にしてください: %s", minTrackInterval, interval)}
		}
		if env.RegisterTrack == nil || env.Executable == nil {
			return errors.New("この OS では、まだ追従の処理を登録できません")
		}
		exe, err := env.Executable()
		if err != nil {
			return err
		}
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		lock, err := lifecycle.AcquireLock(root)
		if err != nil {
			return err
		}
		defer lock.Release()
		if push {
			if env.RequestPush == nil {
				return errors.New("この OS では、まだ push 通知に対応していません。--push を外して実行してください")
			}
			if err := env.RequestPush(); err != nil {
				return fmt.Errorf("push 通知を使えないので、追従を有効にしていません。--push を外せば、通知なしで有効にできます: %w", err)
			}
		}
		previous, _, err := lifecycle.LoadTrack(root)
		if err != nil {
			return err
		}
		if err := unregisterTrack(env, previous); err != nil {
			return err
		}
		registration, err := env.RegisterTrack(exe, root, interval)
		if err != nil {
			return fmt.Errorf("追従の処理を OS のスケジューラーに登録できなかったので、有効にしていません: %w", err)
		}
		settings := lifecycle.TrackSettings{Enabled: true, Channel: channel, Mode: mode, IntervalSeconds: int64(interval / time.Second), Registration: &registration, Push: push, ChangedAt: env.Now().UTC()}
		if err := lifecycle.SaveTrackSettings(root, settings); err != nil {
			return errors.Join(err, unregisterTrack(env, settings))
		}
		if asJSON {
			return writeJSON(cmd.OutOrStdout(), map[string]any{"settings": settings})
		}
		fmt.Fprintln(cmd.OutOrStdout(), "追従を有効にしました。")
		writeTrackSettings(cmd.OutOrStdout(), settings)
		return nil
	}
}

func runTrackDisable(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		lock, err := lifecycle.AcquireLock(root)
		if err != nil {
			return err
		}
		defer lock.Release()
		changed, err := disableTrack(env, root)
		if err != nil {
			return err
		}
		if asJSON {
			return writeJSON(cmd.OutOrStdout(), map[string]any{"changed": changed})
		}
		if !changed {
			fmt.Fprintln(cmd.OutOrStdout(), "追従は無効です。")
			return nil
		}
		fmt.Fprintln(cmd.OutOrStdout(), "追従を無効にし、OS のスケジューラーへの登録を外しました。")
		return nil
	}
}

// disableTrack は、追従を無効にし、OS のスケジューラーへの登録を外す。有効でなければ false を返す。呼び出し側は、管理ルートの排他を取っておく。
func disableTrack(env *Env, root string) (bool, error) {
	settings, _, err := lifecycle.LoadTrack(root)
	if err != nil || (!settings.Enabled && settings.Registration == nil) {
		return false, err
	}
	if err := unregisterTrack(env, settings); err != nil {
		return false, err
	}
	return true, lifecycle.SaveTrackSettings(root, lifecycle.TrackSettings{ChangedAt: env.Now().UTC()})
}

func unregisterTrack(env *Env, settings lifecycle.TrackSettings) error {
	if settings.Registration == nil {
		return nil
	}
	if env.UnregisterTrack == nil {
		return errors.New("この OS では、追従の処理の登録を外せません")
	}
	if err := env.UnregisterTrack(*settings.Registration); err != nil {
		return fmt.Errorf("追従の処理の登録 (%s) を外せませんでした: %w", settings.Registration.ID, err)
	}
	return nil
}

func runTrackStatus(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		asJSON, _ := cmd.Flags().GetBool("json")
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		settings, last, err := lifecycle.LoadTrack(root)
		if err != nil {
			return err
		}
		out := cmd.OutOrStdout()
		if asJSON {
			return writeJSON(out, map[string]any{"settings": settings, "last": last})
		}
		if !settings.Enabled {
			fmt.Fprintln(out, "追従:         無効。\"vmeltype track enable\" で有効にします")
		} else {
			writeTrackSettings(out, settings)
		}
		if last != nil {
			fmt.Fprintf(out, "最後の確認:   %s %s\n", last.At.Local().Format("2006-01-02 15:04"), last.Message)
		}
		return nil
	}
}

func writeTrackSettings(w io.Writer, s lifecycle.TrackSettings) {
	push := "使わない"
	if s.Push {
		push = "使う"
	}
	fmt.Fprintf(w, "追従:         有効 (%s、%s、%s ごと)\n", trackChannelLabels[s.Channel], s.Mode, s.Interval())
	fmt.Fprintf(w, "push 通知:    %s\n", push)
	if s.Registration != nil {
		fmt.Fprintf(w, "登録:         %s (%s)\n", s.Registration.ID, s.Registration.Kind)
	}
}

// runTrackRun は、OS のスケジューラーから呼ばれる追従の処理。画面なしで動くので、結果は記録に残し、終了コードは 0 にする。
// 通信できないなど、次の回に回せるものは静かに延期する。
func runTrackRun(env *Env) func(*cobra.Command, []string) error {
	return func(cmd *cobra.Command, _ []string) error {
		root, err := managementRoot(cmd, env)
		if err != nil {
			return err
		}
		settings, _, err := lifecycle.LoadTrack(root)
		if err != nil || !settings.Enabled {
			return err
		}
		lock, err := lifecycle.AcquireTrackLock(root)
		if errors.Is(err, lifecycle.ErrTrackRunning) {
			return nil
		}
		if err != nil {
			return err
		}
		defer lock.Release()
		run := trackOnce(cmd, env, root, settings)
		run.At = env.Now()
		if err := lifecycle.SaveTrackRun(root, run); err != nil {
			return err
		}
		if settings.Push && run.Outcome != trackLatest && run.Outcome != trackDeferred && env.Push != nil {
			if err := env.Push("vMeltype", run.Message); err != nil {
				fmt.Fprintf(cmd.ErrOrStderr(), "push 通知を出せませんでした: %v\n", err)
			}
		}
		return nil
	}
}

func trackOnce(cmd *cobra.Command, env *Env, root string, settings lifecycle.TrackSettings) lifecycle.TrackRun {
	if journal, err := lifecycle.LoadJournal(root); err != nil || journal != nil {
		return lifecycle.TrackRun{Outcome: trackDeferred, Message: "中断した変更があるので、次の回に回しました。\"vmeltype recover\" で復旧してください"}
	}
	active, release, err := findUpdate(cmd, env, root, settings.Channel == lifecycle.TrackPrerelease)
	if err != nil {
		return lifecycle.TrackRun{Outcome: trackDeferred, Message: "確かめられなかったので、次の回に回しました: " + err.Error()}
	}
	if release == nil {
		return lifecycle.TrackRun{Outcome: trackLatest, Message: active.Tag + " は最新です"}
	}
	found := lifecycle.TrackRun{Outcome: trackFound, Found: release.Tag, Message: fmt.Sprintf("新しいバージョン %s があります (いまは %s)。\"vmeltype update\" で更新します", release.Tag, active.Tag)}
	if settings.Mode == lifecycle.TrackNotify {
		return found
	}

	st := store.Store{Root: root}
	variant := assets.Variant(active.Variant)
	id := store.ID(release.Tag, env.Platform.OS, env.Platform.Arch, string(variant))
	target, err := st.Get(id)
	if err != nil {
		asset, err := assets.Select(*release, env.Platform, variant)
		if err == nil {
			var plan installPlan
			if plan, err = newInstallPlan(st, id, *release, asset, false, false); err == nil {
				target, err = downloadAndStore(cmd, env, st, plan, *release, asset)
			}
		}
		if err != nil {
			found.Outcome, found.Message = trackFailed, fmt.Sprintf("新しいバージョン %s を取得できませんでした: %v", release.Tag, err)
			return found
		}
	}

	if settings.Mode == lifecycle.TrackDownload {
		return keepPending(env, root, target, found, trackDownloaded, "追従で取得しました")
	}
	if reason := trackApplyBlocker(env, root, target); reason != "" {
		return keepPending(env, root, target, found, trackPending, reason)
	}
	cmd.Flags().Set("yes", "true")
	cmd.SetOut(io.Discard)
	if err := switchVersion(cmd, env, root, st, target, false, "%s に切り替えますか？", "%s に切り替えました"); err != nil {
		return keepPending(env, root, target, found, trackPending, "切り替えられませんでした: "+err.Error())
	}
	found.Outcome, found.Message = trackApplied, fmt.Sprintf("%s から %s に切り替えました", active.Tag, release.Tag)
	return found
}

// keepPending は、取得したバージョンを適用待ちにし、その結果を返す。
func keepPending(env *Env, root string, target store.Stored, run lifecycle.TrackRun, outcome, reason string) lifecycle.TrackRun {
	lock, err := lifecycle.AcquireLock(root)
	if err == nil {
		err = lifecycle.SavePending(root, lifecycle.Pending{StoreID: target.ID, Tag: target.Tag, Variant: target.Variant, Reason: reason, CreatedAt: env.Now()})
		lock.Release()
	}
	if err != nil {
		run.Outcome, run.Message = trackFailed, fmt.Sprintf("%s を取得しましたが、適用待ちにできませんでした: %v", target.Tag, err)
		return run
	}
	run.Outcome, run.Message = outcome, fmt.Sprintf("%s を適用待ちにしました (%s)。\"vmeltype apply\" で適用します", target.Tag, reason)
	return run
}

// trackApplyBlocker は、追従の処理が人の手を借りずに切り替えてよいかを調べ、だめなら理由を返す。
// 固定を越えない、動いている Meltype を終了させない、管理者権限の確認 (UAC) を出さない。
func trackApplyBlocker(env *Env, root string, target store.Stored) string {
	state, err := lifecycle.LoadState(root)
	if err != nil {
		return err.Error()
	}
	if state.Pin != nil && state.Pin.StoreID != target.ID {
		return fmt.Sprintf("%s に固定しているため", state.Pin.Tag)
	}
	place, err := layoutFor(env, root, target.Variant)
	if err != nil {
		return err.Error()
	}
	if running, err := env.RunningUnder(place.appDir); err != nil || len(running) > 0 {
		return "Meltype が動いているため"
	}
	if target.Variant == string(assets.Standard) {
		same, err := tipMatches(env, filepath.Join(store.Store{Root: root}.Path(target.ID), "files", "app", "tip"))
		if err != nil || !same {
			return "Meltype IME の DLL の入れ替えに管理者権限の確認が要るため"
		}
	}
	return ""
}

// tipMatches は、Meltype IME を登録していないか、登録している DLL が tip の下のものと同じかを返す。同じなら、入れ替えに管理者権限は要らない。
func tipMatches(env *Env, tip string) (bool, error) {
	if env.IMERegistered == nil {
		return true, nil
	}
	registered, err := env.IMERegistered()
	if err != nil || !registered {
		return !registered, err
	}
	installed, err := env.InstalledTipHashes()
	if err != nil {
		return false, err
	}
	for _, arch := range []string{"x64", "x86"} {
		data, err := os.ReadFile(filepath.Join(tip, arch, "MeltypeTip.dll.sha256"))
		if err != nil || winsys.NormalizeHash(string(data)) != installed[arch] {
			return false, nil
		}
	}
	return true, nil
}

// trackNotice は、追従の処理が見つけた新しいバージョンのうち、まだ有効になっていないものを知らせる文言を返す。なければ空を返す。
func trackNotice(root, activeTag string) string {
	_, last, err := lifecycle.LoadTrack(root)
	if err != nil || last == nil || last.Found == "" || versions.Compare(last.Found, activeTag) <= 0 {
		return ""
	}
	return fmt.Sprintf("%s (%s に確認)", last.Message, last.At.Local().Format("2006-01-02 15:04"))
}

// Package scheduler は、追従の処理 (vmeltype track run) を、OS のスケジューラーに登録し、外す。
// Windows はタスク スケジューラー、macOS は LaunchAgent、Linux は systemd のユーザー タイマーを使う。どれも利用者ごとで、管理者権限は使わない。
// 登録の中身 (XML、plist、unit) を作る処理は OS に依らないので、どの OS でもテストできる。
package scheduler

import (
	"bytes"
	"encoding/xml"
	"fmt"
	"strings"
	"time"
	"unicode/utf16"

	"github.com/otnc/vmeltype/internal/lifecycle"
)

// Job は、登録する追従の処理。
type Job struct {
	Executable string
	Root       string
	Interval   time.Duration
}

// 登録先での名前。
const (
	taskName         = `vMeltype\track`
	launchAgentLabel = "io.github.otnc.vmeltype.track"
	systemdName      = "vmeltype-track"
)

// 登録先の種類。lifecycle.TrackRegistration.Kind に記録する。
const (
	kindTaskScheduler = "task-scheduler"
	kindLaunchAgent   = "launch-agent"
	kindSystemdTimer  = "systemd-timer"
)

// Register は、job を OS のスケジューラーに登録する。同じ名前の登録があれば置き換える。
//
//	reg, err := scheduler.Register(scheduler.Job{Executable: exe, Root: root, Interval: 24 * time.Hour})
func Register(job Job) (lifecycle.TrackRegistration, error) {
	return register(job)
}

// Unregister は、Register で登録したものを外す。もうなければ何もしない。
func Unregister(reg lifecycle.TrackRegistration) error {
	return unregister(reg)
}

func escapeXML(s string) string {
	var b bytes.Buffer
	xml.EscapeText(&b, []byte(s))
	return b.String()
}

// taskXML は、タスク スケジューラーに登録するタスクの定義を、schtasks /XML が読める UTF-16 (BOM 付き) で返す。
// console の窓を出さないよう、conhost.exe --headless から動かす。ログオンしている間だけ、この利用者として動く。
func taskXML(job Job, start time.Time) []byte {
	args := fmt.Sprintf(`--headless "%s" track run --root "%s"`, job.Executable, job.Root)
	text := `<?xml version="1.0" encoding="UTF-16"?>
<Task version="1.2" xmlns="http://schemas.microsoft.com/windows/2004/02/mit/task">
  <RegistrationInfo>
    <Description>vMeltype が、Meltype の新しいバージョンを確かめる (vmeltype track)</Description>
  </RegistrationInfo>
  <Triggers>
    <TimeTrigger>
      <Repetition>
        <Interval>` + isoDuration(job.Interval) + `</Interval>
        <StopAtDurationEnd>false</StopAtDurationEnd>
      </Repetition>
      <StartBoundary>` + start.UTC().Format("2006-01-02T15:04:05Z") + `</StartBoundary>
      <Enabled>true</Enabled>
    </TimeTrigger>
  </Triggers>
  <Principals>
    <Principal id="Author">
      <LogonType>InteractiveToken</LogonType>
      <RunLevel>LeastPrivilege</RunLevel>
    </Principal>
  </Principals>
  <Settings>
    <MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>
    <DisallowStartIfOnBatteries>false</DisallowStartIfOnBatteries>
    <StopIfGoingOnBatteries>false</StopIfGoingOnBatteries>
    <StartWhenAvailable>true</StartWhenAvailable>
    <RunOnlyIfNetworkAvailable>true</RunOnlyIfNetworkAvailable>
    <ExecutionTimeLimit>PT30M</ExecutionTimeLimit>
    <Enabled>true</Enabled>
  </Settings>
  <Actions Context="Author">
    <Exec>
      <Command>conhost.exe</Command>
      <Arguments>` + escapeXML(args) + `</Arguments>
    </Exec>
  </Actions>
</Task>
`
	units := utf16.Encode([]rune(text))
	out := make([]byte, 2, 2+2*len(units))
	out[0], out[1] = 0xff, 0xfe
	for _, u := range units {
		out = append(out, byte(u), byte(u>>8))
	}
	return out
}

// isoDuration は、間隔を ISO 8601 の期間 (PT6H など) にする。1 時間より細かい部分は切り捨てる。
func isoDuration(d time.Duration) string {
	return fmt.Sprintf("PT%dH", int64(d/time.Hour))
}

// launchAgentPlist は、macOS の LaunchAgent の定義を返す。
func launchAgentPlist(job Job) []byte {
	args := []string{job.Executable, "track", "run", "--root", job.Root}
	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?>
<!DOCTYPE plist PUBLIC "-//Apple//DTD PLIST 1.0//EN" "http://www.apple.com/DTDs/PropertyList-1.0.dtd">
<plist version="1.0">
<dict>
	<key>Label</key>
	<string>` + launchAgentLabel + `</string>
	<key>ProgramArguments</key>
	<array>
`)
	for _, a := range args {
		b.WriteString("\t\t<string>" + escapeXML(a) + "</string>\n")
	}
	b.WriteString(`	</array>
	<key>StartInterval</key>
	<integer>` + fmt.Sprint(int64(job.Interval/time.Second)) + `</integer>
	<key>ProcessType</key>
	<string>Background</string>
</dict>
</plist>
`)
	return []byte(b.String())
}

// systemdUnits は、systemd のユーザー タイマーの service と timer の unit を返す。
func systemdUnits(job Job) (service, timer string) {
	service = `[Unit]
Description=vMeltype が、Meltype の新しいバージョンを確かめる (vmeltype track)

[Service]
Type=oneshot
ExecStart=` + systemdQuote(job.Executable) + ` track run --root ` + systemdQuote(job.Root) + `
`
	timer = `[Unit]
Description=vMeltype の追従の処理を定期的に動かす

[Timer]
OnBootSec=10min
OnUnitActiveSec=` + fmt.Sprint(int64(job.Interval/time.Second)) + `s

[Install]
WantedBy=timers.target
`
	return service, timer
}

// systemdQuote は、systemd の ExecStart の 1 つの引数として、空白や " を含むパスを逃がす。% は指定子なので %% にする。
func systemdQuote(s string) string {
	s = strings.ReplaceAll(s, `\`, `\\`)
	s = strings.ReplaceAll(s, `"`, `\"`)
	s = strings.ReplaceAll(s, `%`, `%%`)
	return `"` + s + `"`
}

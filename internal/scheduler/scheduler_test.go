package scheduler

import (
	"strings"
	"testing"
	"time"
	"unicode/utf16"
)

var job = Job{Executable: `C:\Users\a b\AppData\Local\Programs\vMeltype\vmeltype.exe`, Root: `C:\Users\a b\AppData\Local\vMeltype`, Interval: 6 * time.Hour}

func TestTaskXML(t *testing.T) {
	data := taskXML(job, time.Date(2026, 11, 6, 9, 0, 0, 0, time.UTC))
	t.Run("UTF-16 の BOM で始まる", func(t *testing.T) {
		if len(data) < 2 || data[0] != 0xff || data[1] != 0xfe {
			t.Fatalf("BOM がない: % x", data[:2])
		}
	})
	text := decodeUTF16(data[2:])
	for _, want := range []string{
		`<Interval>PT6H</Interval>`,
		`<Command>conhost.exe</Command>`,
		`--headless &#34;C:\Users\a b\AppData\Local\Programs\vMeltype\vmeltype.exe&#34; track run --root &#34;C:\Users\a b\AppData\Local\vMeltype&#34;`,
		`<LogonType>InteractiveToken</LogonType>`,
		`<StartWhenAvailable>true</StartWhenAvailable>`,
		`<MultipleInstancesPolicy>IgnoreNew</MultipleInstancesPolicy>`,
		`<StartBoundary>2026-11-06T09:00:00Z</StartBoundary>`,
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%q がない:\n%s", want, text)
		}
	}
}

func TestLaunchAgentPlist(t *testing.T) {
	text := string(launchAgentPlist(Job{Executable: "/Users/a/bin/vmeltype", Root: "/Users/a/Library/Application Support/vMeltype", Interval: 24 * time.Hour}))
	for _, want := range []string{
		"<key>Label</key>\n\t<string>" + launchAgentLabel + "</string>",
		"<string>/Users/a/bin/vmeltype</string>\n\t\t<string>track</string>\n\t\t<string>run</string>\n\t\t<string>--root</string>\n\t\t<string>/Users/a/Library/Application Support/vMeltype</string>",
		"<key>StartInterval</key>\n\t<integer>86400</integer>",
	} {
		if !strings.Contains(text, want) {
			t.Errorf("%q がない:\n%s", want, text)
		}
	}
	t.Run("XML の特別な文字を逃がす", func(t *testing.T) {
		text := string(launchAgentPlist(Job{Executable: "/a&b/<c>", Root: "/r", Interval: time.Hour}))
		if !strings.Contains(text, "/a&amp;b/&lt;c&gt;") {
			t.Errorf("逃がしていない:\n%s", text)
		}
	})
}

func TestSystemdUnits(t *testing.T) {
	service, timer := systemdUnits(Job{Executable: "/home/a/bin/vmeltype", Root: "/home/a/my data/vmeltype", Interval: 12 * time.Hour})
	if !strings.Contains(service, `ExecStart="/home/a/bin/vmeltype" track run --root "/home/a/my data/vmeltype"`) || !strings.Contains(service, "Type=oneshot") {
		t.Errorf("service:\n%s", service)
	}
	if !strings.Contains(timer, "OnUnitActiveSec=43200s") || !strings.Contains(timer, "OnBootSec=") || !strings.Contains(timer, "WantedBy=timers.target") {
		t.Errorf("timer:\n%s", timer)
	}
}

func TestSystemdQuote(t *testing.T) {
	if got := systemdQuote(`/a "b"\c%d`); got != `"/a \"b\"\\c%%d"` {
		t.Errorf("got = %s", got)
	}
}

func decodeUTF16(b []byte) string {
	u := make([]uint16, len(b)/2)
	for i := range u {
		u[i] = uint16(b[2*i]) | uint16(b[2*i+1])<<8
	}
	return string(utf16.Decode(u))
}

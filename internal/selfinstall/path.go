package selfinstall

import "strings"

// PathWith は、; で区切った Windows の PATH の paths に、dir がなければ末尾に足したものを返す。足したかも返す。
// ほかの項目は、順も書き方 (%USERPROFILE% などの展開前の形) も変えない。
func PathWith(paths, dir string) (string, bool) {
	for _, entry := range strings.Split(paths, ";") {
		if sameDir(entry, dir) {
			return paths, false
		}
	}
	if paths != "" && !strings.HasSuffix(paths, ";") {
		paths += ";"
	}
	return paths + dir, true
}

// PathWithout は、; で区切った Windows の PATH の paths から、dir と同じ項目をすべて除いたものを返す。除いたかも返す。
// ほかの項目は、空の項目も含めて、順も書き方も変えない。
func PathWithout(paths, dir string) (string, bool) {
	entries := strings.Split(paths, ";")
	kept := entries[:0:0]
	for _, entry := range entries {
		if entry != "" && sameDir(entry, dir) {
			continue
		}
		kept = append(kept, entry)
	}
	if len(kept) == len(entries) {
		return paths, false
	}
	return strings.Join(kept, ";"), true
}

// sameDir は、PATH の 2 つの項目が同じフォルダーかを、Windows と同じく大文字と小文字、前後の空白、末尾の \ を区別せずに比べる。
func sameDir(a, b string) bool {
	return strings.EqualFold(trimDir(a), trimDir(b))
}

func trimDir(s string) string {
	s = strings.TrimSpace(s)
	// C:\ のようなドライブの直下は、\ を外すと別の意味 (そのドライブの今のフォルダー) になるので残す
	if len(s) > 3 || (len(s) == 3 && s[1] != ':') {
		s = strings.TrimSuffix(s, `\`)
	}
	return s
}

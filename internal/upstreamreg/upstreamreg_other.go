//go:build !windows

package upstreamreg

// SetDisplayVersion は、Windows 以外では何もしない。「設定 → アプリ」の登録は Windows にだけある。
func SetDisplayVersion(string, string) (bool, error) {
	return false, nil
}

//go:build windows

package upstreamreg

import (
	"errors"
	"fmt"

	"golang.org/x/sys/windows/registry"
)

// SetDisplayVersion は、インストール先が appDir の項目の DisplayVersion を version にする。書き換えた項目があれば true を返す。
func SetDisplayVersion(appDir, version string) (bool, error) {
	changed := false
	for _, path := range keys {
		key, err := registry.OpenKey(registry.CURRENT_USER, path, registry.QUERY_VALUE|registry.SET_VALUE)
		if errors.Is(err, registry.ErrNotExist) {
			continue
		}
		if err != nil {
			return changed, fmt.Errorf("Meltype の登録を開けませんでした: %w", err)
		}
		location, _, err := key.GetStringValue("InstallLocation")
		if err == nil && sameLocation(location, appDir) {
			err = key.SetStringValue("DisplayVersion", version)
			if err == nil {
				changed = true
			}
		} else if errors.Is(err, registry.ErrNotExist) {
			err = nil
		}
		key.Close()
		if err != nil {
			return changed, fmt.Errorf("Meltype の登録のバージョンを書き換えられませんでした: %w", err)
		}
	}
	return changed, nil
}

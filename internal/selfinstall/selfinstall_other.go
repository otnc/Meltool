//go:build !windows

package selfinstall

import (
	"context"
	"errors"
)

// Find は、Windows 以外では、インストーラーで入れたものを見つけない。
func Find(string) (Installation, bool, error) {
	return Installation{}, false, nil
}

// RunSetup は、Windows 以外では使わない。
func RunSetup(context.Context, string) error {
	return errors.New("インストーラーは Windows にだけあります")
}

// StartUninstaller は、Windows 以外では使わない。
func StartUninstaller(Installation) error {
	return errors.New("インストーラーは Windows にだけあります")
}

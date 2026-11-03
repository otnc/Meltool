//go:build windows

package privatefs

import (
	"fmt"
	"strings"

	"golang.org/x/sys/windows"
)

// OthersCanAccess は、本人、SYSTEM、Administrators のほかに path を使える主体があれば、その ACL (SDDL) を返す。なければ空を返す。
func OthersCanAccess(path string) (string, error) {
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return "", fmt.Errorf("%s の権限を読めませんでした: %w", path, err)
	}
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return "", fmt.Errorf("利用者の SID を取得できませんでした: %w", err)
	}
	sddl := sd.String()
	rest := sddl
	for _, allowed := range []string{user.User.Sid.String(), ";SY)", ";BA)"} {
		rest = strings.ReplaceAll(rest, allowed, "")
	}
	// WD は Everyone、BU は Users、AU は Authenticated Users。S-1- はほかの利用者やグループの SID
	for _, other := range []string{";WD)", ";BU)", ";AU)", ";S-1-"} {
		if strings.Contains(rest, other) {
			return sddl, nil
		}
	}
	return "", nil
}

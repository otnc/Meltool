//go:build windows

package privatefs

import (
	"os"
	"strings"
	"testing"
	"unsafe"

	"golang.org/x/sys/windows"
)

// othersCanAccess は、ほかの主体が使えるか、ディレクトリが親から ACL を継承していれば、その SDDL を返す。
func othersCanAccess(t *testing.T, path string) string {
	t.Helper()
	others, err := OthersCanAccess(path)
	if err != nil {
		t.Fatal(err)
	}
	if others != "" {
		return others
	}
	// ディレクトリは親から継承しない。中のファイルは、そのディレクトリから継承してよい
	sd, err := windows.GetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		t.Fatal(err)
	}
	if info, err := os.Stat(path); err == nil && info.IsDir() && !strings.Contains(sd.String(), "D:P") {
		return "継承を止めていない: " + sd.String()
	}
	return ""
}

// mkdirShared は、PC の利用者 (Users) も読めるディレクトリを作る。
func mkdirShared(t *testing.T, path string) {
	t.Helper()
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	sd, err := windows.SecurityDescriptorFromString("D:P(A;OICI;FA;;;" + user.User.Sid.String() + ")(A;OICI;GR;;;BU)")
	if err != nil {
		t.Fatal(err)
	}
	p, _ := windows.UTF16PtrFromString(path)
	if err := windows.CreateDirectory(p, &windows.SecurityAttributes{Length: uint32(unsafe.Sizeof(windows.SecurityAttributes{})), SecurityDescriptor: sd}); err != nil {
		t.Fatal(err)
	}
}

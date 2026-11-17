package ownerfile

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

// fileAllAccess is FILE_ALL_ACCESS, which x/sys/windows does not name.
const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff

func sid(t *testing.T, k windows.WELL_KNOWN_SID_TYPE) *windows.SID {
	t.Helper()
	s, err := windows.CreateWellKnownSid(k)
	if err != nil {
		t.Fatal(err)
	}
	return s
}

func processUser(t *testing.T) *windows.SID {
	t.Helper()
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	return u.User.Sid
}

func entry(s *windows.SID, mode windows.ACCESS_MODE, mask windows.ACCESS_MASK) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: mask,
		AccessMode:        mode,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeValue: windows.TrusteeValueFromSID(s),
		},
	}
}

// setDACL replaces the DACL of path with entries and stops it inheriting
// from its directory; nil entries set a null DACL, which allows everyone.
func setDACL(t *testing.T, path string, entries []windows.EXPLICIT_ACCESS) {
	t.Helper()
	var acl *windows.ACL
	if entries != nil {
		var err error
		if acl, err = windows.ACLFromEntries(entries, nil); err != nil {
			t.Fatal(err)
		}
	}
	info := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
	if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, info, nil, nil, acl, nil); err != nil {
		t.Fatal(err)
	}
}

func TestCheckDACL(t *testing.T) {
	user := processUser(t)
	owner := entry(user, windows.SET_ACCESS, fileAllAccess)
	read := windows.ACCESS_MASK(windows.FILE_GENERIC_READ)
	tests := []struct {
		name    string
		entries []windows.EXPLICIT_ACCESS // nil keeps the file's inherited DACL
		null    bool
		shared  bool
	}{
		{name: "inherited in the temp directory"},
		{name: "owner only", entries: []windows.EXPLICIT_ACCESS{owner}},
		{name: "owner, SYSTEM and Administrators", entries: []windows.EXPLICIT_ACCESS{
			owner,
			entry(sid(t, windows.WinLocalSystemSid), windows.SET_ACCESS, fileAllAccess),
			entry(sid(t, windows.WinBuiltinAdministratorsSid), windows.SET_ACCESS, fileAllAccess),
		}},
		{name: "Everyone denied", entries: []windows.EXPLICIT_ACCESS{
			owner, entry(sid(t, windows.WinWorldSid), windows.DENY_ACCESS, windows.FILE_WRITE_DATA),
		}},
		{name: "Everyone may read", entries: []windows.EXPLICIT_ACCESS{
			owner, entry(sid(t, windows.WinWorldSid), windows.SET_ACCESS, read),
		}, shared: true},
		{name: "Users may read", entries: []windows.EXPLICIT_ACCESS{
			owner, entry(sid(t, windows.WinBuiltinUsersSid), windows.SET_ACCESS, read),
		}, shared: true},
		{name: "Authenticated Users may read", entries: []windows.EXPLICIT_ACCESS{
			owner, entry(sid(t, windows.WinAuthenticatedUserSid), windows.SET_ACCESS, read),
		}, shared: true},
		{name: "null DACL", null: true, shared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "secret")
			if err := os.WriteFile(path, []byte("x"), 0o600); err != nil {
				t.Fatal(err)
			}
			switch {
			case tt.null:
				setDACL(t, path, nil)
			case tt.entries != nil:
				setDACL(t, path, tt.entries)
			}
			f, err := os.Open(path)
			if err != nil {
				t.Fatal(err)
			}
			defer f.Close()
			err = Check(f)
			if got := errors.Is(err, ErrShared); got != tt.shared || (!tt.shared && err != nil) {
				t.Fatalf("Check = %v, want shared %v", err, tt.shared)
			}
		})
	}
}

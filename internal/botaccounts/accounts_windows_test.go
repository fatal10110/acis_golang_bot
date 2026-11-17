package botaccounts

import (
	"errors"
	"strings"
	"testing"

	"golang.org/x/sys/windows"
)

// fileAllAccess is FILE_ALL_ACCESS, which x/sys/windows does not name.
const fileAllAccess = windows.STANDARD_RIGHTS_REQUIRED | windows.SYNCHRONIZE | 0x1ff

// TestLoadSecretACL: Windows ignores mode bits, so a secret file whose DACL
// lets Users or Everyone read it is refused, and an owner-only one loads.
func TestLoadSecretACL(t *testing.T) {
	good := genSecret(t)
	u, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		t.Fatal(err)
	}
	tests := []struct {
		name   string
		reader windows.WELL_KNOWN_SID_TYPE // granted read beside the owner; 0 for none
		shared bool
	}{
		{name: "owner only"},
		{name: "Users may read", reader: windows.WinBuiltinUsersSid, shared: true},
		{name: "Everyone may read", reader: windows.WinWorldSid, shared: true},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			path := writeSecret(t, good, 0o600)
			entries := []windows.EXPLICIT_ACCESS{grant(u.User.Sid, fileAllAccess)}
			if tt.reader != 0 {
				sid, err := windows.CreateWellKnownSid(tt.reader)
				if err != nil {
					t.Fatal(err)
				}
				entries = append(entries, grant(sid, windows.FILE_GENERIC_READ))
			}
			acl, err := windows.ACLFromEntries(entries, nil)
			if err != nil {
				t.Fatal(err)
			}
			// Protected: the file stops inheriting its directory's entries.
			info := windows.SECURITY_INFORMATION(windows.DACL_SECURITY_INFORMATION | windows.PROTECTED_DACL_SECURITY_INFORMATION)
			if err := windows.SetNamedSecurityInfo(path, windows.SE_FILE_OBJECT, info, nil, nil, acl, nil); err != nil {
				t.Fatal(err)
			}
			s, err := LoadSecret(path)
			if tt.shared {
				if !errors.Is(err, ErrSecretMode) {
					t.Fatalf("err = %v, want ErrSecretMode", err)
				}
				if strings.Contains(err.Error(), good) {
					t.Fatal("error quotes the secret")
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if string(s.key) != good {
				t.Fatal("loaded secret differs from the file's")
			}
		})
	}
}

func grant(sid *windows.SID, mask windows.ACCESS_MASK) windows.EXPLICIT_ACCESS {
	return windows.EXPLICIT_ACCESS{
		AccessPermissions: mask,
		AccessMode:        windows.SET_ACCESS,
		Inheritance:       windows.NO_INHERITANCE,
		Trustee: windows.TRUSTEE{
			TrusteeForm:  windows.TRUSTEE_IS_SID,
			TrusteeValue: windows.TrusteeValueFromSID(sid),
		},
	}
}

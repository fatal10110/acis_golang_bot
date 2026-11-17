//go:build windows

package ownerfile

import (
	"fmt"
	"os"
	"unsafe"

	"golang.org/x/sys/windows"
)

// Check returns nil when the DACL of f, an open file, allows access to no
// one but the file's owner, the current user, SYSTEM and Administrators;
// otherwise an error wrapping ErrShared that names the first other
// principal. A null DACL allows everyone. Deny entries only narrow access, an
// inherit-only entry does not apply to the file, and an entry of any other
// type is refused rather than guessed at. The error never carries the file's
// content.
func Check(f *os.File) error {
	sd, err := windows.GetSecurityInfo(windows.Handle(f.Fd()), windows.SE_FILE_OBJECT,
		windows.OWNER_SECURITY_INFORMATION|windows.DACL_SECURITY_INFORMATION)
	if err != nil {
		return fmt.Errorf("read the file's security descriptor: %w", err)
	}
	owner, _, err := sd.Owner()
	if err != nil {
		return fmt.Errorf("read the file's owner: %w", err)
	}
	dacl, _, err := sd.DACL()
	if err != nil {
		return fmt.Errorf("read the file's DACL: %w", err)
	}
	if dacl == nil {
		return fmt.Errorf("null DACL allows everyone: %w", ErrShared)
	}
	trusted, err := trustedSIDs(owner)
	if err != nil {
		return err
	}
	for i := range uint32(dacl.AceCount) {
		var ace *windows.ACCESS_ALLOWED_ACE
		if err := windows.GetAce(dacl, i, &ace); err != nil {
			return fmt.Errorf("read the file's DACL entry %d: %w", i, err)
		}
		if ace.Header.AceFlags&windows.INHERIT_ONLY_ACE != 0 {
			continue
		}
		switch ace.Header.AceType {
		case windows.ACCESS_DENIED_ACE_TYPE:
			continue
		case windows.ACCESS_ALLOWED_ACE_TYPE:
		default:
			return fmt.Errorf("DACL entry %d has type %d, want allow or deny entries only: %w", i, ace.Header.AceType, ErrShared)
		}
		sid := (*windows.SID)(unsafe.Pointer(&ace.SidStart))
		if ace.Mask == 0 || isTrusted(sid, trusted) {
			continue
		}
		return fmt.Errorf("DACL allows %s, want only the owner, SYSTEM and Administrators: %w", sid, ErrShared)
	}
	return nil
}

// trustedSIDs are the principals a private file may allow: its owner, the
// user running the process, SYSTEM and the Administrators group.
func trustedSIDs(owner *windows.SID) ([]*windows.SID, error) {
	user, err := windows.GetCurrentProcessToken().GetTokenUser()
	if err != nil {
		return nil, fmt.Errorf("read the process user: %w", err)
	}
	trusted := []*windows.SID{owner, user.User.Sid}
	for _, k := range []windows.WELL_KNOWN_SID_TYPE{windows.WinLocalSystemSid, windows.WinBuiltinAdministratorsSid} {
		sid, err := windows.CreateWellKnownSid(k)
		if err != nil {
			return nil, fmt.Errorf("well-known SID %d: %w", k, err)
		}
		trusted = append(trusted, sid)
	}
	return trusted, nil
}

func isTrusted(sid *windows.SID, trusted []*windows.SID) bool {
	for _, t := range trusted {
		if t != nil && sid.Equals(t) {
			return true
		}
	}
	return false
}

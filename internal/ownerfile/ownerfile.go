// Package ownerfile checks that a file holding a secret is private to its
// owner before it is read: on unix no group or other mode bit, on Windows a
// DACL that allows no one but the owner, the current user, SYSTEM and
// Administrators. Windows ignores mode bits, so the guarantee there comes
// from the ACL, not from skipping the check.
package ownerfile

import "errors"

// ErrShared reports a file others than its owner may access.
var ErrShared = errors.New("file is accessible to others than its owner")

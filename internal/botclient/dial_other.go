//go:build !linux

package botclient

import "errors"

// lineSockopts is empty off Linux: TCP_USER_TIMEOUT is Linux's, and the
// keepalive alone bounds an idle dead line.
var lineSockopts []sockopt

func setsockopt(fd uintptr, level, name, value int) error {
	return errors.ErrUnsupported
}

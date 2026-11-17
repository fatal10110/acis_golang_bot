package botclient

import "syscall"

// userTimeout matches the keepalive's give-up time.
const userTimeout = keepAliveIdle + keepAliveCount*keepAliveInterval

// tcpUserTimeout is TCP_USER_TIMEOUT (include/uapi/linux/tcp.h), which the
// syscall package does not name: how long sent data may stay unacknowledged
// before the kernel ends the connection with ETIMEDOUT.
const tcpUserTimeout = 0x12

// lineSockopts are the socket options of every dial beyond the keepalive.
var lineSockopts = []sockopt{
	{level: syscall.IPPROTO_TCP, name: tcpUserTimeout, value: int(userTimeout.Milliseconds())},
}

func setsockopt(fd uintptr, level, name, value int) error {
	return syscall.SetsockoptInt(int(fd), level, name, value)
}

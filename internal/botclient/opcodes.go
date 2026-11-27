package botclient

// Opcodes of the login packets this package reads and sends, redeclared from
// the acis_golang server's packet packages (the file each value comes from is
// named in its group). The game opcodes are botwire's (forwards.go). Login
// and game opcodes overlap, so each constant names its protocol and
// direction.

// Login server to client (internal/loginserver/network/serverpackets).
const (
	opLoginInit          = 0x00 // init.go
	opLoginFail          = 0x01 // loginfail.go
	opLoginAccountKicked = 0x02 // accountkicked.go
	opLoginOk            = 0x03 // loginok.go
	opLoginServerList    = 0x04 // serverlist.go
	opLoginPlayFail      = 0x06 // playfail.go
	opLoginPlayOk        = 0x07 // playok.go
	opLoginGGAuth        = 0x0b // ggauth.go
)

// Client to login server (internal/loginserver/network/clientpackets).
const (
	opLoginRequestAuthLogin   = 0x00 // requestauthlogin.go
	opLoginRequestServerLogin = 0x02 // requestserverlogin.go
	opLoginRequestServerList  = 0x05 // requestserverlist.go
	opLoginAuthGameGuard      = 0x07 // authgameguard.go
)

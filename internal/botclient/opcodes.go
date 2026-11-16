package botclient

// Opcodes of the packets this package reads and sends, redeclared from the
// acis_golang server's packet packages (the file each value comes from is
// named in its group). Login and game opcodes overlap, so each constant
// names its protocol and direction.

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

// Game server to client (internal/gameserver/network/serverpackets).
const (
	opGameVersionCheck   = 0x00 // versioncheck.go
	opGameUserInfo       = 0x04 // userinfo.go
	opGameCharSelectInfo = 0x13 // charselectinfo.go
	opGameAuthLoginFail  = 0x14 // authloginfail.go
	opGameCharSelected   = 0x15 // charselected.go
	opGameCharCreateOk   = 0x19 // charcreateok.go
	opGameCharCreateFail = 0x1a // charcreatefail.go
	opGameActionFailed   = 0x25 // actionfailed.go
	opGameServerClose    = 0x26 // serverclose.go
	opGameLeaveWorld     = 0x7e // lifecycle.go
)

// Client to game server (internal/gameserver/network/clientpackets).
const (
	opGameProtocolVersion        = 0x00 // protocolversion.go
	opGameEnterWorld             = 0x03 // enterworld.go
	opGameAuthLogin              = 0x08 // authlogin.go
	opGameLogout                 = 0x09 // lifecycle.go
	opGameRequestCharacterCreate = 0x0b // requestcharactercreate.go
	opGameRequestGameStart       = 0x0d // requestgamestart.go
)

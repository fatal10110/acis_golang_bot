package e2e

import "encoding/binary"

// RequestPledgePower (clientpackets/pledge.go OpcodeRequestPledgePower)
// reads a rank's privileges, or sets them with Action
// pledgePowerSetPrivileges (pledgePowerSetPrivilegesAction). The bot never
// sends it (docs/crafting.md D17): a scripted clan leader of a scenario
// does, to give its members' rank the warehouse privilege.
const (
	opRequestPledgePower     byte  = 0xc0
	pledgePowerSetPrivileges int32 = 2
)

// Clan ranks and privileges a scenario's leader sets (clan/clan.go
// MemberPowerGrade, clan/privilege.go): a member joins the main clan at
// rank 6, and privWarehouseSearch lets it withdraw from the clan warehouse
// while MembersCanWithdrawFromClanWH is set (clan/warehouse.go CanWithdraw).
const (
	memberRank          int32 = 6
	privWarehouseSearch int32 = 8
)

// encodeRequestPledgePower sets rank's privileges to privs: Rank, Action
// and Privs, each an int32 little-endian after the opcode, the privileges
// read only when Action sets them (clientpackets/pledge.go
// DecodeRequestPledgePower). Only the clan leader's set takes effect, for
// ranks 1 to 9 (clan/rank.go SetRankPrivileges); a set that changes them
// refreshes every online member's clan window (network/clan_view.go
// broadcastClanStatus), one that does not is unanswered.
func encodeRequestPledgePower(rank, privs int32) []byte {
	b := make([]byte, 1, 13)
	b[0] = opRequestPledgePower
	for _, v := range []int32{rank, pledgePowerSetPrivileges, privs} {
		b = binary.LittleEndian.AppendUint32(b, uint32(v))
	}
	return b
}

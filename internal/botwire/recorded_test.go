package botwire

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"testing"
)

// recordedMustDecode pins the recorded opcodes whose decoder must run: a
// decoder dropped later, or a fixture deleted, fails the test instead of
// passing silently.
var recordedMustDecode = []byte{
	OpGameMoveToLocation, OpGameUserInfo, OpGameStatusUpdate, OpGameDeleteObject,
	OpGameNPCInfo, OpGameActionFailed, OpGameChangeWaitType,
	OpGameSystemMessage, OpGameLeaveWorld, OpGameSkillList, OpGameSkillCoolTime, OpGameItemList,
	OpGameAskJoinPledge, OpGameJoinPledge, OpGamePledgeShowMemberListAll,
	OpGamePledgeShowMemberListDeleteAll, OpGameWarehouseDepositList, OpGameWarehouseWithdrawList,
	OpGameRecipeBookItemList, OpGameRecipeItemMakeInfo,
}

// TestRecordedFixtures replays the -record captures checked in under
// testdata/recorded: every payload with a decoder must decode with exactly
// len(payload) consumed and carry a plausible object id. Ids, coordinates
// and times vary between runs and are not pinned. A payload with no decoder
// yet is skipped, not passed. CharSelectInfo, which the handshake reads,
// is botclient's TestRecordedCharSelectInfo.
func TestRecordedFixtures(t *testing.T) {
	paths, err := filepath.Glob(filepath.Join("testdata", "recorded", "*.bin"))
	if err != nil {
		t.Fatal(err)
	}
	if len(paths) == 0 {
		t.Skip("no recorded fixtures yet: they need a live server (#5)")
	}
	pinned := map[byte]bool{}
	for _, op := range recordedMustDecode {
		pinned[op] = true
	}
	decoded := map[byte]bool{}
	for _, path := range paths {
		t.Run(filepath.Base(path), func(t *testing.T) {
			raw, err := os.ReadFile(path)
			if err != nil {
				t.Fatal(err)
			}
			if len(raw) == 0 || filepath.Base(path) != fmt.Sprintf("%02x.bin", raw[0]) {
				t.Fatalf("fixture name does not match its opcode")
			}
			ev, ok, err := DecodeEvent(raw)
			if err != nil {
				t.Fatal(err)
			}
			if !ok {
				if pinned[raw[0]] {
					t.Fatalf("opcode %#02x is pinned but has no decoder", raw[0])
				}
				t.Skipf("no decoder for opcode %#02x yet", raw[0])
			}
			decoded[raw[0]] = true
			if oid := reflect.ValueOf(ev).FieldByName("ObjectID"); oid.IsValid() && oid.Int() == 0 {
				t.Errorf("%T has object id 0", ev)
			}
		})
	}
	for _, op := range recordedMustDecode {
		if !decoded[op] {
			t.Errorf("pinned opcode %#02x was not decoded from a fixture", op)
		}
	}
}

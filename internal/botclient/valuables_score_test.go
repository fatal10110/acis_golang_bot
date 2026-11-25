package botclient

import (
	"math"
	"testing"
	"time"

	"github.com/fatal10110/acis_golang_bot/internal/datapack"
)

// The score fixture: scoreLoot (priced scoreRef) drops from scoreMob and
// scoreEscort, 0.2 a kill each (one DROP category at 100%, the item at 10%,
// 1 to 3); scoreSpoiler spoils it, scoreHerb and scoreCoin drop it as HERB and
// CURRENCY, scoreLeader and scoreBlank drop nothing. One scoreMob at
// respawn 60 s, KillFloor 60 s, is 30 kills an hour: S 6.
const (
	scoreLoot    int32 = 5000
	scoreRef     int32 = 50000
	scoreMob     int32 = 20100
	scoreEscort  int32 = 20101
	scoreLeader  int32 = 20102
	scoreSpoiler int32 = 20103
	scoreBlank   int32 = 20104
	scoreHerb    int32 = 20105
	scoreCoin    int32 = 20106
	scoreBoss    int32 = 25001
	scoreTrader  int32 = 30001
)

func scoreDrop(kind datapack.DropKind) []datapack.DropCategory {
	return []datapack.DropCategory{{Kind: kind, Chance: 100, Drops: []datapack.Drop{{ItemID: scoreLoot, Min: 1, Max: 3, Chance: 10}}}}
}

func scoreNPCs() map[int32]datapack.NPC {
	return map[int32]datapack.NPC{
		scoreMob:     {ID: scoreMob, Drops: scoreDrop(datapack.DropKindDrop)},
		scoreEscort:  {ID: scoreEscort, Drops: scoreDrop(datapack.DropKindDrop)},
		scoreSpoiler: {ID: scoreSpoiler, Drops: scoreDrop(datapack.DropKindSpoil)},
		scoreHerb:    {ID: scoreHerb, Drops: scoreDrop(datapack.DropKindHerb)},
		scoreCoin:    {ID: scoreCoin, Drops: scoreDrop(datapack.DropKindCurrency)},
		scoreLeader:  {ID: scoreLeader},
		scoreBlank:   {ID: scoreBlank},
		scoreBoss:    {ID: scoreBoss, Type: "RaidBoss", Drops: scoreDrop(datapack.DropKindDrop)},
		scoreTrader:  {ID: scoreTrader, Type: "Merchant"},
	}
}

func scoreSpawn(npc int32, total int, respawn time.Duration) datapack.Spawn {
	return datapack.Spawn{NPCID: npc, Total: total, Respawn: respawn}
}

func scoreMaker(name, ai string, params map[string]string, spawns ...datapack.Spawn) *datapack.Maker {
	return &datapack.Maker{Name: name, AIType: ai, Params: params, Spawns: spawns}
}

func scoreDatapack(makers ...*datapack.Maker) *datapack.Datapack {
	return &datapack.Datapack{
		NPCs:      scoreNPCs(),
		Prices:    map[int32]int32{scoreLoot: scoreRef},
		Spawnlist: &datapack.Spawnlist{Makers: makers},
	}
}

func TestValueScoreSupply(t *testing.T) {
	const s = 6.0
	mob := func(total int, respawn time.Duration) datapack.Spawn { return scoreSpawn(scoreMob, total, respawn) }
	plain := func(sp ...datapack.Spawn) *datapack.Maker { return scoreMaker("m", "default_maker", nil, sp...) }
	evented := func(ev string) *datapack.Maker { mk := plain(mob(1, time.Minute)); mk.Event = ev; return mk }
	gated := func(ev string) *datapack.Maker {
		return scoreMaker("m", "event_maker", map[string]string{"EventName": ev}, mob(1, time.Minute))
	}
	withPrivates := func(sp datapack.Spawn, rows ...datapack.Private) datapack.Spawn { sp.Privates = rows; return sp }
	escort := func(weight int, respawn time.Duration) datapack.Private {
		return datapack.Private{NPCID: scoreEscort, Weight: weight, Respawn: respawn}
	}
	tests := []struct {
		name   string
		makers []*datapack.Maker
		// npcs edits the NPC templates; cfg the supply config.
		npcs func(map[int32]datapack.NPC)
		cfg  func(*SupplyConfig)
		want float64
	}{
		{name: "default maker", makers: []*datapack.Maker{plain(mob(1, time.Minute))}, want: s},
		{name: "total 2 twice", makers: []*datapack.Maker{plain(mob(2, time.Minute))}, want: 2 * s},
		{name: "respawnRand no change", makers: []*datapack.Maker{plain(datapack.Spawn{NPCID: scoreMob, Total: 1, Respawn: time.Minute, RespawnRandom: 30 * time.Second})}, want: s},
		{name: "respawn 0sec once a day", makers: []*datapack.Maker{plain(mob(1, 0))}, want: 0.2 / 24},
		{name: "respawn no once a day", makers: []*datapack.Maker{plain(mob(1, datapack.NoRespawn))}, want: 0.2 / 24},
		{name: "raid boss like any spawn", makers: []*datapack.Maker{plain(scoreSpawn(scoreBoss, 1, 35*time.Hour))}, want: 0.2 * 3600 / (35*3600 + 60)},
		{name: "spoil 0.2", makers: []*datapack.Maker{plain(scoreSpawn(scoreSpoiler, 1, time.Minute))}, want: 0.2 * s},
		{name: "herb nothing", makers: []*datapack.Maker{plain(scoreSpawn(scoreHerb, 1, time.Minute))}, want: 0},
		{name: "currency nothing", makers: []*datapack.Maker{plain(scoreSpawn(scoreCoin, 1, time.Minute))}, want: 0},
		{name: "changed kill floor", makers: []*datapack.Maker{plain(mob(1, time.Minute))}, cfg: func(c *SupplyConfig) { c.KillFloor = 2 * time.Minute }, want: 4},
		{name: "changed no respawn cycle", makers: []*datapack.Maker{plain(mob(1, 0))}, cfg: func(c *SupplyConfig) { c.NoRespawnCycle = time.Hour }, want: 0.2},
		{name: "changed spoil weight", makers: []*datapack.Maker{plain(scoreSpawn(scoreSpoiler, 1, time.Minute))}, cfg: func(c *SupplyConfig) { c.SpoilWeight = 0.5 }, want: 0.5 * s},

		{name: "spawn event", makers: []*datapack.Maker{evented("extra_mob")}, want: s},
		{name: "seven signs event", makers: []*datapack.Maker{evented("ssq_event")}, want: s / 2},
		{name: "seal 1 no owner", makers: []*datapack.Maker{evented("ssq_seal1_none")}, want: s / 2},
		{name: "seal 2 no owner", makers: []*datapack.Maker{evented("ssq_seal2_none")}, want: s / 2},
		{name: "changed seven signs share", makers: []*datapack.Maker{evented("ssq_event")}, cfg: func(c *SupplyConfig) { c.SevenSignsShare = 0.25 }, want: s / 4},
		{name: "seal dawn nothing", makers: []*datapack.Maker{evented("ssq_seal1_dawn")}, want: 0},
		{name: "seal twilight nothing", makers: []*datapack.Maker{evented("ssq_seal2_twilight")}, want: 0},
		{name: "medal nothing", makers: []*datapack.Maker{evented("medal")}, want: 0},
		{name: "event maker gated by EventName", makers: []*datapack.Maker{gated("extra_mob")}, want: s},
		{name: "event maker christmas nothing", makers: []*datapack.Maker{gated("christmas")}, want: 0},
		{name: "changed list drops extra_mob", makers: []*datapack.Maker{gated("extra_mob")}, cfg: func(c *SupplyConfig) { c.SpawnEvents = []string{"christmas"} }, want: 0},
		{name: "changed list takes christmas", makers: []*datapack.Maker{gated("christmas")}, cfg: func(c *SupplyConfig) { c.SpawnEvents = []string{"christmas"} }, want: s},

		{name: "spawnTime nothing", makers: []*datapack.Maker{{Name: "m", SpawnTime: "siege_warfare_start(8)", Spawns: []datapack.Spawn{mob(1, time.Minute)}}}, want: 0},
		{name: "on_start_spawn 0 nothing", makers: []*datapack.Maker{scoreMaker("m", "default_maker", map[string]string{"on_start_spawn": "0"}, mob(1, time.Minute))}, want: 0},
		{name: "on_start_spawn 1 fully", makers: []*datapack.Maker{scoreMaker("m", "default_maker", map[string]string{"on_start_spawn": "1"}, mob(1, time.Minute))}, want: s},
		{name: "no_on_start_maker nothing", makers: []*datapack.Maker{scoreMaker("m", "no_on_start_maker", nil, mob(1, time.Minute))}, want: 0},
		{name: "random_spawn a third each", makers: []*datapack.Maker{scoreMaker("m", "random_spawn", nil, mob(1, time.Minute), scoreSpawn(scoreBlank, 1, time.Minute), scoreSpawn(scoreBlank, 1, time.Minute))}, want: s / 3},
		{name: "random_spawn_pawn a third each", makers: []*datapack.Maker{scoreMaker("m", "random_spawn_pawn", nil, mob(1, time.Minute), scoreSpawn(scoreBlank, 1, time.Minute), scoreSpawn(scoreBlank, 1, time.Minute))}, want: s / 3},
		{name: "night 0.25", makers: []*datapack.Maker{scoreMaker("m", "on_day_night_spawn", map[string]string{"IsNight": "1"}, mob(1, time.Minute))}, want: 0.25 * s},
		{name: "day 0.75", makers: []*datapack.Maker{scoreMaker("m", "on_day_night_spawn", map[string]string{"IsNight": "0"}, mob(1, time.Minute))}, want: 0.75 * s},
		// A group of three back whole every 3 × KillFloor, whatever its
		// respawn: 20 kills an hour each.
		{name: "instant group of three", makers: []*datapack.Maker{scoreMaker("m", "maker_instant_spawn", nil, mob(3, 17*time.Minute))}, want: 3 * 20 * 0.2},
		// A linked pair with one starter: half each, every respawn +
		// KillFloor × 1 = 120 s.
		{name: "instant random pair one starter", makers: []*datapack.Maker{
			scoreMaker("a", "maker_instant_spawn_random", map[string]string{"on_start_spawn": "1", "maker_name1": "a", "maker_name2": "b"}, mob(1, time.Minute)),
			scoreMaker("b", "maker_instant_spawn_random", map[string]string{"on_start_spawn": "0", "maker_name1": "a", "maker_name2": "b"}, mob(1, time.Minute)),
		}, want: s},
		{name: "instant random pair, the off one alone", makers: []*datapack.Maker{
			scoreMaker("a", "maker_instant_spawn_random", map[string]string{"on_start_spawn": "1", "maker_name1": "a", "maker_name2": "b"}, scoreSpawn(scoreBlank, 1, time.Minute)),
			scoreMaker("b", "maker_instant_spawn_random", map[string]string{"on_start_spawn": "0", "maker_name1": "a", "maker_name2": "b"}, mob(1, time.Minute)),
		}, want: s / 2},
		{name: "instant random cycle respawn + KillFloor × N", makers: []*datapack.Maker{
			scoreMaker("a", "maker_instant_spawn_random", map[string]string{"maker_name1": "a"}, mob(2, 2*time.Minute)),
		}, want: 2 * 15 * 0.2},
		{name: "unnamed own-maker type like default", makers: []*datapack.Maker{scoreMaker("m", "random_spawn_treasurebox", nil, mob(1, time.Minute))}, want: s},

		{name: "private at its leader's cycle", makers: []*datapack.Maker{plain(withPrivates(scoreSpawn(scoreLeader, 1, time.Minute), escort(1, 0)))}, want: s},
		{name: "private its shorter own cycle", makers: []*datapack.Maker{plain(withPrivates(scoreSpawn(scoreLeader, 1, 3*time.Minute), escort(1, 30*time.Second)))}, want: 40 * 0.2},
		{name: "private own cycle longer", makers: []*datapack.Maker{plain(withPrivates(scoreSpawn(scoreLeader, 1, time.Minute), escort(1, 10*time.Minute)))}, want: s},
		{name: "private under total 2 twice", makers: []*datapack.Maker{plain(withPrivates(scoreSpawn(scoreLeader, 2, time.Minute), escort(1, 0)))}, want: 2 * s},
		{name: "private weight 2 still once", makers: []*datapack.Maker{plain(withPrivates(scoreSpawn(scoreLeader, 1, time.Minute), escort(2, 0)))}, want: s},
		{name: "private weight 0 still once", makers: []*datapack.Maker{plain(withPrivates(scoreSpawn(scoreLeader, 1, time.Minute), escort(0, 0)))}, want: s},
		{name: "private takes its leader's share", makers: []*datapack.Maker{func() *datapack.Maker {
			mk := plain(withPrivates(scoreSpawn(scoreLeader, 1, time.Minute), escort(1, 0)))
			mk.Event = "ssq_event"
			return mk
		}()}, want: s / 2},
		{
			name: "no rows takes the template's", makers: []*datapack.Maker{plain(scoreSpawn(scoreLeader, 1, time.Minute))},
			npcs: func(n map[int32]datapack.NPC) {
				l := n[scoreLeader]
				l.Privates = []datapack.Private{escort(1, 0)}
				n[scoreLeader] = l
			}, want: s,
		},
		{
			name: "own rows replace the template's", makers: []*datapack.Maker{plain(withPrivates(scoreSpawn(scoreLeader, 1, time.Minute), datapack.Private{NPCID: scoreBlank, Weight: 1}))},
			npcs: func(n map[int32]datapack.NPC) {
				l := n[scoreLeader]
				l.Privates = []datapack.Private{escort(1, 0)}
				n[scoreLeader] = l
			}, want: 0,
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dp := scoreDatapack(tt.makers...)
			if tt.npcs != nil {
				tt.npcs(dp.NPCs)
			}
			cfg := DefaultSupplyConfig()
			if tt.cfg != nil {
				tt.cfg(&cfg)
			}
			got := NewTravelMap(dp, cfg).supply[scoreLoot].perHour
			if math.Abs(got-tt.want) > 1e-9 {
				t.Fatalf("S = %v, want %v", got, tt.want)
			}
		})
	}
}

// scoreMap is a travel map whose scoreLoot has S 6 (R log10(100/6), 1.22 at
// the defaults) and whose other templates have their own source or mark.
func scoreMap(t *testing.T, edit func(dp *datapack.Datapack)) *TravelMap {
	t.Helper()
	dp := scoreDatapack(scoreMaker("m", "default_maker", nil, scoreSpawn(scoreMob, 1, time.Minute)))
	if edit != nil {
		edit(dp)
	}
	return NewTravelMap(dp, DefaultSupplyConfig())
}

func TestValueScore(t *testing.T) {
	const (
		sword   int32 = 6000 // a D-grade weapon
		plate   int32 = 6001 // a D-grade full body armor
		ring    int32 = 6002 // no grade
		bought  int32 = 6003 // a spawned trader's multisell makes it of adena
		upgrade int32 = 6004 // a spawned trader's multisell makes it of a sword
		bowl    int32 = 6005 // a recipe makes it
	)
	baseR := math.Log10(100.0 / 6)
	edit := func(dp *datapack.Datapack) {
		dp.Items = map[int32]datapack.Item{
			sword: {ID: sword, Type: "Weapon", Grade: 1, BodyPart: "rhand"},
			plate: {ID: plate, Type: "Armor", Grade: 1, BodyPart: "fullarmor"},
			ring:  {ID: ring, Type: "Armor", BodyPart: "rfinger;lfinger"},
		}
		for id, ref := range map[int32]int32{
			sword: 50000, plate: 50000, ring: 50000, bought: 50000, upgrade: 50000, bowl: 50000,
			955: 5000, 956: 1000, 6575: 90000, 957: 80000, // D scrolls: weapon, armor, blessed, crystal
		} {
			dp.Prices[id] = ref
		}
		drops := []datapack.Drop{}
		for _, id := range []int32{scoreLoot, sword, plate, ring, upgrade, bowl} {
			drops = append(drops, datapack.Drop{ItemID: id, Min: 1, Max: 3, Chance: 10})
		}
		mob := dp.NPCs[scoreMob]
		mob.Drops = []datapack.DropCategory{{Kind: datapack.DropKindDrop, Chance: 100, Drops: drops}}
		dp.NPCs[scoreMob] = mob
		dp.Spawnlist.Makers = append(dp.Spawnlist.Makers, scoreMaker("trader", "default_maker", nil,
			datapack.Spawn{NPCID: scoreTrader, Total: 1, Positions: []datapack.Position{{X: 1, Y: 2, Z: 3}}}))
		dp.Multisells = []datapack.Multisell{{Name: "trade", NPCs: []int32{scoreTrader}, Rows: []datapack.MultisellRow{
			{Ingredients: []datapack.MultisellItem{{ItemID: adenaID, Count: 1000}}, Products: []datapack.MultisellItem{{ItemID: bought, Count: 1}, {ItemID: sword, Count: 1}}},
			{Ingredients: []datapack.MultisellItem{{ItemID: plate, Count: 1}}, Products: []datapack.MultisellItem{{ItemID: upgrade, Count: 1}}},
		}}}
		dp.Recipes = map[int32]datapack.Recipe{1: {ID: 1, Product: datapack.Ingredient{ItemID: bowl, Count: 1}}}
	}
	tests := []struct {
		name    string
		edit    func(*datapack.Datapack)
		id      int32
		enchant int
		wantR   float64
		wantW   float64
	}{
		{name: "drop source", id: scoreLoot, wantR: baseR, wantW: 50000},
		{name: "no source R 0", id: bought, edit: func(dp *datapack.Datapack) { dp.Multisells = nil }, wantR: 0, wantW: 50000},
		{name: "shop-sold no loot", id: scoreLoot, edit: func(dp *datapack.Datapack) {
			dp.BuyLists = []datapack.BuyList{{ID: 1, NPCID: scoreTrader, Products: []datapack.Product{{ItemID: scoreLoot, Price: 1, Count: -1}}}}
		}, wantR: 0, wantW: 50000},
		{name: "npcId -1 list not counted", id: scoreLoot, edit: func(dp *datapack.Datapack) {
			dp.BuyLists = []datapack.BuyList{{ID: 1, NPCID: -1, Products: []datapack.Product{{ItemID: scoreLoot, Price: 1, Count: -1}}}}
		}, wantR: baseR, wantW: 50000},
		{name: "multisell of no gear R 0", id: bought, wantR: 0, wantW: 50000},
		{name: "multisell gear product R 0", id: sword, wantR: 0, wantW: 50000},
		{name: "upgrade row no source", id: upgrade, wantR: baseR, wantW: 50000},
		{name: "unspawned trader's multisell no mark", id: sword, edit: func(dp *datapack.Datapack) {
			dp.Spawnlist.Makers = dp.Spawnlist.Makers[:1]
		}, wantR: baseR, wantW: 50000},
		{name: "recipe halves R", id: bowl, wantR: baseR / 2, wantW: 50000},
		{name: "+3 full body armor safe", id: plate, enchant: 3, wantR: baseR, wantW: 50000 + 3*1000},
		{name: "+5 graded weapon", id: sword, enchant: 5, edit: func(dp *datapack.Datapack) { dp.Multisells = nil }, wantR: baseR + 0.5, wantW: 50000 + 5*5000},
		{name: "+5 full body armor", id: plate, enchant: 5, wantR: baseR + 0.25, wantW: 50000 + 5*1000},
		{name: "+5 no grade no price", id: ring, enchant: 5, wantR: baseR + 0.5, wantW: 50000},
		{name: "raid-only drop rare", id: scoreLoot, edit: func(dp *datapack.Datapack) {
			dp.Spawnlist.Makers = []*datapack.Maker{scoreMaker("boss", "default_maker", nil, scoreSpawn(scoreBoss, 1, 35*time.Hour))}
		}, wantR: 4, wantW: 50000},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			m := scoreMap(t, func(dp *datapack.Datapack) {
				edit(dp)
				if tt.edit != nil {
					tt.edit(dp)
				}
			})
			sc := m.valueScore(defaultValuablePolicy(), tt.id, tt.enchant)
			if math.Abs(sc.r-tt.wantR) > 1e-9 || sc.w != tt.wantW {
				t.Fatalf("R %v W %v, want R %v W %v", sc.r, sc.w, tt.wantR, tt.wantW)
			}
			if want := tt.wantW * math.Exp2(tt.wantR); math.Abs(sc.v-want) > 1e-6 {
				t.Fatalf("V %v, want %v", sc.v, want)
			}
		})
	}
}

func TestValueScoreThresholds(t *testing.T) {
	const cheap, rich int32 = 7000, 7001
	m := scoreMap(t, func(dp *datapack.Datapack) {
		dp.Prices[cheap], dp.Prices[rich] = 9999, 60000
	})
	// scoreLoot: ref 50000, R 1.22 at CommonSupply 100: V 116700.
	def := defaultValuablePolicy()
	scarce := def
	scarce.CommonSupply = 10 // R 0.22: V 58400
	picky := def
	picky.ValueMin = 200000
	tests := []struct {
		name string
		p    ValuablePolicy
		id   int32
		want bool
	}{
		{name: "both thresholds met", p: def, id: scoreLoot, want: true},
		{name: "zero policy finds none", p: ValuablePolicy{}, id: scoreLoot},
		{name: "unpriced none", p: def, id: 4242},
		{name: "under PriceFloor", p: ValuablePolicy{PriceFloor: 10000, CommonSupply: 100, RMax: 4, ValueMin: 1}, id: cheap},
		{name: "no source under ValueMin", p: def, id: cheap},
		{name: "no source over ValueMin by price", p: ValuablePolicy{PriceFloor: 10000, ValueMin: 60000, CommonSupply: 100, RMax: 4}, id: rich, want: true},
		{name: "other persona's CommonSupply", p: scarce, id: scoreLoot},
		{name: "other persona's ValueMin", p: picky, id: scoreLoot},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := tt.p.valuable(m.valueScore(tt.p, tt.id, 0)); got != tt.want {
				t.Fatalf("valuable = %v, want %v", got, tt.want)
			}
		})
	}
	if a, b := m.valueScore(def, scoreLoot, 0).v, m.valueScore(scarce, scoreLoot, 0).v; a == b {
		t.Fatalf("two CommonSupply on one map score %v both", a)
	}
}

func TestRestartRegion(t *testing.T) {
	// The server's fixture (model/restart/restart_core_test.go TestPointAt):
	// talking_island_town lists tile 17;25, and its spawn coordinate maps
	// back to it; the world origin (tile 20;18) is in no region.
	m := NewTravelMap(&datapack.Datapack{Spawnlist: &datapack.Spawnlist{}, RestartPoints: []datapack.RestartPoint{
		{Name: "gludio_castle_town", Tiles: []datapack.MapTile{{X: 19, Y: 21}}},
		{Name: "talking_island_town", Tiles: []datapack.MapTile{{X: 15, Y: 23}, {X: 17, Y: 25}}},
		{Name: "shadow", Tiles: []datapack.MapTile{{X: 17, Y: 25}}},
	}}, DefaultSupplyConfig())
	tests := []struct {
		name string
		x, y int32
		want int
	}{
		{name: "talking island town", x: -83990, y: 243336, want: 1},
		{name: "tile corner", x: -131072 + 32768, y: -262144 + 15*32768, want: 1},
		{name: "last unit of a tile", x: -131072 + 2*32768 - 1, y: -262144 + 16*32768 - 1, want: 1},
		{name: "world origin no region", x: 0, y: 0, want: -1},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := m.restartRegion(tt.x, tt.y); got != tt.want {
				t.Fatalf("restartRegion(%d, %d) = %d, want %d", tt.x, tt.y, got, tt.want)
			}
		})
	}
	if got := (&TravelMap{}).restartRegion(-83990, 243336); got != -1 {
		t.Fatalf("no restart points: region %d, want -1", got)
	}
}

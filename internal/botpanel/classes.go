package botpanel

import (
	"cmp"
	"slices"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// classNames are the profession names by class id, as the server names
// them (acis_golang internal/gameserver/model/actor/player/class.go
// classInfos): the panel shows a bot's class by name.
var classNames = map[int32]string{
	0: "Human Fighter", 1: "Warrior", 2: "Gladiator", 3: "Warlord", 4: "Human Knight", 5: "Paladin",
	6: "Dark Avenger", 7: "Rogue", 8: "Treasure Hunter", 9: "Hawkeye",
	10: "Human Mystic", 11: "Human Wizard", 12: "Sorcerer", 13: "Necromancer", 14: "Warlock", 15: "Cleric",
	16: "Bishop", 17: "Prophet",
	18: "Elven Fighter", 19: "Elven Knight", 20: "Temple Knight", 21: "Sword Singer", 22: "Elven Scout",
	23: "Plains Walker", 24: "Silver Ranger",
	25: "Elven Mystic", 26: "Elven Wizard", 27: "Spellsinger", 28: "Elemental Summoner", 29: "Elven Oracle",
	30: "Elven Elder",
	31: "Dark Fighter", 32: "Palus Knight", 33: "Shillien Knight", 34: "Bladedancer", 35: "Assassin",
	36: "Abyss Walker", 37: "Phantom Ranger",
	38: "Dark Mystic", 39: "Dark Wizard", 40: "Spellhowler", 41: "Phantom Summoner", 42: "Shillien Oracle",
	43: "Shillien Elder",
	44: "Orc Fighter", 45: "Orc Raider", 46: "Destroyer", 47: "Monk", 48: "Tyrant",
	49: "Orc Mystic", 50: "Orc Shaman", 51: "Overlord", 52: "Warcryer",
	53: "Dwarven Fighter", 54: "Scavenger", 55: "Bounty Hunter", 56: "Artisan", 57: "Warsmith",

	88: "Duelist", 89: "Dreadnought", 90: "Phoenix Knight", 91: "Hell Knight", 92: "Sagittarius", 93: "Adventurer",
	94: "Archmage", 95: "Soultaker", 96: "Arcana Lord", 97: "Cardinal", 98: "Hierophant",
	99: "Eva's Templar", 100: "Sword Muse", 101: "Wind Rider", 102: "Moonlight Sentinel", 103: "Mystic Muse",
	104: "Elemental Master", 105: "Eva's Saint",
	106: "Shillien Templar", 107: "Spectral Dancer", 108: "Ghost Hunter", 109: "Ghost Sentinel",
	110: "Storm Screamer", 111: "Spectral Master", 112: "Shillien Saint",
	113: "Titan", 114: "Grand Khavatari", 115: "Dominator", 116: "Doom Cryer",
	117: "Fortune Seeker", 118: "Maestro",
}

// classView is a class in the API; Starting marks the nine a new bot may be
// created with, and Branches are the first professions a bot of a base
// class may be pinned to (botclient.BranchPins, POST
// /api/bots/{account}/branch).
type classView struct {
	ID       int32   `json:"id"`
	Name     string  `json:"name"`
	Starting bool    `json:"starting"`
	Branches []int32 `json:"branches,omitempty"`
}

// classList returns every class by id.
func classList() []classView {
	starting := botclient.StartingClasses()
	out := make([]classView, 0, len(classNames))
	for id, name := range classNames {
		isStart := slices.ContainsFunc(starting, func(c botclient.StartingClass) bool { return c.ID == id })
		out = append(out, classView{ID: id, Name: name, Starting: isStart, Branches: botclient.BranchPins(id)})
	}
	slices.SortFunc(out, func(a, b classView) int { return cmp.Compare(a.ID, b.ID) })
	return out
}

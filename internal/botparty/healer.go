package botparty

// Class ids of the healer lines (data/xml/classes; botclient trainer.go).
const (
	classHumanMystic    int32 = 10
	classCleric         int32 = 15
	classBishop         int32 = 16
	classProphet        int32 = 17
	classElvenMystic    int32 = 25
	classElvenOracle    int32 = 29
	classElvenElder     int32 = 30
	classDarkMystic     int32 = 38
	classShillienOracle int32 = 42
	classShillienElder  int32 = 43
	classOrcShaman      int32 = 50
	classWarcryer       int32 = 52
)

// Levels the base classes' heals come at: 1011 Heal at 7 for the human, elven and dark mystic,
// 1229 Chant of Life at 20 for the Orc Shaman (composition.md, "Healer").
const (
	mysticHealLevel = 7
	shamanHealLevel = 20
)

// healerClass reports a class and level whose row heals: a human, elven or dark mystic from 7, a
// Cleric, Elven Oracle or Shillien Oracle, an Orc Shaman from 20, and the professions after them
// (Bishop, Prophet, Elven Elder, Shillien Elder, Warcryer: learned skills stay). Never a level 1-6
// mystic nor an orc mystic (1090 Life Drain heals only itself).
func healerClass(classID, level int32) bool {
	switch classID {
	case classHumanMystic, classElvenMystic, classDarkMystic:
		return level >= mysticHealLevel
	case classCleric, classBishop, classProphet, classElvenOracle, classElvenElder,
		classShillienOracle, classShillienElder:
		return true
	case classOrcShaman, classWarcryer:
		return level >= shamanHealLevel
	}
	return false
}

// healer reports a Support candidate with the healer flag whose run knows its heal.
func (c *Candidate) healer() bool {
	return c.Preset == PresetSupport && c.Heals && healerClass(c.ClassID, c.Level)
}

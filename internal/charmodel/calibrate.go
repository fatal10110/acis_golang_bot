package charmodel

// Calibration corrects the model by what the server reports: per stat, the
// ratio of the reported value to the modeled one of the current build. A
// buff, an enchantment or anything else the model does not know scales the
// reported stats alike, so it carries over to a candidate build and cancels
// out of the difference the candidate makes.
type Calibration struct {
	ratio Profile
}

// Calibrate is the calibration of the modeled current profile by the
// reported one. A stat either side lacks (0) keeps a ratio of 1.
func Calibrate(modeled, reported Profile) Calibration {
	var c Calibration
	r, m, rep := c.ratio.fields(), modeled.fields(), reported.fields()
	for i := range r {
		*r[i] = 1
		if *m[i] > 0 && *rep[i] > 0 {
			*r[i] = *rep[i] / *m[i]
		}
	}
	return c
}

// Apply scales a modeled profile by c. The zero Calibration changes
// nothing.
func (c Calibration) Apply(p Profile) Profile {
	r, f := c.ratio.fields(), p.fields()
	for i := range f {
		if *r[i] != 0 {
			*f[i] *= *r[i]
		}
	}
	return p
}

// fields are p's stats, in a fixed order.
func (p *Profile) fields() []*float64 {
	return []*float64{
		&p.MaxHP, &p.MaxMP, &p.PAtk, &p.MAtk, &p.PAtkSpd, &p.MAtkSpd, &p.Critical,
		&p.Accuracy, &p.Evasion, &p.PDef, &p.MDef, &p.RunSpeed, &p.WeightLimit,
	}
}

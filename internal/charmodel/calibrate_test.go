package charmodel

import (
	"math"
	"testing"
)

// TestCalibrate checks that a reported stat 10 % above the model scales the
// prediction for a candidate by the same ratio, and that a stat either side
// lacks keeps the model's value.
func TestCalibrate(t *testing.T) {
	modeled := Profile{MaxHP: 500, PAtk: 100, MAtk: 50}
	reported := Profile{PAtk: 110, MAtk: 50}
	candidate := Profile{MaxHP: 600, PAtk: 200, MAtk: 80}
	got := Calibrate(modeled, reported).Apply(candidate)
	for _, tt := range []struct {
		name      string
		got, want float64
	}{
		{"PAtk", got.PAtk, 220},
		{"MAtk", got.MAtk, 80},
		{"MaxHP", got.MaxHP, 600},
		{"damage", PhysicalDamage(got.PAtk, 77, false) / PhysicalDamage(candidate.PAtk, 77, false), 1.1},
	} {
		if math.Abs(tt.got-tt.want) > 1e-9 {
			t.Errorf("%s = %v, want %v", tt.name, tt.got, tt.want)
		}
	}
	if got := (Calibration{}).Apply(candidate); got != candidate {
		t.Errorf("zero Calibration Apply = %+v, want %+v", got, candidate)
	}
}

// TestCalibrateCancelsBuff checks that a buff the model does not know (a
// 20 % P.Atk Might) carries over to the candidate build: the calibrated
// candidate is the buffed candidate.
func TestCalibrateCancelsBuff(t *testing.T) {
	cur := Model(Build{Class: humanFighter, Level: 20})
	reported := cur
	reported.PAtk *= 1.2
	cand := Model(Build{Class: humanFighter, Level: 20, Dyes: Attrs{STR: 5}})
	got := Calibrate(cur, reported).Apply(cand)
	if math.Abs(got.PAtk-cand.PAtk*1.2) > 1e-9 || got.PDef != cand.PDef {
		t.Errorf("calibrated PAtk, PDef = %v %v, want %v %v", got.PAtk, got.PDef, cand.PAtk*1.2, cand.PDef)
	}
}

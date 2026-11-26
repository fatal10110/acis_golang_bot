package e2e

import "testing"

func TestKarmaReset(t *testing.T) {
	tests := []struct {
		name, answer string
		ok           bool
	}{
		{"e2emint1Bot", "You successfully set e2emint1Bot's karma to 0.", true},
		{"e2emint1Bot", "You successfully set E2Emint1bot's karma to 0.", true},
		// With a target the server sets the target's karma, not the GM's.
		{"e2emint1Bot", "You successfully set e2epvh1Bot's karma to 0.", false},
		{"e2emint1Bot", "You successfully set e2emint1Bot's karma to 5.", false},
		{"e2emint1Bot", "Usage: //set karma <number>", false},
		{"e2emint1Bot", "The karma value must be greater or equal to 0.", false},
	}
	for _, tt := range tests {
		if err := karmaReset(tt.name, tt.answer); (err == nil) != tt.ok {
			t.Errorf("karmaReset(%q, %q) = %v, want ok %v", tt.name, tt.answer, err, tt.ok)
		}
	}
}

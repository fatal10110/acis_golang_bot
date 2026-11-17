package botnames

import "fmt"

// String is the placement's flag and file spelling: "suffix" or "prefix".
func (p Placement) String() string {
	switch p {
	case Suffix:
		return "suffix"
	case Prefix:
		return "prefix"
	}
	return fmt.Sprintf("Placement(%d)", uint8(p))
}

// ParsePlacement reads a placement spelled as String spells it.
func ParsePlacement(s string) (Placement, error) {
	switch s {
	case "suffix":
		return Suffix, nil
	case "prefix":
		return Prefix, nil
	}
	return 0, fmt.Errorf("name marker placement %q: want suffix or prefix", s)
}

// MarshalText spells the placement as String does; it refuses an unknown one.
func (p Placement) MarshalText() ([]byte, error) {
	if p != Suffix && p != Prefix {
		return nil, fmt.Errorf("name marker placement %d: want prefix or suffix", uint8(p))
	}
	return []byte(p.String()), nil
}

// UnmarshalText reads a placement as ParsePlacement does.
func (p *Placement) UnmarshalText(b []byte) error {
	v, err := ParsePlacement(string(b))
	if err != nil {
		return err
	}
	*p = v
	return nil
}

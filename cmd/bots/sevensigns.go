package main

import (
	"flag"
	"fmt"
	"strings"

	"github.com/fatal10110/acis_golang_bot/internal/botclient"
)

// presetPvP is the pvp preset's name, the one preset Seven Signs member
// mode applies to (docs/pvp/cp-potions.md, D43).
const presetPvP = "pvp"

// sevenSignsFlags are the flags of Seven Signs member mode.
var sevenSignsFlags = []string{"sevensigns-members", "sevensigns-max-members", "signs-seal"}

// addSevenSignsFlags adds the Seven Signs member mode flags to fs: the
// member names, read by checkSevenSignsFlags from the returned value, the
// cap on them into p.MaxMembers and the seal into p.Seal.
func addSevenSignsFlags(fs *flag.FlagSet, p *botclient.SevenSignsPolicy) *string {
	members := fs.String("sevensigns-members", "", "comma-separated character names of the -persona pvp bots that play Seven Signs member mode: sign up with Dusk, farm the catacombs and necropolises, keep their seal stones; default: none")
	fs.IntVar(&p.MaxMembers, "sevensigns-max-members", botclient.DefaultSignsMembers, "the most names -sevensigns-members may list: a longer list is refused")
	fs.Func("signs-seal", "the seal a Seven Signs member votes when Dusk holds none: avarice or gnosis (default gnosis; strife is refused)", func(v string) error {
		switch strings.ToLower(v) {
		case "avarice":
			p.Seal = botclient.SealAvarice
		case "gnosis":
			p.Seal = botclient.SealGnosis
		case "strife":
			return fmt.Errorf("a Seven Signs member never votes the Seal of Strife")
		default:
			return fmt.Errorf("want avarice or gnosis, not %q", v)
		}
		return nil
	})
	return members
}

// checkSevenSignsFlags parses the member names into p and validates the
// mode: its flags apply to -persona pvp only, and the list holds at most
// -sevensigns-max-members names (SevenSignsPolicy.Check).
func checkSevenSignsFlags(fs *flag.FlagSet, persona, members string, p *botclient.SevenSignsPolicy) error {
	if persona != presetPvP {
		for _, name := range sevenSignsFlags {
			if flagSet(fs, name) {
				return fmt.Errorf("-%s applies to -persona %s, not %s", name, presetPvP, persona)
			}
		}
	}
	for _, n := range strings.Split(members, ",") {
		if n = strings.TrimSpace(n); n != "" {
			p.Members = append(p.Members, n)
		}
	}
	if err := p.Check(); err != nil {
		return fmt.Errorf("-sevensigns-members: %w", err)
	}
	return nil
}

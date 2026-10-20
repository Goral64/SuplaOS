package system

import "testing"

func TestParseUpgradable(t *testing.T) {
	out := `Listing...
suplaos/trixie 0.2.0 arm64 [upgradable from: 0.1.0]
linux-image-rpi-v8/trixie 1:6.18.51-1+rpt1 arm64 [upgradable from: 1:6.18.50-1+rpt1]
`
	got := parseUpgradable(out)
	if len(got) != 2 || got[0] != (Package{"suplaos", "0.1.0", "0.2.0"}) || got[1].From != "1:6.18.50-1+rpt1" {
		t.Errorf("parseUpgradable = %+v", got)
	}
}

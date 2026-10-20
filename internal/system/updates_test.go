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

func TestNewerKernel(t *testing.T) {
	installed := []string{"6.18.50+rpt-rpi-v8", "6.18.51+rpt-rpi-2712", "6.18.50+rpt-rpi-2712"}
	if newerKernel("6.18.50+rpt-rpi-v8", installed) {
		t.Error("Pi 5 kernel counted for Pi 4")
	}
	if !newerKernel("6.18.50+rpt-rpi-2712", installed) {
		t.Error("newer Pi 5 kernel not found")
	}
	if !newerKernel("6.12.48+deb13-amd64", []string{"6.12.48+deb13-amd64", "6.12.57+deb13-amd64"}) {
		t.Error("newer Debian kernel not found")
	}
	if newerKernel("6.12.57+deb13-amd64", []string{"6.12.48+deb13-amd64", "6.12.57+deb13-amd64"}) {
		t.Error("older kernel counted")
	}
}

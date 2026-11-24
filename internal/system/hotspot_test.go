package system

import (
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
)

func TestHotspotPassword(t *testing.T) {
	dir := t.TempDir()
	hotspotPasswordFile = filepath.Join(dir, "lib", "hotspot-password")
	if p, _ := hotspotPassword(HotspotSettings{}); p != HotspotPassword {
		t.Errorf("before the wizard: %q", p)
	}
	p, err := hotspotPassword(HotspotSettings{SetupDone: true})
	if err != nil || !regexp.MustCompile(`^[a-km-np-z2-9]{4}-[a-km-np-z2-9]{4}-[a-km-np-z2-9]{4}$`).MatchString(p) {
		t.Fatalf("device password %q %v", p, err)
	}
	if fi, err := os.Stat(hotspotPasswordFile); err != nil || fi.Mode().Perm() != 0o600 {
		t.Errorf("file %v %v", fi, err)
	}
	if again, _ := DeviceHotspotPassword(); again != p {
		t.Errorf("not kept: %q %q", again, p)
	}
	if n, _ := NewHotspotPassword(); n == p {
		t.Error("new password is the old one")
	}
}

func TestHotspotIssue(t *testing.T) {
	hotspotIssueFile = filepath.Join(t.TempDir(), "issue.d", "suplaos-hotspot.issue")
	text := hotspotIssue(HotspotSettings{SetupDone: true}, "abcd-efgh-ijkm")
	if !strings.Contains(text, "SuplaOS") || !strings.Contains(text, "password abcd-efgh-ijkm") {
		t.Errorf("text %q", text)
	}
	writeHotspotIssue(text)
	if data, _ := os.ReadFile(hotspotIssueFile); string(data) != text {
		t.Errorf("written %q", data)
	}
	if hotspotIssue(HotspotSettings{SetupDone: true, Off: true}, "x") != "" ||
		hotspotIssue(HotspotSettings{Off: true}, HotspotPassword) == "" {
		t.Error("off only after the wizard")
	}
	writeHotspotIssue("")
	if _, err := os.Stat(hotspotIssueFile); !os.IsNotExist(err) {
		t.Error("not removed")
	}
	writeHotspotIssue("") // nothing to remove
}

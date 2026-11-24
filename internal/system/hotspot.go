package system

import (
	"crypto/rand"
	"errors"
	"fmt"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

// The hotspot: when the device has no network (no cable, no Wi-Fi it
// knows in range), it makes its own Wi-Fi network, so that a phone can
// open the panel and pick the Wi-Fi to join. A phone that connects is
// sent to the panel (dnsmasq answers every name with HotspotAddress).
// Until the first run wizard is done its password is HotspotPassword, from
// the instructions; then the one of the device, made at random.
const (
	HotspotConnection = "suplaos-hotspot"
	HotspotPassword   = "suplaos-setup"
	HotspotAddress    = "192.168.4.1"
)

// The password of the device, and the console text that shows it.
var (
	hotspotPasswordFile = "/var/lib/suplaos/hotspot-password"
	hotspotIssueFile    = "/etc/issue.d/suplaos-hotspot.issue"
)

// DeviceHotspotPassword is the password of the hotspot of the device,
// made the first time it is needed.
func DeviceHotspotPassword() (string, error) {
	data, err := os.ReadFile(hotspotPasswordFile)
	if err == nil {
		if p := strings.TrimSpace(string(data)); len(p) >= 8 {
			return p, nil
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return "", err
	}
	return NewHotspotPassword()
}

// NewHotspotPassword makes a new password of the device: three groups of
// four letters and digits that are easy to tell apart, about 60 bits.
func NewHotspotPassword() (string, error) {
	const chars = "abcdefghijkmnpqrstuvwxyz23456789"
	b := make([]byte, 12)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	var p strings.Builder
	for i, c := range b {
		if i > 0 && i%4 == 0 {
			p.WriteByte('-')
		}
		p.WriteByte(chars[int(c)%len(chars)])
	}
	if err := os.MkdirAll(filepath.Dir(hotspotPasswordFile), 0o755); err != nil {
		return "", err
	}
	if err := os.WriteFile(hotspotPasswordFile, []byte(p.String()+"\n"), 0o600); err != nil {
		return "", err
	}
	return p.String(), nil
}

// HotspotSettings are the choices of the panel: whether the first run
// wizard is done (until then the hotspot is always on, with
// HotspotPassword) and whether the hotspot is turned off.
type HotspotSettings struct {
	SetupDone bool
	Off       bool
}

// hotspotPassword is the password the hotspot starts with now.
func hotspotPassword(set HotspotSettings) (string, error) {
	if !set.SetupDone {
		return HotspotPassword, nil
	}
	return DeviceHotspotPassword()
}

// hotspotIssue is the console text with the name and the password of the
// hotspot, empty when it is off.
func hotspotIssue(set HotspotSettings, password string) string {
	if set.SetupDone && set.Off {
		return ""
	}
	return fmt.Sprintf("  Wi-Fi without a network: %s, password %s\n\n", HotspotSSID(), password)
}

// writeHotspotIssue keeps the console text up to date.
func writeHotspotIssue(text string) {
	old, _ := os.ReadFile(hotspotIssueFile)
	if string(old) == text {
		return
	}
	var err error
	if text == "" {
		err = os.Remove(hotspotIssueFile)
	} else if err = os.MkdirAll(filepath.Dir(hotspotIssueFile), 0o755); err == nil {
		err = os.WriteFile(hotspotIssueFile, []byte(text), 0o644)
	}
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		log.Printf("hotspot: %v", err)
	}
}

// How long the device waits for a network before the hotspot goes on, and
// how long the hotspot stays on with nobody connected before the device
// looks for its networks again (a router that was off at the start).
var (
	hotspotWait = 2 * time.Minute
	hotspotIdle = 5 * time.Minute
)

// The networks in range, scanned before the hotspot went on: a card that
// is an access point does not scan.
var (
	scanMu     sync.Mutex
	scanBefore []WifiNetwork
)

// HotspotSSID is the name of the hotspot of the device: SuplaOS and the
// end of the machine ID, which stays the same (NetworkManager changes the
// MAC address of the card when it scans).
func HotspotSSID() string {
	id, _ := os.ReadFile("/etc/machine-id")
	m := strings.ToUpper(strings.TrimSpace(string(id)))
	if len(m) < 4 {
		return "SuplaOS"
	}
	return "SuplaOS-" + m[len(m)-4:]
}

// online tells whether a cable or Wi-Fi connection other than the hotspot
// is up or coming up.
func online() bool {
	rows, err := nmcliFields("-f", "NAME,TYPE,STATE", "connection", "show", "--active")
	if err != nil {
		return true // nothing to do while NetworkManager does not answer
	}
	return onlineIn(rows)
}

func onlineIn(rows [][]string) bool {
	for _, r := range rows {
		if len(r) == 3 && r[0] != HotspotConnection &&
			(r[1] == "802-3-ethernet" || r[1] == "802-11-wireless") &&
			(r[2] == "activated" || r[2] == "activating") {
			return true
		}
	}
	return false
}

func hotspotActive() bool {
	rows, err := nmcliFields("-f", "NAME", "connection", "show", "--active")
	if err != nil {
		return false
	}
	for _, r := range rows {
		if len(r) > 0 && r[0] == HotspotConnection {
			return true
		}
	}
	return false
}

// hotspotClients is the number of phones connected to the hotspot.
func hotspotClients(device string) int {
	out, err := exec.Command("iw", "dev", device, "station", "dump").Output()
	if err != nil {
		return 0
	}
	return strings.Count(string(out), "Station ")
}

// startHotspot scans for the networks in range first, then makes the
// device an access point.
func startHotspot(device, password string) error {
	if list, err := ScanWifi(device); err == nil {
		scanMu.Lock()
		scanBefore = list
		scanMu.Unlock()
	}
	nmcliC("connection", "delete", "id", HotspotConnection).Run()
	if err := run("nmcli", "connection", "add", "type", "wifi", "ifname", device,
		"con-name", HotspotConnection, "autoconnect", "no", "ssid", HotspotSSID(),
		"802-11-wireless.mode", "ap", "802-11-wireless.band", "bg",
		"ipv4.method", "shared", "ipv4.addresses", HotspotAddress+"/24", "ipv6.method", "disabled",
		"wifi-sec.key-mgmt", "wpa-psk", "wifi-sec.proto", "rsn", "wifi-sec.pairwise", "ccmp",
		"wifi-sec.group", "ccmp", "wifi-sec.psk", password); err != nil {
		return err
	}
	return run("nmcli", "--wait", "30", "connection", "up", "id", HotspotConnection)
}

func stopHotspot() error {
	return run("nmcli", "connection", "down", "id", HotspotConnection)
}

// leaveHotspot turns the hotspot off before the card joins a network: an
// access point does not see the networks around. The watcher turns it on
// again if joining fails.
func leaveHotspot(device string) {
	if hotspotActive() {
		stopHotspot()
		nmcliC("device", "wifi", "list", "ifname", device, "--rescan", "yes").Run()
	}
}

// WatchHotspot turns the hotspot on when the device has had no network for
// a while, and off when a network is back, nobody uses it or it is turned
// off. settings gives the choices of the panel.
func WatchHotspot(settings func() HotspotSettings) {
	// Left from before a restart: the device looks for its networks first.
	if hotspotActive() {
		stopHotspot()
	}
	var offline, started time.Time
	for ; ; time.Sleep(15 * time.Second) {
		w, err := ReadWifi()
		if err != nil || w.Device == "" {
			continue
		}
		set := settings()
		password, err := hotspotPassword(set)
		if err != nil {
			log.Printf("hotspot: %v", err)
			continue
		}
		writeHotspotIssue(hotspotIssue(set, password))
		off := set.SetupDone && set.Off
		now := time.Now()
		if w.Hotspot != "" {
			switch {
			case off:
				log.Printf("hotspot: turned off")
				stopHotspot()
			case online():
				log.Printf("hotspot: network is back, off")
				stopHotspot()
			case hotspotClients(w.Device) == 0 && now.Sub(started) >= hotspotIdle:
				log.Printf("hotspot: nobody connected, looking for networks")
				stopHotspot()
				offline = now
			}
			continue
		}
		if off || online() {
			offline = time.Time{}
			continue
		}
		if offline.IsZero() || offline.Before(started) {
			// Just offline, or the hotspot was turned off from outside
			// (a Wi-Fi joined in the panel): wait again.
			offline = now
			continue
		}
		if now.Sub(offline) < hotspotWait {
			continue
		}
		log.Printf("hotspot: no network for %s, on as %s", hotspotWait, HotspotSSID())
		if err := startHotspot(w.Device, password); err != nil {
			log.Printf("hotspot: %v", err)
		}
		started = time.Now()
	}
}

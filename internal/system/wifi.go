package system

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"sort"
	"strconv"
	"strings"
)

// Wifi is the state of the Wi-Fi of the device.
type Wifi struct {
	// Device is the first Wi-Fi interface, empty when there is none (no
	// card, or no firmware or wpa_supplicant for it).
	Device string `json:"device"`
	// State of the device as NetworkManager shows it: connected,
	// disconnected, unavailable...
	State   string `json:"state"`
	Enabled bool   `json:"enabled"`
	// Connection is the active connection, Address its IPv4 address.
	Connection string `json:"connection"`
	Address    string `json:"address"`
	// Saved are the Wi-Fi connections NetworkManager remembers, without
	// access points (hotspots).
	Saved []SavedWifi `json:"saved"`
}

// SavedWifi is a remembered Wi-Fi connection.
type SavedWifi struct {
	Name string `json:"name"`
	// Autoconnect: joined on its own when in range, also after a restart.
	Autoconnect bool `json:"autoconnect"`
}

// WifiNetwork is a network found by a scan.
type WifiNetwork struct {
	SSID     string `json:"ssid"`
	Signal   int    `json:"signal"`
	Security string `json:"security"`
	InUse    bool   `json:"inUse"`
}

// nmcliC is nmcli with messages and states in English, whatever the
// language of the system, so that they can be recognized.
func nmcliC(args ...string) *exec.Cmd {
	cmd := exec.Command("nmcli", args...)
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	return cmd
}

// nmcliFields runs nmcli in terse mode with escaping and splits every line
// of the output into fields.
func nmcliFields(args ...string) ([][]string, error) {
	out, err := nmcliC(append([]string{"-t", "-e", "yes"}, args...)...).Output()
	if err != nil {
		var ee *exec.ExitError
		if errors.As(err, &ee) {
			return nil, fmt.Errorf("nmcli: %s", strings.TrimSpace(string(ee.Stderr)))
		}
		return nil, err
	}
	return parseTerse(string(out)), nil
}

// parseTerse splits terse nmcli output: fields separated by ':', with ':'
// and '\' inside a field escaped by '\'.
func parseTerse(out string) [][]string {
	var rows [][]string
	for _, line := range strings.Split(out, "\n") {
		if line == "" {
			continue
		}
		var fields []string
		var f strings.Builder
		for i := 0; i < len(line); i++ {
			switch c := line[i]; {
			case c == '\\' && i+1 < len(line):
				i++
				f.WriteByte(line[i])
			case c == ':':
				fields = append(fields, f.String())
				f.Reset()
			default:
				f.WriteByte(c)
			}
		}
		rows = append(rows, append(fields, f.String()))
	}
	return rows
}

// ReadWifi returns the state of the first Wi-Fi interface.
func ReadWifi() (*Wifi, error) {
	devs, err := nmcliFields("-f", "DEVICE,TYPE,STATE,CONNECTION", "device")
	if err != nil {
		return nil, err
	}
	w := &Wifi{Saved: []SavedWifi{}}
	for _, d := range devs {
		if len(d) == 4 && d[1] == "wifi" && !accessPoint(d[3]) {
			w.Device, w.State, w.Connection = d[0], d[2], d[3]
			break
		}
	}
	if w.Device == "" {
		return w, nil
	}
	if radio, err := nmcliFields("radio", "wifi"); err == nil && len(radio) > 0 {
		w.Enabled = radio[0][0] == "enabled"
	}
	if w.Connection != "" {
		if m, err := nmcli("-f", "IP4.ADDRESS", "device", "show", w.Device); err == nil {
			w.Address, _, _ = strings.Cut(m["IP4.ADDRESS"], ",")
		}
	}
	conns, err := nmcliFields("-f", "NAME,TYPE,AUTOCONNECT", "connection", "show")
	if err != nil {
		return nil, err
	}
	for _, c := range conns {
		if len(c) == 3 && c[1] == "802-11-wireless" && !accessPoint(c[0]) {
			w.Saved = append(w.Saved, SavedWifi{Name: c[0], Autoconnect: c[2] == "yes"})
		}
	}
	sort.Slice(w.Saved, func(i, j int) bool { return w.Saved[i].Name < w.Saved[j].Name })
	return w, nil
}

// accessPoint tells whether the connection makes the device an access
// point (a hotspot): such a card is not the one the device connects with.
func accessPoint(conn string) bool {
	if conn == "" {
		return false
	}
	out, err := nmcliC("-g", "802-11-wireless.mode", "connection", "show", "id", conn).Output()
	return err == nil && strings.TrimSpace(string(out)) == "ap"
}

// ScanWifi lists the networks in range, the strongest first, each SSID
// once. Hidden networks are left out.
func ScanWifi(device string) ([]WifiNetwork, error) {
	nmcliC("radio", "wifi", "on").Run()
	rows, err := nmcliFields("-f", "IN-USE,SSID,SIGNAL,SECURITY", "device", "wifi", "list", "ifname", device, "--rescan", "yes")
	if err != nil {
		return nil, err
	}
	return parseScan(rows), nil
}

func parseScan(rows [][]string) []WifiNetwork {
	best := map[string]WifiNetwork{}
	for _, r := range rows {
		if len(r) != 4 || r[1] == "" {
			continue
		}
		n := WifiNetwork{SSID: r[1], Security: r[3], InUse: r[0] == "*"}
		n.Signal, _ = strconv.Atoi(r[2])
		if n.Security == "--" {
			n.Security = ""
		}
		if b, ok := best[n.SSID]; ok {
			n.InUse = n.InUse || b.InUse
			if b.Signal > n.Signal {
				n.Signal, n.Security = b.Signal, b.Security
			}
		}
		best[n.SSID] = n
	}
	list := make([]WifiNetwork, 0, len(best))
	for _, n := range best {
		list = append(list, n)
	}
	sort.Slice(list, func(i, j int) bool {
		if list[i].Signal != list[j].Signal {
			return list[i].Signal > list[j].Signal
		}
		return list[i].SSID < list[j].SSID
	})
	return list
}

// WifiConnect is a request to join a network.
type WifiConnect struct {
	SSID     string `json:"ssid"`
	Password string `json:"password"`
	Hidden   bool   `json:"hidden"`
	// Autoconnect, on when not given.
	Autoconnect *bool `json:"autoconnect"`
}

// Validate checks the SSID and the WPA password (none for an open network).
func (c *WifiConnect) Validate() error {
	if c.SSID == "" || len(c.SSID) > 32 {
		return errors.New("the network name has to be 1 to 32 characters")
	}
	if c.Password != "" && (len(c.Password) < 8 || len(c.Password) > 64) {
		return errors.New("the Wi-Fi password has to be 8 to 63 characters")
	}
	return nil
}

// ConnectWifi joins a network and waits until the device has an address.
// NetworkManager saves the connection, named after the network, and joins
// it again after a restart.
func ConnectWifi(device string, c *WifiConnect) error {
	if err := c.Validate(); err != nil {
		return err
	}
	if out, err := nmcliC("radio", "wifi", "on").CombinedOutput(); err != nil {
		return fmt.Errorf("nmcli: %s", strings.TrimSpace(string(out)))
	}
	// nmcli fails on a saved connection of the same name ("key-mgmt:
	// property is missing"), and the password is entered anew anyway.
	nmcliC("connection", "delete", "id", c.SSID).Run()
	args := []string{"--wait", "45", "device", "wifi", "connect", c.SSID, "ifname", device}
	if c.Password != "" {
		args = append(args, "password", c.Password)
	}
	if c.Hidden {
		args = append(args, "hidden", "yes")
	}
	out, err := nmcliC(args...).CombinedOutput()
	if err != nil && strings.Contains(string(out), "No network with SSID") {
		// The list of networks NetworkManager keeps may be old: scan and
		// try once more.
		nmcliC("device", "wifi", "list", "ifname", device, "--rescan", "yes").Run()
		out, err = nmcliC(args...).CombinedOutput()
	}
	if err != nil {
		// The failed connection stays saved and NetworkManager would keep
		// trying it.
		nmcliC("connection", "delete", "id", c.SSID).Run()
		msg := strings.TrimSpace(string(out))
		// A wrong password often shows as a network not found.
		if strings.Contains(msg, "Secrets were required") || strings.Contains(msg, "could not be found") {
			return fmt.Errorf("could not connect to %s: wrong password, or the network is out of range", c.SSID)
		}
		return fmt.Errorf("nmcli: %s", msg)
	}
	if c.Autoconnect != nil && !*c.Autoconnect {
		return SetWifiAutoconnect(c.SSID, false)
	}
	return nil
}

// savedWifi checks that name is a saved Wi-Fi connection.
func savedWifi(name string) error {
	w, err := ReadWifi()
	if err != nil {
		return err
	}
	for _, s := range w.Saved {
		if s.Name == name {
			return nil
		}
	}
	return fmt.Errorf("no saved Wi-Fi network %q", name)
}

// ConnectSavedWifi joins a saved network with its saved password.
func ConnectSavedWifi(device, name string) error {
	if err := savedWifi(name); err != nil {
		return err
	}
	nmcliC("radio", "wifi", "on").Run()
	out, err := nmcliC("--wait", "45", "connection", "up", "id", name, "ifname", device).CombinedOutput()
	if err != nil {
		return fmt.Errorf("could not connect to %s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// DisconnectWifi disconnects the Wi-Fi card. NetworkManager joins no
// network with it on its own until a connect or the next restart.
func DisconnectWifi(device string) error {
	return run("nmcli", "device", "disconnect", device)
}

// SetWifiAutoconnect turns joining a saved network on its own on or off.
func SetWifiAutoconnect(name string, on bool) error {
	if err := savedWifi(name); err != nil {
		return err
	}
	v := "no"
	if on {
		v = "yes"
	}
	return run("nmcli", "connection", "modify", "id", name, "connection.autoconnect", v)
}

// ForgetWifi deletes a saved Wi-Fi connection, disconnecting it first.
func ForgetWifi(name string) error {
	if err := savedWifi(name); err != nil {
		return err
	}
	return run("nmcli", "connection", "delete", "id", name)
}

// zoneTab maps time zones to countries (tzdata).
var zoneTab = "/usr/share/zoneinfo/zone.tab"

// CountryOf returns the country code of a time zone, "" when there is
// none (UTC).
func CountryOf(tz string) string {
	data, err := os.ReadFile(zoneTab)
	if err != nil {
		return ""
	}
	for _, line := range strings.Split(string(data), "\n") {
		f := strings.Fields(line)
		if len(f) >= 3 && !strings.HasPrefix(f[0], "#") && f[2] == tz {
			return f[0]
		}
	}
	return ""
}

// wifiCountryFile sets the country when cfg80211 loads as a module (x86-64);
// the panel sets it again at every start.
var wifiCountryFile = "/etc/modprobe.d/suplaos-wifi-country.conf"

// SetWifiCountry sets the country whose radio rules Wi-Fi follows. Without
// it the card keeps to the rules valid everywhere: no 5 GHz channels on
// some cards, and their drivers complain on the console.
func SetWifiCountry(country string) error {
	if len(country) != 2 {
		return nil
	}
	line := "options cfg80211 ieee80211_regdom=" + country + "\n"
	if data, err := os.ReadFile(wifiCountryFile); err != nil || string(data) != line {
		if err := os.WriteFile(wifiCountryFile, []byte(line), 0o644); err != nil {
			return err
		}
	}
	// The database may have come after cfg80211 started (wireless-regdb
	// installed by an update), which reads it only once; built into the
	// kernel (Raspberry Pi) it does not see the modprobe option either.
	run("iw", "reg", "reload")
	return run("iw", "reg", "set", country)
}

// ApplyWifiCountry sets the country of the time zone of the system.
func ApplyWifiCountry() error {
	return SetWifiCountry(CountryOf(Timezone()))
}

// Package system reads and changes the settings of the SuplaOS host:
// password of the supla account, time zone, host name, network, power.
package system

import (
	"bufio"
	"bytes"
	"crypto/subtle"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"syscall"
	"time"

	"github.com/openwall/yescrypt-go"
)

// User is the account of SuplaOS, used for SSH and the panel.
const User = "supla"

// DefaultPassword is the password the image comes with.
const DefaultPassword = "supla"

// Info is the state of the host shown on the dashboard.
type Info struct {
	Hostname    string   `json:"hostname"`
	Version     string   `json:"version"`
	Addresses   []string `json:"addresses"`
	Uptime      int64    `json:"uptime"`
	Load        []string `json:"load"`
	CPUs        int      `json:"cpus"`
	MemTotal    uint64   `json:"memTotal"`
	MemFree     uint64   `json:"memFree"`
	DiskTotal   uint64   `json:"diskTotal"`
	DiskFree    uint64   `json:"diskFree"`
	Temperature *float64 `json:"temperature"`
	Timezone    string   `json:"timezone"`
	Time        string   `json:"time"`
}

// ReadInfo collects Info; dataDir is the disk shown.
func ReadInfo(dataDir string) Info {
	var i Info
	i.Hostname, _ = os.Hostname()
	i.Version = Version()
	i.Addresses = Addresses()
	if data, err := os.ReadFile("/proc/uptime"); err == nil {
		f := strings.Fields(string(data))
		if len(f) > 0 {
			up, _ := strconv.ParseFloat(f[0], 64)
			i.Uptime = int64(up)
		}
	}
	if data, err := os.ReadFile("/proc/loadavg"); err == nil {
		f := strings.Fields(string(data))
		if len(f) >= 3 {
			i.Load = f[:3]
		}
	}
	i.CPUs = cpuCount()
	mem := meminfo()
	i.MemTotal, i.MemFree = mem["MemTotal"]*1024, mem["MemAvailable"]*1024
	var st syscall.Statfs_t
	if syscall.Statfs(dataDir, &st) == nil {
		i.DiskTotal = st.Blocks * uint64(st.Bsize)
		i.DiskFree = st.Bavail * uint64(st.Bsize)
	}
	if data, err := os.ReadFile("/sys/class/thermal/thermal_zone0/temp"); err == nil {
		if t, err := strconv.ParseFloat(strings.TrimSpace(string(data)), 64); err == nil {
			t /= 1000
			i.Temperature = &t
		}
	}
	i.Timezone = Timezone()
	i.Time = time.Now().Format(time.RFC3339)
	return i
}

func releaseVersion() string {
	data, _ := os.ReadFile("/etc/suplaos-release")
	for _, line := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(line, "VERSION="); ok {
			return v
		}
	}
	return ""
}

func cpuCount() int {
	data, _ := os.ReadFile("/proc/cpuinfo")
	n := bytes.Count(data, []byte("\nprocessor"))
	if bytes.HasPrefix(data, []byte("processor")) {
		n++
	}
	return n
}

func meminfo() map[string]uint64 {
	m := map[string]uint64{}
	f, err := os.Open("/proc/meminfo")
	if err != nil {
		return m
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		k, v, ok := strings.Cut(sc.Text(), ":")
		if !ok {
			continue
		}
		fields := strings.Fields(v)
		if len(fields) > 0 {
			m[k], _ = strconv.ParseUint(fields[0], 10, 64)
		}
	}
	return m
}

// Addresses returns the IPv4 addresses of the network interfaces, without
// loopback and Docker bridges.
func Addresses() []string {
	var out []string
	ifaces, _ := net.Interfaces()
	for _, ifc := range ifaces {
		if ifc.Flags&net.FlagLoopback != 0 || ifc.Flags&net.FlagUp == 0 || isDockerIface(ifc.Name) {
			continue
		}
		addrs, _ := ifc.Addrs()
		for _, a := range addrs {
			if ipn, ok := a.(*net.IPNet); ok && ipn.IP.To4() != nil {
				out = append(out, ipn.IP.String())
			}
		}
	}
	return out
}

func isDockerIface(name string) bool {
	return name == "docker0" || strings.HasPrefix(name, "br-") || strings.HasPrefix(name, "veth")
}

// run runs a command and returns its error with the command output.
func run(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %v: %s", name, err, strings.TrimSpace(string(out)))
	}
	return nil
}

// CheckPassword tells whether the password is the one of the supla
// account. Needs root to read /etc/shadow.
func CheckPassword(password string) (bool, error) {
	hash, err := shadowHash(User)
	if err != nil {
		return false, err
	}
	if !strings.HasPrefix(hash, "$y$") {
		return false, fmt.Errorf("unsupported password hash")
	}
	got, err := yescrypt.Hash([]byte(password), []byte(hash))
	if err != nil {
		return false, err
	}
	return subtle.ConstantTimeCompare(got, []byte(hash)) == 1, nil
}

func shadowHash(user string) (string, error) {
	f, err := os.Open("/etc/shadow")
	if err != nil {
		return "", err
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	for sc.Scan() {
		fields := strings.Split(sc.Text(), ":")
		if len(fields) > 1 && fields[0] == user {
			return fields[1], nil
		}
	}
	return "", fmt.Errorf("no user %s", user)
}

// SetPassword changes the password of the supla account.
func SetPassword(password string) error {
	if len(password) < 8 {
		return errors.New("password too short")
	}
	if strings.ContainsAny(password, ":\n") {
		return errors.New("password must not contain ':' or line breaks")
	}
	cmd := exec.Command("chpasswd")
	cmd.Stdin = strings.NewReader(User + ":" + password + "\n")
	if out, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("chpasswd: %v: %s", err, strings.TrimSpace(string(out)))
	}
	return nil
}

// Timezone returns the system time zone.
func Timezone() string {
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if _, tz, ok := strings.Cut(link, "zoneinfo/"); ok {
			return tz
		}
	}
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		return strings.TrimSpace(string(data))
	}
	return "UTC"
}

// Timezones lists the time zones the system knows.
func Timezones() ([]string, error) {
	out, err := exec.Command("timedatectl", "list-timezones").Output()
	if err != nil {
		return nil, err
	}
	return strings.Fields(string(out)), nil
}

// SetTimezone changes the system time zone.
func SetTimezone(tz string) error {
	if err := run("timedatectl", "set-timezone", tz); err != nil {
		return err
	}
	return os.WriteFile("/etc/timezone", []byte(tz+"\n"), 0o644)
}

var hostnameRe = regexp.MustCompile(`^[a-z0-9]([a-z0-9-]{0,61}[a-z0-9])?$`)

// SetHostname changes the host name; with avahi the device is then
// <name>.local.
func SetHostname(name string) error {
	if !hostnameRe.MatchString(name) {
		return fmt.Errorf("invalid host name %q", name)
	}
	if err := run("hostnamectl", "set-hostname", name); err != nil {
		return err
	}
	data, err := os.ReadFile("/etc/hosts")
	if err != nil {
		return err
	}
	lines := strings.Split(string(data), "\n")
	found := false
	for i, l := range lines {
		if strings.HasPrefix(l, "127.0.1.1") {
			lines[i] = "127.0.1.1\t" + name
			found = true
		}
	}
	if !found {
		lines = append([]string{"127.0.1.1\t" + name}, lines...)
	}
	if err := os.WriteFile("/etc/hosts", []byte(strings.Join(lines, "\n")), 0o644); err != nil {
		return err
	}
	// avahi picks up the new name on restart.
	return run("systemctl", "try-restart", "avahi-daemon")
}

// Language codes of the interface and their system locales.
var Locales = map[string]string{"en": "en_US.UTF-8", "pl": "pl_PL.UTF-8"}

// SetLanguage sets the system locale for the language. The file is written
// directly: localectl is refused by systemd-localed on the image.
func SetLanguage(lang string) error {
	loc, ok := Locales[lang]
	if !ok {
		return fmt.Errorf("unknown language %q", lang)
	}
	return os.WriteFile("/etc/default/locale", []byte("LANG="+loc+"\n"), 0o644)
}

// Reboot and Poweroff run after a moment, so that the answer to the request
// still gets out.
func Reboot() { later("reboot") }

func Poweroff() { later("poweroff") }

func later(action string) {
	go func() {
		time.Sleep(time.Second)
		exec.Command("systemctl", action).Run()
	}()
}

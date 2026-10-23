package system

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"
)

// The update itself is /usr/lib/suplaos/upgrade, run as its own systemd
// unit: it may restart the panel. It writes these files.
const (
	upgradeUnit   = "suplaos-upgrade"
	upgradeScript = "/usr/lib/suplaos/upgrade"
	upgradeState  = "/var/lib/suplaos/upgrade-state"
	upgradeLog    = "/var/log/suplaos/upgrade.log"
	rebootFlag    = "/run/suplaos/reboot-required"

	// Daily automatic updates of Debian (unattended-upgrades).
	autoUpgradesFile  = "/etc/apt/apt.conf.d/20auto-upgrades"
	autoUpgradesStamp = "/var/lib/apt/periodic/unattended-upgrades-stamp"
)

// Package is an update of a Debian package.
type Package struct {
	Name string `json:"name"`
	From string `json:"from"`
	To   string `json:"to"`
}

// Updates is the update state of the system.
type Updates struct {
	Version string `json:"version"`
	// State of the last update: "" (none yet), running, done, failed.
	State    string     `json:"state"`
	Finished *time.Time `json:"finished"`
	// Checked is when the package lists were downloaded last.
	Checked        *time.Time `json:"checked"`
	Packages       []Package  `json:"packages"`
	RebootRequired bool       `json:"rebootRequired"`
	// Auto: daily automatic updates of Debian packages; AutoLast is their
	// last run.
	Auto     bool       `json:"auto"`
	AutoLast *time.Time `json:"autoLast"`
}

// ReadUpdates returns the state from the downloaded package lists; it does
// not download them (CheckUpdates does).
func ReadUpdates() (*Updates, error) {
	u := &Updates{Version: Version(), Packages: []Package{}}
	if data, err := os.ReadFile(upgradeState); err == nil {
		f := strings.Fields(string(data))
		if len(f) == 2 {
			u.State = f[0]
			if ts, err := strconv.ParseInt(f[1], 10, 64); err == nil && u.State != "running" {
				t := time.Unix(ts, 0)
				u.Finished = &t
			}
		}
	}
	// The unit is gone, e.g. after a power cut: the update did not finish.
	if u.State == "running" && exec.Command("systemctl", "is-active", "--quiet", upgradeUnit).Run() != nil {
		u.State = "failed"
	}
	u.RebootRequired = rebootRequired()
	u.Auto = AutoUpgrades()
	if st, err := os.Stat(autoUpgradesStamp); err == nil {
		t := st.ModTime()
		u.AutoLast = &t
	}
	if lists, _ := filepath.Glob("/var/lib/apt/lists/*Release"); len(lists) > 0 {
		var newest time.Time
		for _, l := range lists {
			if st, err := os.Stat(l); err == nil && st.ModTime().After(newest) {
				newest = st.ModTime()
			}
		}
		u.Checked = &newest
	}
	cmd := exec.Command("apt", "list", "--upgradable")
	// The output is parsed: no translations.
	cmd.Env = append(os.Environ(), "LC_ALL=C")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("apt list: %w", err)
	}
	u.Packages = parseUpgradable(string(out))
	return u, nil
}

// parseUpgradable reads lines like
// "suplaos/trixie 0.2.0 arm64 [upgradable from: 0.1.0]".
func parseUpgradable(out string) []Package {
	list := []Package{}
	sc := bufio.NewScanner(strings.NewReader(out))
	for sc.Scan() {
		line := sc.Text()
		name, rest, ok := strings.Cut(line, "/")
		if !ok || !strings.Contains(rest, "upgradable from: ") {
			continue
		}
		f := strings.Fields(rest)
		if len(f) < 2 {
			continue
		}
		_, from, _ := strings.Cut(rest, "upgradable from: ")
		list = append(list, Package{Name: name, To: f[1], From: strings.TrimSuffix(from, "]")})
	}
	return list
}

// rebootRequired tells whether a newer kernel than the running one is
// installed, by any update (also the automatic one).
func rebootRequired() bool {
	if _, err := os.Stat(rebootFlag); err == nil {
		return true
	}
	data, err := os.ReadFile("/proc/sys/kernel/osrelease")
	if err != nil {
		return false
	}
	running := strings.TrimSpace(string(data))
	files, _ := filepath.Glob("/boot/vmlinuz-*")
	var installed []string
	for _, f := range files {
		installed = append(installed, strings.TrimPrefix(filepath.Base(f), "vmlinuz-"))
	}
	return newerKernel(running, installed)
}

// newerKernel tells whether installed has a newer kernel of the flavour of
// running, e.g. 6.18.51+rpt-rpi-v8 for 6.18.50+rpt-rpi-v8 (and not
// 6.18.51+rpt-rpi-2712, the Pi 5 kernel).
func newerKernel(running string, installed []string) bool {
	_, flavour, ok := strings.Cut(running, "-")
	if !ok {
		return false
	}
	for _, k := range installed {
		if _, f, _ := strings.Cut(k, "-"); f == flavour && compareVersions(k, running) > 0 {
			return true
		}
	}
	return false
}

// compareVersions compares kernel versions number by number.
func compareVersions(a, b string) int {
	split := func(s string) []int {
		var n []int
		for _, p := range strings.FieldsFunc(s, func(r rune) bool { return r < '0' || r > '9' }) {
			v, _ := strconv.Atoi(p)
			n = append(n, v)
		}
		return n
	}
	x, y := split(a), split(b)
	for i := 0; i < len(x) && i < len(y); i++ {
		if x[i] != y[i] {
			if x[i] > y[i] {
				return 1
			}
			return -1
		}
	}
	return len(x) - len(y)
}

// AutoUpgrades tells whether the daily automatic updates are on.
func AutoUpgrades() bool {
	data, err := os.ReadFile(autoUpgradesFile)
	if err != nil {
		return false
	}
	for _, line := range strings.Split(string(data), "\n") {
		if strings.Contains(line, "APT::Periodic::Unattended-Upgrade") {
			return strings.Contains(line, `"1"`)
		}
	}
	return false
}

// SetAutoUpgrades turns the daily automatic updates on or off, like
// dpkg-reconfigure unattended-upgrades.
func SetAutoUpgrades(on bool) error {
	v := "0"
	if on {
		v = "1"
	}
	content := fmt.Sprintf("APT::Periodic::Update-Package-Lists \"%s\";\nAPT::Periodic::Unattended-Upgrade \"%s\";\n", v, v)
	return os.WriteFile(autoUpgradesFile, []byte(content), 0o644)
}

// Version is the version of the installed suplaos package.
func Version() string {
	if data, err := os.ReadFile("/usr/share/suplaos/version"); err == nil {
		return strings.TrimSpace(string(data))
	}
	return releaseVersion()
}

// CheckUpdates downloads the package lists, writing apt output to w.
func CheckUpdates(w io.Writer) error {
	cmd := exec.Command("apt-get", "update")
	cmd.Stdout, cmd.Stderr = w, w
	cmd.Env = append(os.Environ(), "DEBIAN_FRONTEND=noninteractive")
	return cmd.Run()
}

// ErrUpgradeRunning is returned when an update is already going on.
var ErrUpgradeRunning = errors.New("an update is already running")

// StartUpgrade starts the update in the background.
func StartUpgrade() error {
	if exec.Command("systemctl", "is-active", "--quiet", upgradeUnit).Run() == nil {
		return ErrUpgradeRunning
	}
	// The browser starts reading the log right away: no lines of the last
	// update.
	if err := os.MkdirAll(filepath.Dir(upgradeLog), 0o755); err == nil {
		os.WriteFile(upgradeLog, nil, 0o644)
	}
	os.WriteFile(upgradeState, []byte(fmt.Sprintf("running %d\n", time.Now().Unix())), 0o644)
	return run("systemd-run", "--unit="+upgradeUnit, "--collect", "--description=SuplaOS update", upgradeScript)
}

// RunUpgrade runs the update and waits for it, with its output on this
// terminal (suplaos upgrade).
func RunUpgrade() error {
	cmd := exec.Command("systemd-run", "--unit="+upgradeUnit, "--collect", "--wait", "--pipe", "--quiet",
		"--description=SuplaOS update", upgradeScript)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = os.Stdin, os.Stdout, os.Stderr
	return cmd.Run()
}

// UpgradeLog returns the update log from the byte offset on and the new
// offset.
func UpgradeLog(offset int64) (string, int64, error) {
	f, err := os.Open(upgradeLog)
	if errors.Is(err, os.ErrNotExist) {
		return "", 0, nil
	}
	if err != nil {
		return "", offset, err
	}
	defer f.Close()
	st, err := f.Stat()
	if err != nil {
		return "", offset, err
	}
	// A new update started: the log was truncated.
	if offset > st.Size() {
		offset = 0
	}
	if _, err := f.Seek(offset, io.SeekStart); err != nil {
		return "", offset, err
	}
	data, err := io.ReadAll(io.LimitReader(f, 1<<20))
	return string(data), offset + int64(len(data)), err
}

// dpkgLock is a variable for the tests.
var dpkgLock = "/var/lib/dpkg/lock-frontend"

// PackagesBusy tells whether apt or dpkg is installing packages now: they
// hold a lock on the frontend lock file while they work.
func PackagesBusy() bool {
	f, err := os.Open(dpkgLock)
	if err != nil {
		return false
	}
	defer f.Close()
	lk := syscall.Flock_t{Type: syscall.F_WRLCK}
	if err := syscall.FcntlFlock(f.Fd(), syscall.F_GETLK, &lk); err != nil {
		return false
	}
	return lk.Type != syscall.F_UNLCK
}

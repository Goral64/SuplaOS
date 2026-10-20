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
	if _, err := os.Stat(rebootFlag); err == nil {
		u.RebootRequired = true
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

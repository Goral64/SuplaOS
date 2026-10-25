package fan

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// Files on the Raspberry Pi the fan settings go to; tests replace them.
var (
	configTxt = "/boot/firmware/config.txt"
	// modulesFile loads i2c-dev, for /dev/i2c-1.
	modulesFile = "/etc/modules-load.d/suplaos-fan.conf"
	// argonFile tells the shutdown hook (package/usr/lib/systemd/
	// system-shutdown/suplaos-argon) to cut the power of the Argon ONE.
	argonFile = "/etc/suplaos/argon"
)

const (
	blockStart = "# Fan, set on the System page of the panel."
	blockEnd   = "# End of the fan settings."
)

// block returns the lines of config.txt for the settings.
func (s *Settings) block() []string {
	var lines []string
	switch s.Kind {
	case "argon":
		lines = append(lines, "dtparam=i2c_arm=on")
	case "gpio":
		lines = append(lines, fmt.Sprintf("dtoverlay=gpio-fan,gpiopin=%d,temp=%d", s.Pin, s.Temp*1000))
	}
	if len(lines) == 0 {
		return nil
	}
	return append(append([]string{blockStart}, lines...), blockEnd)
}

// Apply writes the settings to config.txt and the other files. It returns
// whether config.txt changed: the firmware reads it only at the start, a
// restart is needed.
func (s *Settings) Apply() (bool, error) {
	data, err := os.ReadFile(configTxt)
	if err != nil {
		return false, err
	}
	var kept []string
	in := false
	for _, line := range strings.Split(strings.TrimRight(string(data), "\n"), "\n") {
		switch {
		case line == blockStart:
			in = true
		case line == blockEnd:
			in = false
		case !in:
			kept = append(kept, line)
		}
	}
	next := strings.Join(append(kept, s.block()...), "\n") + "\n"
	changed := next != string(data)
	if changed {
		tmp := configTxt + ".new"
		if err := os.WriteFile(tmp, []byte(next), 0o644); err != nil {
			return false, err
		}
		if err := os.Rename(tmp, configTxt); err != nil {
			return false, err
		}
	}
	argon := s.Kind == "argon"
	if err := setFile(modulesFile, "i2c-dev\n", argon); err != nil {
		return changed, err
	}
	return changed, setFile(argonFile, "Cut the power of the Argon ONE after shutdown.\n", argon)
}

// setFile writes a file with the content when on, deletes it otherwise.
func setFile(path, content string, on bool) error {
	if !on {
		if err := os.Remove(path); err != nil && !os.IsNotExist(err) {
			return err
		}
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return os.WriteFile(path, []byte(content), 0o644)
}

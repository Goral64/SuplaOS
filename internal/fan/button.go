package fan

import (
	"bufio"
	"context"
	"os/exec"
	"strconv"
	"strings"
	"time"
)

// The Argon ONE tells about its power button with a pulse on GPIO 4: 20-30
// ms for a double press, 40-50 ms when it is held for 3 s.
func pulseAction(width time.Duration) string {
	switch {
	case width >= 15*time.Millisecond && width < 35*time.Millisecond:
		return "reboot"
	case width >= 35*time.Millisecond && width < 70*time.Millisecond:
		return "poweroff"
	}
	return ""
}

// gpiomon is the command that reports the edges on GPIO 4; tests replace
// it.
var gpiomon = func(ctx context.Context) *exec.Cmd {
	return exec.CommandContext(ctx, "gpiomon", "-b", "pull-down", "-F", "%e %S", "GPIO4")
}

// WatchButton calls act with reboot or poweroff when the power button of
// the Argon ONE asks for it, until ctx is done.
func WatchButton(ctx context.Context, act func(string)) error {
	cmd := gpiomon(ctx)
	out, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}
	if err := cmd.Start(); err != nil {
		return err
	}
	var rise float64
	sc := bufio.NewScanner(out)
	for sc.Scan() {
		edge, ts, ok := strings.Cut(strings.TrimSpace(sc.Text()), " ")
		if !ok {
			continue
		}
		t, err := strconv.ParseFloat(ts, 64)
		if err != nil {
			continue
		}
		switch edge {
		case "1":
			rise = t
		case "2":
			if rise > 0 {
				if a := pulseAction(time.Duration((t - rise) * float64(time.Second))); a != "" {
					act(a)
				}
			}
			rise = 0
		}
	}
	return cmd.Wait()
}

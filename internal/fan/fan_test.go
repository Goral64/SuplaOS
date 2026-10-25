package fan

import (
	"context"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"testing"
)

func TestSpeed(t *testing.T) {
	s := Default()
	for _, c := range []struct {
		temp    float64
		current int
		want    int
	}{
		{40, 0, 0}, {54.9, 0, 0}, {55, 0, 30}, {62.5, 0, 65}, {70, 0, 100}, {80, 50, 100},
		{52, 30, 30}, {50.9, 30, 0}, {52, 0, 0},
	} {
		if got := s.Speed(c.temp, c.current); got != c.want {
			t.Errorf("Speed(%v, %d) = %d, want %d", c.temp, c.current, got, c.want)
		}
	}
}

func TestValidate(t *testing.T) {
	for _, s := range []Settings{
		{Kind: "argon", Start: 70, Full: 60}, {Kind: "argon", Start: 20, Full: 60},
		{Kind: "gpio", Pin: 40, Temp: 60}, {Kind: "gpio", Pin: 14, Temp: 95}, {Kind: "noctua"},
	} {
		if s.Validate() == nil {
			t.Errorf("%+v accepted", s)
		}
	}
	for _, s := range []Settings{Default(), {Kind: "argon", Start: 50, Full: 65}, {Kind: "gpio", Pin: 14, Temp: 60}} {
		if err := s.Validate(); err != nil {
			t.Errorf("%+v: %v", s, err)
		}
	}
}

func TestApply(t *testing.T) {
	dir := t.TempDir()
	configTxt = filepath.Join(dir, "config.txt")
	modulesFile = filepath.Join(dir, "modules-load.d", "suplaos-fan.conf")
	argonFile = filepath.Join(dir, "suplaos", "argon")
	base := "arm_64bit=1\ndisable_overscan=1\n"
	os.WriteFile(configTxt, []byte(base), 0o644)

	s := Settings{Kind: "argon", Start: 55, Full: 70, Button: true}
	changed, err := s.Apply()
	if err != nil || !changed {
		t.Fatalf("argon: %v %v", changed, err)
	}
	data, _ := os.ReadFile(configTxt)
	want := base + blockStart + "\ndtparam=i2c_arm=on\n" + blockEnd + "\n"
	if string(data) != want {
		t.Errorf("config.txt:\n%s", data)
	}
	for _, f := range []string{modulesFile, argonFile} {
		if _, err := os.Stat(f); err != nil {
			t.Error(err)
		}
	}
	if changed, _ := s.Apply(); changed {
		t.Error("changed again")
	}
	// The temperatures do not touch config.txt.
	s.Start = 50
	if changed, _ := s.Apply(); changed {
		t.Error("changed by the temperatures")
	}

	g := Settings{Kind: "gpio", Pin: 14, Temp: 60}
	must(t, nil2(g.Apply()))
	data, _ = os.ReadFile(configTxt)
	if string(data) != base+blockStart+"\ndtoverlay=gpio-fan,gpiopin=14,temp=60000\n"+blockEnd+"\n" {
		t.Errorf("gpio config.txt:\n%s", data)
	}
	if _, err := os.Stat(argonFile); err == nil {
		t.Error("argon file left")
	}
	n := Default()
	must(t, nil2(n.Apply()))
	if data, _ := os.ReadFile(configTxt); string(data) != base {
		t.Errorf("none config.txt:\n%s", data)
	}
}

func TestSetArgon(t *testing.T) {
	var got [][]byte
	old := i2cWrite
	i2cWrite = func(addr int, data []byte) error {
		if addr != 0x1a {
			t.Errorf("address %#x", addr)
		}
		got = append(got, data)
		return nil
	}
	t.Cleanup(func() { i2cWrite = old })
	SetArgon("Raspberry Pi 4 Model B Rev 1.1", 55)
	SetArgon("Raspberry Pi 5 Model B Rev 1.0", 55)
	if len(got) != 2 || !slices.Equal(got[0], []byte{55}) || !slices.Equal(got[1], []byte{0x80, 55}) {
		t.Errorf("writes %v", got)
	}
}

func nil2(_ bool, err error) error { return err }

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func TestWatchButton(t *testing.T) {
	old := gpiomon
	gpiomon = func(ctx context.Context) *exec.Cmd {
		// A double press, a pulse too short, a held button.
		return exec.CommandContext(ctx, "printf", "1 100.000000000\n2 100.025000000\n1 101.0\n2 101.005\n2 102.0\n1 103.000\n2 103.045\n")
	}
	t.Cleanup(func() { gpiomon = old })
	var got []string
	WatchButton(context.Background(), func(a string) { got = append(got, a) })
	if !slices.Equal(got, []string{"reboot", "poweroff"}) {
		t.Errorf("actions %v", got)
	}
}

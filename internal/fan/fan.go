// Package fan controls the fan of a Raspberry Pi case: the Argon ONE over
// I2C, by temperature, or a fan on a GPIO pin, which the kernel switches
// itself (gpio-fan overlay).
package fan

import (
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"
	"syscall"
)

// Settings of the fan, kept with the panel settings.
type Settings struct {
	// Kind is none, argon or gpio.
	Kind string `yaml:"kind" json:"kind"`
	// Argon ONE: the fan starts at Start °C and runs full at Full °C.
	Start int `yaml:"start" json:"start"`
	Full  int `yaml:"full" json:"full"`
	// Button: the power button of the Argon ONE restarts (double press)
	// or shuts down (held) the system.
	Button bool `yaml:"button" json:"button"`
	// Fan on a GPIO pin, on from Temp °C.
	Pin  int `yaml:"pin" json:"pin"`
	Temp int `yaml:"temp" json:"temp"`
}

// Default are the settings before any are saved.
func Default() Settings {
	return Settings{Kind: "none", Start: 55, Full: 70, Pin: 14, Temp: 60}
}

// Validate checks the settings.
func (s *Settings) Validate() error {
	switch s.Kind {
	case "none":
	case "argon":
		if s.Start < 30 || s.Full > 85 || s.Start >= s.Full {
			return errors.New("the fan has to start between 30 and 85 °C, below the full speed temperature")
		}
	case "gpio":
		if s.Pin < 2 || s.Pin > 27 {
			return errors.New("the GPIO pin has to be 2 to 27")
		}
		if s.Temp < 30 || s.Temp > 85 {
			return errors.New("the temperature has to be 30 to 85 °C")
		}
	default:
		return fmt.Errorf("unknown fan %q", s.Kind)
	}
	return nil
}

// Model is the model of the Raspberry Pi, "" on other computers.
func Model() string {
	data, err := os.ReadFile(modelFile)
	if err != nil || !strings.HasPrefix(string(data), "Raspberry Pi") {
		return ""
	}
	return strings.TrimRight(string(data), "\x00\n")
}

var modelFile = "/proc/device-tree/model"

// pi5 tells whether the model is a Raspberry Pi 5: its Argon ONE V3 takes
// the speed in a register.
func pi5(model string) bool { return strings.HasPrefix(model, "Raspberry Pi 5") }

// Temperature reads the temperature of the CPU in °C.
func Temperature() (float64, error) {
	data, err := os.ReadFile(tempFile)
	if err != nil {
		return 0, err
	}
	milli, err := strconv.Atoi(strings.TrimSpace(string(data)))
	if err != nil {
		return 0, err
	}
	return float64(milli) / 1000, nil
}

var tempFile = "/sys/class/thermal/thermal_zone0/temp"

// hysteresis: the fan stops this many degrees below Start, so that it does
// not start and stop all the time around it.
const hysteresis = 4

// Speed returns the speed in percent for the temperature: off below Start
// (below Start - hysteresis when it runs already), from 30% at Start up to
// 100% at Full.
func (s *Settings) Speed(temp float64, current int) int {
	start := float64(s.Start)
	if current > 0 {
		start -= hysteresis
	}
	switch {
	case temp < start:
		return 0
	case temp >= float64(s.Full):
		return 100
	}
	frac := (temp - float64(s.Start)) / float64(s.Full-s.Start)
	if frac < 0 {
		frac = 0
	}
	return 30 + int(frac*70+0.5)
}

// argonAddr is the I2C address of the Argon ONE microcontroller.
const argonAddr = 0x1a

// i2cBus is the I2C bus on the GPIO header (dtparam=i2c_arm=on).
var i2cBus = "/dev/i2c-1"

// SetArgon sets the speed of the Argon ONE fan, in percent.
func SetArgon(model string, speed int) error {
	data := []byte{byte(speed)}
	if pi5(model) {
		data = []byte{0x80, byte(speed)}
	}
	return i2cWrite(argonAddr, data)
}

// ArgonPowerOff tells the Argon ONE to cut the power, after the system has
// shut down.
func ArgonPowerOff() error {
	return i2cWrite(argonAddr, []byte{0xff})
}

// i2cWrite writes bytes to a device on the bus; tests replace it.
var i2cWrite = func(addr int, data []byte) error {
	f, err := os.OpenFile(i2cBus, os.O_RDWR, 0)
	if err != nil {
		return err
	}
	defer f.Close()
	const i2cSlave = 0x0703
	if _, _, errno := syscall.Syscall(syscall.SYS_IOCTL, f.Fd(), i2cSlave, uintptr(addr)); errno != 0 {
		return fmt.Errorf("I2C address %#x: %w", addr, errno)
	}
	_, err = f.Write(data)
	return err
}

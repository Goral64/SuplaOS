package panel

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/Goral64/SuplaOS/internal/fan"
	"github.com/Goral64/SuplaOS/internal/system"
)

// fanState is what the fan loop last did.
type fanState struct {
	mu    sync.Mutex
	temp  float64
	speed int
	err   string
}

var errNotRaspberry = errors.New("the fan is set only on a Raspberry Pi")

// fanReboot marks that config.txt changed and the device has to be
// restarted; /run is emptied by the restart.
var fanReboot = "/run/suplaos/fan-reboot"

func (s *Server) fanSettings() fan.Settings {
	if f := s.getSettings().Fan; f != nil {
		return *f
	}
	return fan.Default()
}

func (s *Server) getFan(w http.ResponseWriter, r *http.Request) {
	model := fan.Model()
	_, err := os.Stat(fanReboot)
	s.fan.mu.Lock()
	defer s.fan.mu.Unlock()
	temp := s.fan.temp
	if t, err := fan.Temperature(); err == nil {
		temp = t
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"model":        model,
		"settings":     s.fanSettings(),
		"temperature":  temp,
		"speed":        s.fan.speed,
		"error":        s.fan.err,
		"rebootNeeded": err == nil,
	})
}

func (s *Server) putFan(w http.ResponseWriter, r *http.Request) {
	if fan.Model() == "" {
		jsonError(w, http.StatusBadRequest, errNotRaspberry)
		return
	}
	var set fan.Settings
	if err := readJSON(r, &set); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := set.Validate(); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	changed, err := set.Apply()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	if changed {
		os.MkdirAll("/run/suplaos", 0o755)
		os.WriteFile(fanReboot, nil, 0o644)
	}
	if err := s.saveSettings(func(st *Settings) { st.Fan = &set }); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	s.getFan(w, r)
}

// WatchFan sets the speed of the Argon ONE fan by the temperature; it does
// not return. Other fans are switched by the kernel.
func (s *Server) WatchFan() {
	model := fan.Model()
	if model == "" {
		return
	}
	current, written := -1, time.Time{}
	var button *argonButton
	for ; ; time.Sleep(10 * time.Second) {
		set := s.fanSettings()
		button = button.update(set.Kind == "argon" && set.Button)
		temp, err := fan.Temperature()
		if err != nil {
			continue
		}
		speed := 0
		if set.Kind == "argon" {
			speed = set.Speed(temp, max(current, 0))
		} else if current <= 0 {
			// Not an Argon (any more): stopped it once, nothing to do.
			s.setFanState(temp, 0, "")
			current = 0
			continue
		}
		// Written again now and then: the case forgets it when its power
		// is cut.
		if speed == current && time.Since(written) < time.Minute {
			s.setFanState(temp, speed, "")
			continue
		}
		msg := ""
		if err := fan.SetArgon(model, speed); err != nil {
			msg = err.Error()
			if current != -2 {
				log.Printf("fan: %v", err)
			}
			current = -2
		} else {
			current, written = speed, time.Now()
		}
		s.setFanState(temp, speed, msg)
	}
}

func (s *Server) setFanState(temp float64, speed int, err string) {
	s.fan.mu.Lock()
	s.fan.temp, s.fan.speed, s.fan.err = temp, speed, err
	s.fan.mu.Unlock()
}

// argonButton watches the power button of the Argon ONE while it runs.
type argonButton struct {
	cancel context.CancelFunc
	done   chan struct{}
}

// update starts or stops watching the button; it is started again when
// gpiomon ended (e.g. it was not installed yet).
func (b *argonButton) update(want bool) *argonButton {
	if b != nil {
		select {
		case <-b.done:
			b = nil
		default:
		}
	}
	switch {
	case want && b == nil:
		ctx, cancel := context.WithCancel(context.Background())
		b = &argonButton{cancel: cancel, done: make(chan struct{})}
		go func() {
			defer close(b.done)
			err := fan.WatchButton(ctx, func(action string) {
				log.Printf("Argon ONE power button: %s", action)
				if action == "reboot" {
					system.Reboot()
				} else {
					system.Poweroff()
				}
			})
			if err != nil && ctx.Err() == nil {
				log.Printf("Argon ONE power button: %v", err)
			}
		}()
	case !want && b != nil:
		b.cancel()
		b = nil
	}
	return b
}

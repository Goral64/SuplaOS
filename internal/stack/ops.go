package stack

import (
	"crypto/rand"
	"errors"
	"fmt"
	"math/big"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// Install adds an add-on to the state with the given option values on top
// of the defaults and writes the stack directory. Starting it is up to the
// caller (Up).
func (s *Stack) Install(id string, set map[string]string) error {
	a := s.Catalog[id]
	if a == nil {
		return fmt.Errorf("unknown add-on %q", id)
	}
	if s.IsInstalled(id) {
		return fmt.Errorf("%s is already installed", id)
	}
	for _, r := range a.Requires {
		if !s.IsInstalled(r) {
			return fmt.Errorf("%s requires %s, install it first", id, r)
		}
	}
	inst := &Installed{Options: map[string]string{}}
	vars := systemVars()
	for _, o := range a.Options {
		switch {
		case o.Generate > 0:
			inst.Options[o.Key] = randomString(o.Generate)
		case strings.Contains(o.Default, "{"), o.UpgradeDefault != "":
			inst.Options[o.Key] = expand(o.Default, vars)
		}
	}
	if err := s.setOptions(a, inst, set); err != nil {
		return err
	}
	if a.Fetch != nil {
		if err := s.fetch(a); err != nil {
			return err
		}
	}
	s.State.Addons[id] = inst
	if err := s.Apply(); err != nil {
		delete(s.State.Addons, id)
		return err
	}
	s.queueAfter(a, nil)
	return s.Save()
}

// queueAfter queues the actions of options whose value changed from old
// (nil on install) to the value of the action.
func (s *Stack) queueAfter(a *addon.Addon, old map[string]string) {
	values := s.Values(a)
	for _, o := range a.Options {
		for _, act := range o.After {
			if values[o.Key] == act.Value && (old == nil || old[o.Key] != act.Value) {
				s.after = append(s.after, act)
			}
		}
	}
}

// QueueAfter queues the actions for the current value of an option, e.g.
// after creating a Supla account; Up or RunAfter runs them.
func (s *Stack) QueueAfter(id, key string) {
	a := s.Catalog[id]
	if a == nil || !s.IsInstalled(id) || a.Option(key) == nil {
		return
	}
	v := s.Values(a)[key]
	for _, act := range a.Option(key).After {
		if act.Value == v {
			s.after = append(s.after, act)
		}
	}
}

// Configure changes option values of an installed add-on and rewrites the
// stack directory.
func (s *Stack) Configure(id string, set map[string]string) error {
	a := s.Catalog[id]
	inst := s.State.Addons[id]
	if inst == nil {
		return fmt.Errorf("%s is not installed", id)
	}
	old := inst.Options
	inst.Options = map[string]string{}
	for k, v := range old {
		inst.Options[k] = v
	}
	before := s.Values(a)
	if err := s.setOptions(a, inst, set); err != nil {
		inst.Options = old
		return err
	}
	if err := s.Apply(); err != nil {
		inst.Options = old
		return err
	}
	s.queueAfter(a, before)
	return s.Save()
}

// Dependents returns the installed add-ons that require id.
func (s *Stack) Dependents(id string) []string {
	var out []string
	for _, a := range s.Order() {
		for _, r := range a.Requires {
			if r == id {
				out = append(out, a.ID)
			}
		}
	}
	return out
}

// Remove drops an add-on from the state and rewrites the stack directory.
// Its containers have to be removed before (with the old configuration).
func (s *Stack) Remove(id string) error {
	if !s.IsInstalled(id) {
		return fmt.Errorf("%s is not installed", id)
	}
	if d := s.Dependents(id); len(d) > 0 {
		return fmt.Errorf("%s is required by %s, remove it first", id, strings.Join(d, ", "))
	}
	inst := s.State.Addons[id]
	delete(s.State.Addons, id)
	if err := s.Apply(); err != nil {
		s.State.Addons[id] = inst
		return err
	}
	return s.Save()
}

// Purge deletes the data of an add-on that is not installed.
func (s *Stack) Purge(id string) error {
	a := s.Catalog[id]
	if a == nil || s.IsInstalled(id) {
		return fmt.Errorf("%s: purge only removed add-ons", id)
	}
	for _, d := range a.Data {
		if err := os.RemoveAll(filepath.Join(s.Dir(), d)); err != nil {
			if errors.Is(err, os.ErrPermission) {
				return fmt.Errorf("%w (files created by containers belong to root, try: sudo suplaos purge %s)", err, id)
			}
			return err
		}
	}
	return os.RemoveAll(s.cacheDir(id))
}

// setOptions validates new values against the option definitions and
// stores them; then all active options of the add-on have to be valid.
func (s *Stack) setOptions(a *addon.Addon, inst *Installed, set map[string]string) error {
	for k, v := range set {
		o := a.Option(k)
		if o == nil {
			return fmt.Errorf("%s has no option %s", a.ID, k)
		}
		// An empty value restores the default.
		if v == "" {
			if o.Generate > 0 {
				v = randomString(o.Generate)
			} else {
				v = expand(o.Default, systemVars())
			}
		}
		inst.Options[k] = v
	}
	values := map[string]string{}
	for i := range a.Options {
		values[a.Options[i].Key] = installedDefault(&a.Options[i])
	}
	for k, v := range inst.Options {
		values[k] = v
	}
	var errs []string
	for i := range a.Options {
		o := &a.Options[i]
		if !o.When.Match(s.IsInstalled, s.withOthers(values)) {
			continue
		}
		if err := checkValue(o, values[o.Key]); err != nil {
			errs = append(errs, err.Error())
		}
	}
	if len(errs) > 0 {
		return fmt.Errorf("%s: %s", a.ID, strings.Join(errs, "; "))
	}
	return nil
}

func checkValue(o *addon.Option, v string) error {
	if strings.ContainsAny(v, "'\n\r") {
		return fmt.Errorf("%s must not contain quotes or line breaks", o.Key)
	}
	if v == "" {
		if o.Required {
			return fmt.Errorf("%s is required", o.Key)
		}
		return nil
	}
	switch o.Type {
	case addon.TypePort:
		if n, err := strconv.Atoi(v); err != nil || n < 1 || n > 65535 {
			return fmt.Errorf("%s: invalid port %q", o.Key, v)
		}
	case addon.TypeEmail:
		if !strings.Contains(v, "@") {
			return fmt.Errorf("%s: invalid e-mail %q", o.Key, v)
		}
	case addon.TypeSelect:
		for _, c := range o.Choices {
			if v == c {
				return nil
			}
		}
		return fmt.Errorf("%s must be one of: %s", o.Key, strings.Join(o.Choices, ", "))
	case addon.TypeSerial:
		if !strings.HasPrefix(v, "/dev/") {
			return fmt.Errorf("%s: expected a device path (/dev/...)", o.Key)
		}
	}
	return nil
}

const alphanumeric = "abcdefghijklmnopqrstuvwxyzABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

func randomString(n int) string {
	b := make([]byte, n)
	max := big.NewInt(int64(len(alphanumeric)))
	for i := range b {
		r, err := rand.Int(rand.Reader, max)
		if err != nil {
			panic(err)
		}
		b[i] = alphanumeric[r.Int64()]
	}
	return string(b)
}

// DefaultValue returns the default of an option with placeholders such as
// {ip} filled in.
func DefaultValue(o *addon.Option) string {
	if strings.Contains(o.Default, "{") {
		return expand(o.Default, systemVars())
	}
	return o.Default
}

// installedDefault is the value of an option an installed add-on has no
// value of: it was installed before the option was added.
func installedDefault(o *addon.Option) string {
	if o.UpgradeDefault != "" {
		return o.UpgradeDefault
	}
	return DefaultValue(o)
}

// systemVars are the placeholders allowed in option defaults.
func systemVars() map[string]string {
	host, _ := os.Hostname()
	return map[string]string{"ip": PrimaryIP(), "hostname": host}
}

func expand(s string, vars map[string]string) string {
	for k, v := range vars {
		s = strings.ReplaceAll(s, "{"+k+"}", v)
	}
	return s
}

// PrimaryIP returns the address of the interface with the default route.
func PrimaryIP() string {
	// No packet is sent, connecting a UDP socket only picks the route.
	conn, err := net.Dial("udp4", "192.0.2.1:9")
	if err != nil {
		return "127.0.0.1"
	}
	defer conn.Close()
	return conn.LocalAddr().(*net.UDPAddr).IP.String()
}

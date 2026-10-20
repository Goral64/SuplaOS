// Package stack turns the installed add-ons into one docker compose project
// in <root>/stack and runs docker compose on it.
package stack

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gopkg.in/yaml.v3"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// ProjectName is the compose project name. supla-docker names its
// containers after it (supla-cloud, supla-db, supla-server).
const ProjectName = "supla"

// Installed is an installed add-on with the values of its options.
type Installed struct {
	Options map[string]string `yaml:"options"`
}

type State struct {
	Addons map[string]*Installed `yaml:"addons"`
}

// Stack is the SuplaOS data directory: state.yaml, the compose project in
// stack/, downloaded add-on files in cache/ and backups in backups/.
type Stack struct {
	Root    string
	Catalog addon.Catalog
	State   *State
	// TZ is the system time zone, passed to the containers.
	TZ string

	lock *os.File
}

// Open loads the catalog and the state.
func Open(root, catalogDir string) (*Stack, error) {
	c, err := addon.Load(catalogDir)
	if err != nil {
		return nil, err
	}
	s := &Stack{Root: root, Catalog: c, State: &State{}, TZ: systemTZ()}
	data, err := os.ReadFile(s.statePath())
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		return nil, err
	}
	if err := yaml.Unmarshal(data, s.State); err != nil {
		return nil, fmt.Errorf("%s: %w", s.statePath(), err)
	}
	if s.State.Addons == nil {
		s.State.Addons = map[string]*Installed{}
	}
	for id, inst := range s.State.Addons {
		if s.Catalog[id] == nil {
			return nil, fmt.Errorf("installed add-on %q is missing from the catalog", id)
		}
		if inst.Options == nil {
			inst.Options = map[string]string{}
		}
	}
	return s, nil
}

func (s *Stack) statePath() string  { return filepath.Join(s.Root, "state.yaml") }
func (s *Stack) Dir() string        { return filepath.Join(s.Root, "stack") }
func (s *Stack) BackupDir() string  { return filepath.Join(s.Root, "backups") }
func (s *Stack) cacheDir(id string) string {
	return filepath.Join(s.Root, "cache", id)
}

// Lock takes an exclusive lock on the data directory, held until the
// process exits. Taken by every command that changes something.
func (s *Stack) Lock() error {
	if err := os.MkdirAll(s.Root, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(s.Root, ".lock"), os.O_CREATE|os.O_RDWR, 0o644)
	if err != nil {
		return err
	}
	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX|syscall.LOCK_NB); err != nil {
		f.Close()
		return fmt.Errorf("another suplaos command is running")
	}
	s.lock = f
	return nil
}

// Save writes state.yaml. It holds secrets, so it is readable only by the
// owner.
func (s *Stack) Save() error {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(s.State); err != nil {
		return err
	}
	return writeFile(s.statePath(), buf.Bytes(), 0o600)
}

// IsInstalled tells whether the add-on is installed.
func (s *Stack) IsInstalled(id string) bool {
	return s.State.Addons[id] != nil
}

// writeFile replaces a file atomically.
func writeFile(path string, data []byte, perm os.FileMode) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

func systemTZ() string {
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if tz := string(bytes.TrimSpace(data)); tz != "" {
			return tz
		}
	}
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		const prefix = "/usr/share/zoneinfo/"
		if i := bytes.Index([]byte(link), []byte(prefix)); i >= 0 {
			return link[i+len(prefix):]
		}
	}
	return "UTC"
}

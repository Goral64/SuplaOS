// Package stack turns the installed add-ons into one docker compose project
// in <root>/stack and runs docker compose on it.
package stack

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
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
	// Stdin, Stdout and Stderr of docker compose; nil means those of this
	// process.
	Stdin          io.Reader
	Stdout, Stderr io.Writer

	lock *os.File
	// after are the option actions Up runs, queued by Install and
	// Configure.
	after []addon.After
	// composeHook runs docker compose in tests.
	composeHook func(stdin io.Reader, stdout io.Writer, args ...string) error
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

func (s *Stack) statePath() string { return filepath.Join(s.Root, "state.yaml") }
func (s *Stack) Dir() string       { return filepath.Join(s.Root, "stack") }
func (s *Stack) BackupDir() string { return filepath.Join(s.Root, "backups") }
func (s *Stack) cacheDir(id string) string {
	return filepath.Join(s.Root, "cache", id)
}

// Lock takes an exclusive lock on the data directory, held until the
// process exits or Unlock. Taken by every command that changes something.
func (s *Stack) Lock() error {
	if err := s.mkdir(s.Root); err != nil {
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
	s.chown(f.Name())
	s.lock = f
	return nil
}

// Unlock releases the lock taken by Lock.
func (s *Stack) Unlock() {
	if s.lock != nil {
		s.lock.Close()
		s.lock = nil
	}
}

// Save writes state.yaml. It holds secrets, so it is readable only by the
// owner.
func (s *Stack) Save() error {
	data, err := s.stateYAML()
	if err != nil {
		return err
	}
	return s.writeFile(s.statePath(), data, 0o600)
}

func (s *Stack) stateYAML() ([]byte, error) {
	var buf bytes.Buffer
	enc := yaml.NewEncoder(&buf)
	enc.SetIndent(2)
	if err := enc.Encode(s.State); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

// IsInstalled tells whether the add-on is installed.
func (s *Stack) IsInstalled(id string) bool {
	return s.State.Addons[id] != nil
}

// writeFile replaces a file atomically.
func (s *Stack) writeFile(path string, data []byte, perm os.FileMode) error {
	if err := s.mkdir(filepath.Dir(path)); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, perm); err != nil {
		return err
	}
	if err := os.Chmod(tmp, perm); err != nil {
		return err
	}
	s.chown(tmp)
	return os.Rename(tmp, path)
}

// mkdir creates a directory with its parents inside the data directory.
func (s *Stack) mkdir(dir string) error {
	var created []string
	for d := dir; ; d = filepath.Dir(d) {
		if _, err := os.Stat(d); err == nil || d == filepath.Dir(d) {
			break
		}
		created = append(created, d)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	for _, d := range created {
		s.chown(d)
	}
	return nil
}

// chown gives a file created by root (the panel) to the owner of the data
// directory, so that the suplaos command keeps working for that user.
func (s *Stack) chown(path string) {
	if os.Geteuid() != 0 || !strings.HasPrefix(path, s.Root) {
		return
	}
	st, err := os.Stat(s.Root)
	if err != nil {
		return
	}
	if sys, ok := st.Sys().(*syscall.Stat_t); ok {
		os.Lchown(path, int(sys.Uid), int(sys.Gid))
	}
}

func systemTZ() string {
	if link, err := os.Readlink("/etc/localtime"); err == nil {
		if _, tz, ok := strings.Cut(link, "zoneinfo/"); ok {
			return tz
		}
	}
	if data, err := os.ReadFile("/etc/timezone"); err == nil {
		if tz := string(bytes.TrimSpace(data)); tz != "" {
			return tz
		}
	}
	return "UTC"
}

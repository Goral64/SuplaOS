package stack

import (
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// Compose runs docker compose in the stack directory with the standard
// streams of this process.
func (s *Stack) Compose(args ...string) error {
	return s.compose(os.Stdin, os.Stdout, args...)
}

func (s *Stack) compose(stdin io.Reader, stdout io.Writer, args ...string) error {
	if len(s.State.Addons) == 0 {
		return fmt.Errorf("no add-ons installed")
	}
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = s.Dir()
	cmd.Env = s.environ()
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("docker compose %s: %w", strings.Join(args, " "), err)
	}
	return nil
}

// environ is the environment of this process without the variables set in
// .env: compose prefers the environment, and .env has to win.
func (s *Stack) environ() []string {
	p, err := s.Plan()
	if err != nil {
		return os.Environ()
	}
	skip := map[string]bool{}
	for _, kv := range p.Env {
		skip[kv.Key] = true
	}
	var out []string
	for _, e := range os.Environ() {
		k, _, _ := strings.Cut(e, "=")
		if !skip[k] {
			out = append(out, e)
		}
	}
	return out
}

// Up creates and starts the containers of all installed add-ons and
// removes containers of add-ons no longer installed.
func (s *Stack) Up() error {
	return s.Compose("up", "-d", "--remove-orphans")
}

// Services returns the compose services of the add-ons, all installed ones
// when ids is empty.
func (s *Stack) Services(ids ...string) ([]string, error) {
	if len(ids) == 0 {
		for _, a := range s.Order() {
			ids = append(ids, a.ID)
		}
	}
	var out []string
	for _, id := range ids {
		if !s.IsInstalled(id) {
			return nil, fmt.Errorf("%s is not installed", id)
		}
		out = append(out, s.Catalog[id].Services...)
	}
	return out, nil
}

// Backup dumps the data of an add-on to a gzipped file in the backup
// directory and returns its path.
func (s *Stack) Backup(id string) (string, error) {
	a := s.Catalog[id]
	if a == nil || !s.IsInstalled(id) {
		return "", fmt.Errorf("%s is not installed", id)
	}
	if a.Backup == nil {
		return "", fmt.Errorf("%s has no backup", id)
	}
	name := strings.ReplaceAll(a.Backup.File, "{time}", time.Now().Format("20060102150405"))
	if !strings.HasSuffix(name, ".gz") {
		name += ".gz"
	}
	if err := os.MkdirAll(s.BackupDir(), 0o700); err != nil {
		return "", err
	}
	path := filepath.Join(s.BackupDir(), name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	gz := gzip.NewWriter(f)
	args := append([]string{"exec", "-T", a.Backup.Service}, a.Backup.Command...)
	err = s.compose(nil, gz, args...)
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		os.Remove(path)
		return "", err
	}
	return path, nil
}

// Run executes an add-on command in its container.
func (s *Stack) Run(id, name string, args []string) error {
	a := s.Catalog[id]
	if a == nil || !s.IsInstalled(id) {
		return fmt.Errorf("%s is not installed", id)
	}
	c, ok := a.Commands[name]
	if !ok {
		return fmt.Errorf("%s has no command %q", id, name)
	}
	cargs := []string{"exec"}
	if !isTerminal(os.Stdin) {
		cargs = append(cargs, "-T")
	}
	if c.User != "" {
		cargs = append(cargs, "-u", c.User)
	}
	cargs = append(cargs, c.Service)
	cargs = append(cargs, c.Exec...)
	return s.Compose(append(cargs, args...)...)
}

func isTerminal(f *os.File) bool {
	st, err := f.Stat()
	return err == nil && st.Mode()&os.ModeCharDevice != 0
}

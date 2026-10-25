package stack

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// Compose runs docker compose in the stack directory with the standard
// streams of this process.
func (s *Stack) Compose(args ...string) error {
	var stdin io.Reader = os.Stdin
	if s.Stdin != nil {
		stdin = s.Stdin
	}
	return s.compose(stdin, s.stdout(), args...)
}

func (s *Stack) stdout() io.Writer {
	if s.Stdout != nil {
		return s.Stdout
	}
	return os.Stdout
}

func (s *Stack) stderr() io.Writer {
	if s.Stderr != nil {
		return s.Stderr
	}
	return os.Stderr
}

func (s *Stack) compose(stdin io.Reader, stdout io.Writer, args ...string) error {
	if len(s.State.Addons) == 0 {
		return fmt.Errorf("no add-ons installed")
	}
	if s.composeHook != nil {
		return s.composeHook(stdin, stdout, args...)
	}
	cmd := exec.Command("docker", append([]string{"compose"}, args...)...)
	cmd.Dir = s.Dir()
	cmd.Env = s.environ()
	cmd.Stdin = stdin
	cmd.Stdout = stdout
	cmd.Stderr = s.stderr()
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

// Up creates and starts the containers of all installed add-ons, removes
// containers of add-ons no longer installed and runs the queued option
// actions.
func (s *Stack) Up() error {
	if err := s.Compose("up", "-d", "--remove-orphans"); err != nil {
		return err
	}
	return s.RunAfter()
}

// RunAfter runs the queued option actions. The service may still be
// starting (e.g. a database), so the command is retried for a while. An
// action without a command only restarts services (a new certificate).
func (s *Stack) RunAfter() error {
	acts := s.after
	s.after = nil
	for _, act := range acts {
		var err error
		for try := 0; try < 10 && len(act.Command) > 0; try++ {
			if try > 0 {
				time.Sleep(3 * time.Second)
			}
			if err = s.compose(nil, s.stdout(), append([]string{"exec", "-T", act.Service}, act.Command...)...); err == nil {
				break
			}
		}
		if err != nil {
			return err
		}
		if len(act.Restart) > 0 {
			if err := s.Compose(append([]string{"restart"}, act.Restart...)...); err != nil {
				return err
			}
		}
	}
	return nil
}

// RemoveNetwork deletes the network of the project. Needed after the last
// add-on is removed: without compose files compose cannot do it.
func (s *Stack) RemoveNetwork() {
	exec.Command("docker", "network", "rm", ProjectName+"_default").Run()
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
	if err := s.mkdir(s.BackupDir()); err != nil {
		return "", err
	}
	// Two backups in the same second (a restore right after a backup) get
	// a number after the time.
	stamp := time.Now().Format("20060102150405")
	var path string
	var f *os.File
	var err error
	for n := 0; ; n++ {
		t := stamp
		if n > 0 {
			t = fmt.Sprintf("%s-%d", stamp, n)
		}
		name := strings.ReplaceAll(a.Backup.File, "{time}", t)
		if !strings.HasSuffix(name, ".gz") {
			name += ".gz"
		}
		path = filepath.Join(s.BackupDir(), name)
		f, err = os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
		if err == nil {
			break
		}
		if !errors.Is(err, os.ErrExist) || n > 100 {
			return "", err
		}
	}
	s.chown(path)
	gz := gzip.NewWriter(f)
	err = s.dump(a, gz)
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

// Exec runs a command in the container of a service, with stdin and
// stdout given.
func (s *Stack) Exec(service string, stdin io.Reader, stdout io.Writer, cmd ...string) error {
	return s.compose(stdin, stdout, append([]string{"exec", "-T", service}, cmd...)...)
}

// dump writes the database dump of an add-on to w.
func (s *Stack) dump(a *addon.Addon, w io.Writer) error {
	return s.compose(nil, w, append([]string{"exec", "-T", a.Backup.Service}, a.Backup.Command...)...)
}

// Restore loads a backup of an add-on made by Backup. The current data is
// backed up first and loaded back when the restore fails; the other
// services of the add-on are stopped meanwhile.
func (s *Stack) Restore(id, path string) error {
	a := s.Catalog[id]
	if a == nil || !s.IsInstalled(id) {
		return fmt.Errorf("%s is not installed", id)
	}
	if a.Backup == nil || len(a.Backup.Restore) == 0 {
		return fmt.Errorf("%s has no restore", id)
	}
	if err := checkBackup(path, filepath.Base(path)); err != nil {
		return err
	}
	fmt.Fprintln(s.stdout(), "Backing up the current data")
	current, err := s.Backup(id)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.stdout(), "Backup saved to %s\n", current)
	var others []string
	for _, svc := range a.Services {
		if svc != a.Backup.Service {
			others = append(others, svc)
		}
	}
	if len(others) > 0 {
		if err := s.Compose(append([]string{"stop"}, others...)...); err != nil {
			return err
		}
	}
	fmt.Fprintf(s.stdout(), "Restoring %s\n", filepath.Base(path))
	err = s.load(a, path)
	if err != nil {
		fmt.Fprintf(s.stdout(), "Restore failed (%v), loading %s back\n", err, filepath.Base(current))
		if lerr := s.load(a, current); lerr != nil {
			fmt.Fprintf(s.stdout(), "Loading it back failed too: %v\n", lerr)
		}
	}
	if len(others) > 0 {
		if uerr := s.Compose(append([]string{"up", "-d"}, others...)...); err == nil {
			err = uerr
		}
	}
	return err
}

// load feeds a gzipped dump to the restore command of an add-on.
func (s *Stack) load(a *addon.Addon, path string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err != nil {
		return err
	}
	return s.loadDump(a, gz)
}

// loadDump feeds a dump to the restore command of an add-on.
func (s *Stack) loadDump(a *addon.Addon, r io.Reader) error {
	// The restore command is a script, too long for the error message.
	args := append([]string{"exec", "-T", a.Backup.Service}, a.Backup.Restore...)
	if err := s.compose(r, s.stdout(), args...); err != nil {
		return fmt.Errorf("restore of %s: %w", a.ID, errors.Unwrap(err))
	}
	return nil
}

// checkBackup reads a gzipped backup through, so that a damaged or
// truncated file is not loaded in place of the data.
func checkBackup(path, name string) error {
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	gz, err := gzip.NewReader(f)
	if err == nil {
		_, err = io.Copy(io.Discard, gz)
	}
	if err != nil {
		return fmt.Errorf("%s is not a valid backup: %w", name, err)
	}
	return nil
}

// SaveBackup stores a backup read from r under name in the backup
// directory, only when it is a whole gzip file.
func (s *Stack) SaveBackup(name string, r io.Reader) error {
	if err := s.mkdir(s.BackupDir()); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.BackupDir(), ".upload-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = io.Copy(f, r)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = checkBackup(f.Name(), name)
	}
	if err != nil {
		return err
	}
	s.chown(f.Name())
	return os.Rename(f.Name(), filepath.Join(s.BackupDir(), name))
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
	if s.Stdin != nil || !isTerminal(os.Stdin) {
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

// Command prepares docker compose for the stack directory; the caller sets
// its output and runs it. The context stops it, e.g. for logs -f.
func (s *Stack) Command(ctx context.Context, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, "docker", append([]string{"compose"}, args...)...)
	cmd.Dir = s.Dir()
	cmd.Env = s.environ()
	return cmd
}

// Container is one container of the project as docker compose ps shows it.
type Container struct {
	Name    string `json:"Name"`
	Service string `json:"Service"`
	State   string `json:"State"`
	Health  string `json:"Health"`
	Status  string `json:"Status"`
}

// Containers lists the containers of the installed add-ons, stopped ones
// too.
func (s *Stack) Containers(ctx context.Context) ([]Container, error) {
	if len(s.State.Addons) == 0 {
		return nil, nil
	}
	cmd := s.Command(ctx, "ps", "--all", "--format", "json")
	out, err := cmd.Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose ps: %w", err)
	}
	var list []Container
	// One JSON object per line; older compose versions print an array.
	if t := strings.TrimSpace(string(out)); strings.HasPrefix(t, "[") {
		err := json.Unmarshal([]byte(t), &list)
		return list, err
	}
	sc := bufio.NewScanner(strings.NewReader(string(out)))
	for sc.Scan() {
		if strings.TrimSpace(sc.Text()) == "" {
			continue
		}
		var c Container
		if err := json.Unmarshal(sc.Bytes(), &c); err != nil {
			return nil, err
		}
		list = append(list, c)
	}
	return list, nil
}

// ServiceAddress returns the address of the container of a service in the
// network of the project, as the host reaches it.
func (s *Stack) ServiceAddress(ctx context.Context, service string) (string, error) {
	out, err := s.Command(ctx, "ps", "-q", service).Output()
	if err != nil {
		return "", fmt.Errorf("docker compose ps: %w", err)
	}
	id := strings.TrimSpace(string(out))
	if id == "" {
		return "", fmt.Errorf("%s is not running", service)
	}
	out, err = exec.CommandContext(ctx, "docker", "inspect", "--format",
		`{{range .NetworkSettings.Networks}}{{.IPAddress}} {{end}}`, id).Output()
	if err != nil {
		return "", fmt.Errorf("docker inspect: %w", err)
	}
	if f := strings.Fields(string(out)); len(f) > 0 {
		return f[0], nil
	}
	return "", fmt.Errorf("%s has no address", service)
}

package stack

import (
	"bufio"
	"compress/gzip"
	"context"
	"encoding/json"
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
	name := strings.ReplaceAll(a.Backup.File, "{time}", time.Now().Format("20060102150405"))
	if !strings.HasSuffix(name, ".gz") {
		name += ".gz"
	}
	if err := s.mkdir(s.BackupDir()); err != nil {
		return "", err
	}
	path := filepath.Join(s.BackupDir(), name)
	f, err := os.OpenFile(path, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0o600)
	if err != nil {
		return "", err
	}
	s.chown(path)
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

package stack

import (
	"archive/tar"
	"bytes"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// ImportOptions of a restore: the password of the backup and the version
// of this SuplaOS, which may not be older than the one that made it.
type ImportOptions struct {
	Password string
	Version  string
}

// CheckFullBackup reads a full backup through before anything is changed:
// a damaged or truncated file, a wrong password, a newer SuplaOS or add-ons
// this one does not know are found here.
func (s *Stack) CheckFullBackup(file string, opt ImportOptions) (*Manifest, error) {
	f, err := os.Open(file)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	tr, m, err := OpenFullBackup(f, opt.Password)
	if err != nil {
		return nil, err
	}
	if opt.Version != "" && m.Version != "" && versionNewer(m.Version, opt.Version) {
		return nil, fmt.Errorf("the backup was made by SuplaOS %s, this is %s: update first", m.Version, opt.Version)
	}
	for _, id := range m.Addons {
		if s.Catalog[id] == nil {
			return nil, fmt.Errorf("the backup has the add-on %s this SuplaOS does not know: update first", id)
		}
	}
	state := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("the backup is damaged: %w", err)
		}
		if _, err := restorePath(hdr.Name, m, s.Catalog); err != nil {
			return nil, err
		}
		if hdr.Name == "state.yaml" {
			state = true
		}
		if _, err := io.Copy(io.Discard, tr); err != nil {
			return nil, fmt.Errorf("the backup is damaged: %w", err)
		}
	}
	if !state {
		return nil, errors.New("not a SuplaOS backup: no state.yaml")
	}
	return m, nil
}

// restorePath returns where an entry of a backup goes, relative to the data
// directory. Only the state, the CA, the dumps and the data directories of
// the add-ons in the backup are written: a file elsewhere (the stack
// directory is rendered anew) is an error.
func restorePath(name string, m *Manifest, c addon.Catalog) (string, error) {
	clean := path.Clean(strings.TrimSuffix(name, "/"))
	bad := fmt.Errorf("the backup has an unexpected file %q", name)
	if !filepath.IsLocal(clean) {
		return "", bad
	}
	switch {
	case clean == "state.yaml", clean == "ca", strings.HasPrefix(clean, "ca/"):
		return clean, nil
	case strings.HasPrefix(clean, "dumps/"):
		id := strings.TrimSuffix(strings.TrimPrefix(clean, "dumps/"), ".sql")
		if !slices.Contains(m.Addons, id) || c[id].Backup == nil {
			return "", bad
		}
		return filepath.Join("backups", ".restore-"+id+".sql"), nil
	case strings.HasPrefix(clean, "stack/"):
		rel := strings.TrimPrefix(clean, "stack/")
		for _, id := range m.Addons {
			for _, d := range c[id].Data {
				d = path.Clean(d)
				if rel == d || strings.HasPrefix(rel, d+"/") {
					return clean, nil
				}
			}
		}
	}
	return "", bad
}

// Import replaces the add-ons, their data and the CA of the device with a
// full backup. The current state is backed up to the backup directory
// first. The add-on files are downloaded and the images pulled again, so
// it needs the internet.
func (s *Stack) Import(file string, opt ImportOptions) error {
	m, err := s.CheckFullBackup(file, opt)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.stdout(), "Backup of %s from %s, SuplaOS %s: %s\n", m.Hostname,
		m.Created.Local().Format("2006-01-02 15:04"), m.Version, strings.Join(m.Addons, ", "))

	if len(s.State.Addons) > 0 {
		fmt.Fprintln(s.stdout(), "Backing up the current state")
		current, err := s.SaveFullBackup(ExportOptions{Version: opt.Version})
		if err != nil {
			return err
		}
		fmt.Fprintf(s.stdout(), "Saved to %s\n", current)
		if err := s.Compose("down", "--remove-orphans"); err != nil {
			return err
		}
	}

	root, err := os.OpenRoot(s.Root)
	if err != nil {
		return err
	}
	defer root.Close()
	if err := s.clear(root, m); err != nil {
		return err
	}
	fmt.Fprintln(s.stdout(), "Restoring the files")
	if err := s.extract(root, file, opt.Password, m); err != nil {
		return err
	}
	defer func() {
		for _, id := range m.Addons {
			root.Remove(filepath.Join("backups", ".restore-"+id+".sql"))
		}
	}()

	data, err := root.ReadFile("state.yaml")
	if err != nil {
		return err
	}
	st := &State{}
	if err := yaml.Unmarshal(data, st); err != nil {
		return fmt.Errorf("state.yaml of the backup: %w", err)
	}
	if st.Addons == nil {
		st.Addons = map[string]*Installed{}
	}
	for _, inst := range st.Addons {
		if inst.Options == nil {
			inst.Options = map[string]string{}
		}
	}
	s.State = st
	if err := s.Refetch(); err != nil {
		return err
	}
	if err := s.Apply(); err != nil {
		return err
	}

	for _, a := range s.Order() {
		if a.Backup == nil || len(a.Backup.Restore) == 0 {
			continue
		}
		dump := filepath.Join(s.Root, "backups", ".restore-"+a.ID+".sql")
		if _, err := os.Stat(dump); err != nil {
			continue
		}
		fmt.Fprintf(s.stdout(), "Loading the database of %s\n", a.ID)
		if err := s.Compose("up", "-d", a.Backup.Service); err != nil {
			return err
		}
		if err := s.loadNew(a, dump); err != nil {
			return err
		}
	}
	fmt.Fprintln(s.stdout(), "Starting the add-ons")
	// All containers are created anew: the restarts Apply queued for new
	// certificates and files are not needed.
	s.after = nil
	if err := s.Up(); err != nil {
		return err
	}
	s.importWarnings(m)
	return nil
}

// clear deletes the data the backup replaces: the state, the CA and the
// data directories of the add-ons installed now and of those in the backup.
func (s *Stack) clear(root *os.Root, m *Manifest) error {
	ids := slices.Clone(m.Addons)
	for id := range s.State.Addons {
		ids = append(ids, id)
	}
	for _, id := range ids {
		for _, d := range s.Catalog[id].Data {
			if err := root.RemoveAll(filepath.Join("stack", d)); err != nil {
				return err
			}
		}
	}
	if err := root.RemoveAll("ca"); err != nil {
		return err
	}
	return root.RemoveAll("state.yaml")
}

// extract writes the entries of a backup in place, with their mode and,
// as root, their owner. os.Root keeps everything inside the data
// directory, also through symbolic links in the backup.
func (s *Stack) extract(root *os.Root, file, password string, m *Manifest) error {
	f, err := os.Open(file)
	if err != nil {
		return err
	}
	defer f.Close()
	tr, _, err := OpenFullBackup(f, password)
	if err != nil {
		return err
	}
	asRoot := os.Geteuid() == 0
	type dirTime struct {
		name string
		t    time.Time
	}
	var dirs []dirTime
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return err
		}
		name, err := restorePath(hdr.Name, m, s.Catalog)
		if err != nil {
			return err
		}
		if dir := filepath.Dir(name); dir != "." {
			if err := root.MkdirAll(dir, 0o755); err != nil {
				return err
			}
		}
		mode := fs.FileMode(hdr.Mode).Perm()
		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := root.Mkdir(name, mode); err != nil && !errors.Is(err, fs.ErrExist) {
				return err
			}
			if err := root.Chmod(name, mode); err != nil {
				return err
			}
			dirs = append(dirs, dirTime{name, hdr.ModTime})
		case tar.TypeReg:
			out, err := root.OpenFile(name, os.O_CREATE|os.O_TRUNC|os.O_WRONLY, mode)
			if err != nil {
				return err
			}
			_, err = io.Copy(out, tr)
			if cerr := out.Close(); err == nil {
				err = cerr
			}
			if err != nil {
				return err
			}
			if err := root.Chmod(name, mode); err != nil {
				return err
			}
			root.Chtimes(name, hdr.ModTime, hdr.ModTime)
		case tar.TypeSymlink:
			root.Remove(name)
			if err := root.Symlink(hdr.Linkname, name); err != nil {
				return err
			}
		default:
			continue
		}
		switch {
		case asRoot && strings.HasPrefix(name, "stack/"):
			root.Lchown(name, hdr.Uid, hdr.Gid)
		default:
			s.chown(filepath.Join(s.Root, name))
		}
	}
	// Writing into a directory changes its time: set them last.
	for _, d := range dirs {
		root.Chtimes(d.name, d.t, d.t)
	}
	return nil
}

// loadNew loads a dump into a database just started on an empty data
// directory: it takes a while to initialize (minutes on a Raspberry Pi), so
// loading is tried again for up to loadWait. The restore command drops what
// is there first, so a failed try does no harm.
func (s *Stack) loadNew(a *addon.Addon, dump string) error {
	// The errors of the tries before it is ready are not shown, only those
	// of the last one.
	stderr := s.Stderr
	defer func() { s.Stderr = stderr }()
	var msg bytes.Buffer
	var err error
	start := time.Now()
	said := time.Duration(0)
	for try := 0; try == 0 || time.Since(start) < loadWait; try++ {
		if try > 0 {
			// A line now and then, so that the wait does not look stuck.
			if w := time.Since(start).Truncate(time.Minute); try == 1 || w > said {
				fmt.Fprintln(s.stdout(), "Waiting for the database")
				said = w
			}
			time.Sleep(loadRetry)
		}
		var f *os.File
		if f, err = os.Open(dump); err != nil {
			return err
		}
		msg.Reset()
		s.Stderr = &msg
		err = s.loadDump(a, f)
		f.Close()
		if err == nil {
			return nil
		}
	}
	s.Stderr = stderr
	s.stderr().Write(msg.Bytes())
	return err
}

// loadRetry is the pause between tries of loadNew, loadWait how long it
// tries.
var (
	loadRetry = 3 * time.Second
	loadWait  = 10 * time.Minute
)

// importWarnings points at options that may not fit the new device: a
// serial port that is not there, an address of the old device.
func (s *Stack) importWarnings(m *Manifest) {
	_, ips := deviceNames()
	var current []string
	for _, ip := range ips {
		current = append(current, ip.String())
	}
	for _, a := range s.Order() {
		values := s.Values(a)
		for i := range a.Options {
			o := &a.Options[i]
			v := values[o.Key]
			if v == "" || !s.Active(o, values) {
				continue
			}
			if o.Type == "serial" {
				if _, err := os.Stat(v); err != nil {
					fmt.Fprintf(s.stdout(), "Warning: %s %s: %s is not on this device, set the coordinator anew\n", a.ID, o.Key, v)
				}
				continue
			}
			for _, old := range m.Addresses {
				if !slices.Contains(current, old) && containsAddress(v, old) {
					fmt.Fprintf(s.stdout(), "Warning: %s %s is %s, the address of the old device\n", a.ID, o.Key, v)
				}
			}
		}
	}
}

// containsAddress tells whether the IP address ip is in v as a whole, not
// a part of a longer one (192.168.1.2 in 192.168.1.25).
func containsAddress(v, ip string) bool {
	for i := 0; ; {
		j := strings.Index(v[i:], ip)
		if j < 0 {
			return false
		}
		start, end := i+j, i+j+len(ip)
		isPart := func(k int) bool { return k >= 0 && k < len(v) && (v[k] == '.' || v[k] >= '0' && v[k] <= '9') }
		if !isPart(start-1) && !isPart(end) {
			return true
		}
		i = start + 1
	}
}

// versionNewer tells whether the SuplaOS version a is newer than b, by the
// numbers before a "~" (development builds count as their release).
func versionNewer(a, b string) bool {
	nums := func(v string) []int {
		v, _, _ = strings.Cut(v, "~")
		v, _, _ = strings.Cut(v, "+")
		var n []int
		for _, p := range strings.Split(v, ".") {
			x, _ := strconv.Atoi(p)
			n = append(n, x)
		}
		return n
	}
	x, y := nums(a), nums(b)
	for i := 0; i < len(x) || i < len(y); i++ {
		var p, q int
		if i < len(x) {
			p = x[i]
		}
		if i < len(y) {
			q = y[i]
		}
		if p != q {
			return p > q
		}
	}
	return false
}

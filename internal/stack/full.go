package stack

import (
	"archive/tar"
	"bufio"
	"bytes"
	"compress/gzip"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"filippo.io/age"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// FullFormat is the version of the format of full backups.
const FullFormat = 1

// A full backup is a tar.gz, encrypted with age when it has a password:
//
//	manifest.json      Manifest, always first
//	state.yaml         installed add-ons and their options
//	ca/...             CA of the device, own domain settings and certificate
//	dumps/<id>.sql     database dumps of the add-ons with a backup
//	stack/<data>/...   data directories of the add-ons, without full_backup skip
//
// The stack directory is rendered anew on restore, only the data
// directories inside it are kept.

// Manifest describes a full backup.
type Manifest struct {
	Format int `json:"format"`
	// Version of SuplaOS that made it.
	Version  string    `json:"version"`
	Created  time.Time `json:"created"`
	Hostname string    `json:"hostname"`
	// Addresses of the device then, to find them in options on restore.
	Addresses []string `json:"addresses"`
	Arch      string   `json:"arch"`
	Addons    []string `json:"addons"`
}

// ExportOptions of a full backup. Without Password it is not encrypted.
type ExportOptions struct {
	Version  string
	Password string
}

// scryptWorkFactor of the password: 2^16 needs 64 MB of memory, which a
// Raspberry Pi with 1 GB and running add-ons can spare (the age default,
// 2^18, takes 256 MB).
const scryptWorkFactor = 16

// FullBackupName is the file name of a full backup.
func FullBackupName(host string, t time.Time, encrypted bool) string {
	name := fmt.Sprintf("suplaos-%s-%s.tar.gz", host, t.Format("20060102150405"))
	if encrypted {
		name += ".age"
	}
	return name
}

// IsFullBackup tells by its name whether a file is a full backup.
func IsFullBackup(name string) bool {
	return strings.HasPrefix(name, "suplaos-") && (strings.HasSuffix(name, ".tar.gz") || strings.HasSuffix(name, ".tar.gz.age"))
}

// SaveFullBackup writes a full backup to the backup directory and returns
// its path. It holds secrets, so it is readable only by the owner.
func (s *Stack) SaveFullBackup(opt ExportOptions) (string, error) {
	if err := s.mkdir(s.BackupDir()); err != nil {
		return "", err
	}
	host, _ := os.Hostname()
	path := filepath.Join(s.BackupDir(), FullBackupName(host, time.Now(), opt.Password != ""))
	if err := s.ExportFile(path, opt); err != nil {
		return "", err
	}
	return path, nil
}

// ExportFile writes a full backup to path, through a temporary file, so
// that a failed one leaves nothing behind.
func (s *Stack) ExportFile(path string, opt ExportOptions) error {
	f, err := os.CreateTemp(filepath.Dir(path), ".export-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	_, err = s.Export(f, opt)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err != nil {
		return err
	}
	s.chown(f.Name())
	return os.Rename(f.Name(), path)
}

// Export writes a full backup of the device to w. Databases are dumped
// while they run; add-ons with full_backup stop are stopped while their
// files are copied and started again.
func (s *Stack) Export(w io.Writer, opt ExportOptions) (*Manifest, error) {
	host, _ := os.Hostname()
	m := &Manifest{Format: FullFormat, Version: opt.Version, Created: time.Now().UTC(),
		Hostname: host, Addresses: []string{}, Arch: runtime.GOARCH, Addons: []string{}}
	_, ips := deviceNames()
	for _, ip := range ips {
		if !ip.IsLoopback() {
			m.Addresses = append(m.Addresses, ip.String())
		}
	}
	order := s.Order()
	for _, a := range order {
		m.Addons = append(m.Addons, a.ID)
	}

	out := w
	var enc io.WriteCloser
	if opt.Password != "" {
		r, err := age.NewScryptRecipient(opt.Password)
		if err != nil {
			return nil, err
		}
		r.SetWorkFactor(scryptWorkFactor)
		if enc, err = age.Encrypt(w, r); err != nil {
			return nil, err
		}
		out = enc
	}
	gz := gzip.NewWriter(out)
	tw := tar.NewWriter(gz)

	err := s.exportTo(tw, m, order)
	if cerr := tw.Close(); err == nil {
		err = cerr
	}
	if cerr := gz.Close(); err == nil {
		err = cerr
	}
	if enc != nil {
		if cerr := enc.Close(); err == nil {
			err = cerr
		}
	}
	if err != nil {
		return nil, err
	}
	return m, nil
}

func (s *Stack) exportTo(tw *tar.Writer, m *Manifest, order []*addon.Addon) error {
	data, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	if err := addBytes(tw, "manifest.json", data); err != nil {
		return err
	}
	state, err := s.stateYAML()
	if err != nil {
		return err
	}
	if err := addBytes(tw, "state.yaml", state); err != nil {
		return err
	}
	// The certificate of the panel is made anew for the new name.
	if err := addTree(tw, s.CADir(), "ca", []string{"panel"}); err != nil {
		return err
	}
	for _, a := range order {
		if a.Backup == nil {
			continue
		}
		fmt.Fprintf(s.stdout(), "Dumping the database of %s\n", a.ID)
		if err := s.addDump(tw, a); err != nil {
			return err
		}
	}
	for _, a := range order {
		if err := s.exportData(tw, a); err != nil {
			return err
		}
	}
	return nil
}

// addDump dumps the database of an add-on to a temporary file first: tar
// needs the size before the content.
func (s *Stack) addDump(tw *tar.Writer, a *addon.Addon) error {
	if err := s.mkdir(s.BackupDir()); err != nil {
		return err
	}
	f, err := os.CreateTemp(s.BackupDir(), ".dump-*")
	if err != nil {
		return err
	}
	defer os.Remove(f.Name())
	defer f.Close()
	if err := s.dump(a, f); err != nil {
		return err
	}
	return addFile(tw, f.Name(), "dumps/"+a.ID+".sql")
}

// exportData adds the data directories of an add-on, its services stopped
// meanwhile with full_backup stop.
func (s *Stack) exportData(tw *tar.Writer, a *addon.Addon) error {
	if a.FullBackup.Stop {
		running, err := s.running(a)
		if err != nil {
			return err
		}
		if len(running) > 0 {
			fmt.Fprintf(s.stdout(), "Stopping %s for a moment\n", a.ID)
			if err := s.Compose(append([]string{"stop"}, running...)...); err != nil {
				return err
			}
			defer func() {
				if err := s.Compose(append([]string{"start"}, running...)...); err != nil {
					fmt.Fprintf(s.stdout(), "Starting %s failed: %v\n", a.ID, err)
				}
			}()
		}
	}
	for _, d := range a.Data {
		var skip []string
		for _, sk := range a.FullBackup.Skip {
			if rel, err := filepath.Rel(d, sk); err == nil && filepath.IsLocal(rel) {
				skip = append(skip, rel)
			}
		}
		if err := addTree(tw, filepath.Join(s.Dir(), d), "stack/"+filepath.ToSlash(filepath.Clean(d)), skip); err != nil {
			return err
		}
	}
	return nil
}

// running returns the services of an add-on that are running now.
func (s *Stack) running(a *addon.Addon) ([]string, error) {
	var out bytes.Buffer
	if err := s.compose(nil, &out, "ps", "--services", "--status", "running"); err != nil {
		return nil, err
	}
	var list []string
	for _, svc := range strings.Fields(out.String()) {
		for _, own := range a.Services {
			if svc == own {
				list = append(list, svc)
			}
		}
	}
	return list, nil
}

func addBytes(tw *tar.Writer, name string, data []byte) error {
	hdr := &tar.Header{Name: name, Mode: 0o600, Size: int64(len(data)), ModTime: time.Now(), Format: tar.FormatPAX}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	_, err := tw.Write(data)
	return err
}

func addFile(tw *tar.Writer, path, name string) error {
	fi, err := os.Lstat(path)
	if err != nil {
		return err
	}
	return addEntry(tw, path, name, fi)
}

// addTree adds the directory root as name, without the paths in skip
// (relative to root, "." for all of it). A missing directory is left out:
// an add-on that has not written it yet.
func addTree(tw *tar.Writer, root, name string, skip []string) error {
	return filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			// Gone meanwhile (a temporary file of a running service).
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		rel, err := filepath.Rel(root, path)
		if err != nil {
			return err
		}
		for _, sk := range skip {
			if sk == "." || rel == sk || strings.HasPrefix(rel, sk+"/") {
				if d.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
		}
		fi, err := d.Info()
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		if err != nil {
			return err
		}
		entry := name
		if rel != "." {
			entry = name + "/" + filepath.ToSlash(rel)
		}
		return addEntry(tw, path, entry, fi)
	})
}

// addEntry adds a file, directory or symbolic link with its mode and
// owner (files of containers belong to their users). Sockets and the like
// are left out.
func addEntry(tw *tar.Writer, path, name string, fi fs.FileInfo) error {
	var link string
	switch {
	case fi.Mode().IsRegular(), fi.IsDir():
	case fi.Mode()&fs.ModeSymlink != 0:
		var err error
		if link, err = os.Readlink(path); err != nil {
			return err
		}
	default:
		return nil
	}
	hdr, err := tar.FileInfoHeader(fi, link)
	if err != nil {
		return err
	}
	hdr.Name = name
	if fi.IsDir() {
		hdr.Name += "/"
	}
	hdr.Uname, hdr.Gname = "", ""
	hdr.Format = tar.FormatPAX
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if !fi.Mode().IsRegular() {
		return nil
	}
	f, err := os.Open(path)
	if err != nil {
		return err
	}
	defer f.Close()
	// A file of a running service may change size meanwhile: exactly the
	// size from the header is written, padded with zeros when it shrank.
	_, err = io.CopyN(tw, io.MultiReader(f, zeros{}), hdr.Size)
	return err
}

type zeros struct{}

func (zeros) Read(p []byte) (int, error) {
	clear(p)
	return len(p), nil
}

// ageHeader starts every age file.
const ageHeader = "age-encryption.org/v1\n"

// ErrPassword is returned for an encrypted backup without the right
// password.
var ErrPassword = errors.New("wrong password of the backup")

// OpenFullBackup returns the archive of a full backup read from r,
// decrypted when it is encrypted. Its first entry, the manifest, is read
// and checked already.
func OpenFullBackup(r io.Reader, password string) (*tar.Reader, *Manifest, error) {
	br := bufio.NewReader(r)
	head, _ := br.Peek(len(ageHeader))
	var in io.Reader = br
	if string(head) == ageHeader {
		if password == "" {
			return nil, nil, ErrPassword
		}
		id, err := age.NewScryptIdentity(password)
		if err != nil {
			return nil, nil, err
		}
		dec, err := age.Decrypt(br, id)
		var noMatch *age.NoIdentityMatchError
		if errors.As(err, &noMatch) {
			return nil, nil, ErrPassword
		}
		if err != nil {
			return nil, nil, fmt.Errorf("not a SuplaOS backup: %w", err)
		}
		in = dec
	}
	gz, err := gzip.NewReader(in)
	if err != nil {
		return nil, nil, fmt.Errorf("not a SuplaOS backup: %w", err)
	}
	tr := tar.NewReader(gz)
	hdr, err := tr.Next()
	if err != nil || hdr.Name != "manifest.json" {
		return nil, nil, errors.New("not a SuplaOS backup: no manifest")
	}
	var m Manifest
	if err := json.NewDecoder(io.LimitReader(tr, 1<<20)).Decode(&m); err != nil {
		return nil, nil, fmt.Errorf("not a SuplaOS backup: %w", err)
	}
	if m.Format < 1 || m.Format > FullFormat {
		return nil, nil, fmt.Errorf("the backup has format %d, this SuplaOS reads up to %d: update it first", m.Format, FullFormat)
	}
	return tr, &m, nil
}

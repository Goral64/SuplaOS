// Package autobackup makes full backups of the device on a schedule, to
// the disk of the device or to a network share (SMB or NFS), and keeps the
// newest few of them.
package autobackup

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"time"

	"gopkg.in/yaml.v3"

	"github.com/Goral64/SuplaOS/internal/stack"
)

// SMB is a Windows (Samba) share.
type SMB struct {
	// Share is //server/share.
	Share    string `yaml:"share" json:"share"`
	Username string `yaml:"username" json:"username"`
	Password string `yaml:"password" json:"-"`
	Domain   string `yaml:"domain" json:"domain"`
	// Version of the protocol, empty for the newest the server can.
	Version string `yaml:"version" json:"version"`
}

// NFS is an NFS export.
type NFS struct {
	// Export is server:/path.
	Export string `yaml:"export" json:"export"`
}

// Settings of the automatic backups. They hold passwords: the file is
// readable only by root, and they are not in the backups themselves.
type Settings struct {
	// Schedule is off, daily or weekly.
	Schedule string `yaml:"schedule" json:"schedule"`
	// Time of day, HH:MM, and for weekly the Weekday (0 is Sunday).
	Time    string `yaml:"time" json:"time"`
	Weekday int    `yaml:"weekday" json:"weekday"`
	// Target is device, smb or nfs.
	Target string `yaml:"target" json:"target"`
	SMB    SMB    `yaml:"smb" json:"smb"`
	NFS    NFS    `yaml:"nfs" json:"nfs"`
	// Dir is a directory on the share, empty for its top.
	Dir string `yaml:"dir" json:"dir"`
	// Keep is how many automatic backups stay at the target.
	Keep int `yaml:"keep" json:"keep"`
	// Password encrypts the backups; Plain is the choice of none.
	Password string `yaml:"password" json:"-"`
	Plain    bool   `yaml:"plain" json:"plain"`
	// Since is when the schedule was saved: a time before it is not due.
	Since time.Time `yaml:"since" json:"-"`
}

// Status is the result of the last automatic backup.
type Status struct {
	Time  time.Time `json:"time"`
	OK    bool      `json:"ok"`
	Error string    `json:"error,omitempty"`
	File  string    `json:"file,omitempty"`
	Size  int64     `json:"size,omitempty"`
}

func settingsFile(root string) string { return filepath.Join(root, "autobackup.yaml") }
func statusFile(root string) string   { return filepath.Join(root, "autobackup-status.json") }

// Default are the settings before any are saved.
func Default() *Settings {
	return &Settings{Schedule: "off", Time: "03:30", Weekday: 0, Target: "device", Keep: 7}
}

// Load reads the settings, the defaults when there are none.
func Load(root string) (*Settings, error) {
	s := Default()
	data, err := os.ReadFile(settingsFile(root))
	if errors.Is(err, os.ErrNotExist) {
		return s, nil
	}
	if err != nil {
		return nil, err
	}
	if err := yaml.Unmarshal(data, s); err != nil {
		return nil, fmt.Errorf("%s: %w", settingsFile(root), err)
	}
	return s, nil
}

// Save writes the settings, readable only by root.
func (s *Settings) Save(root string) error {
	data, err := yaml.Marshal(s)
	if err != nil {
		return err
	}
	return writeFile(settingsFile(root), data)
}

func writeFile(path string, data []byte) error {
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadStatus reads the result of the last automatic backup.
func LoadStatus(root string) Status {
	var st Status
	if data, err := os.ReadFile(statusFile(root)); err == nil {
		json.Unmarshal(data, &st)
	}
	return st
}

func saveStatus(root string, st Status) error {
	data, err := json.Marshal(st)
	if err != nil {
		return err
	}
	return writeFile(statusFile(root), data)
}

var (
	smbShare  = regexp.MustCompile(`^//[^/\s]+/[^/\x00-\x1f][^\x00-\x1f]*$`)
	nfsExport = regexp.MustCompile(`^[^:/\s]+:/\S*$`)
)

// Validate checks the settings.
func (s *Settings) Validate() error {
	switch s.Schedule {
	case "off", "daily", "weekly":
	default:
		return fmt.Errorf("unknown schedule %q", s.Schedule)
	}
	if _, err := time.Parse("15:04", s.Time); err != nil {
		return fmt.Errorf("the time has to be HH:MM, not %q", s.Time)
	}
	if s.Weekday < 0 || s.Weekday > 6 {
		return errors.New("the day of the week has to be 0 to 6")
	}
	if s.Keep < 1 || s.Keep > 100 {
		return errors.New("keep 1 to 100 backups")
	}
	switch s.Target {
	case "device":
	case "smb":
		if !smbShare.MatchString(s.SMB.Share) {
			return fmt.Errorf("the share has to be //server/share, not %q", s.SMB.Share)
		}
		if strings.ContainsAny(s.SMB.Username+s.SMB.Password+s.SMB.Domain, "\r\n") {
			return errors.New("the user, password and domain of the share cannot have line breaks")
		}
		switch s.SMB.Version {
		case "", "3.1.1", "3.0", "2.1", "2.0", "1.0":
		default:
			return fmt.Errorf("unknown SMB version %q", s.SMB.Version)
		}
	case "nfs":
		if !nfsExport.MatchString(s.NFS.Export) {
			return fmt.Errorf("the export has to be server:/path, not %q", s.NFS.Export)
		}
	default:
		return fmt.Errorf("unknown target %q", s.Target)
	}
	if s.Dir != "" && !filepath.IsLocal(s.Dir) {
		return fmt.Errorf("the directory %q has to be inside the share", s.Dir)
	}
	if s.Schedule != "off" && s.Password == "" && !s.Plain {
		return errors.New("set a password of the backups or choose backups without a password")
	}
	if s.Password != "" && len(s.Password) < 8 {
		return errors.New("the password of the backups has to be at least 8 characters")
	}
	return nil
}

// Last returns the latest scheduled time not after now.
func (s *Settings) Last(now time.Time) time.Time {
	hm, _ := time.Parse("15:04", s.Time)
	t := time.Date(now.Year(), now.Month(), now.Day(), hm.Hour(), hm.Minute(), 0, 0, now.Location())
	for t.After(now) || s.Schedule == "weekly" && int(t.Weekday()) != s.Weekday {
		t = t.AddDate(0, 0, -1)
	}
	return t
}

// Next returns the next scheduled time after now, zero when it is off.
func (s *Settings) Next(now time.Time) time.Time {
	if s.Schedule == "off" {
		return time.Time{}
	}
	step := 1
	if s.Schedule == "weekly" {
		step = 7
	}
	return s.Last(now).AddDate(0, 0, step)
}

// Due tells whether a backup is to be made now: the latest scheduled time
// has passed since the last try and since the schedule was saved. A device
// that was off then makes it when it is on again.
func (s *Settings) Due(now time.Time, last Status) bool {
	if s.Schedule == "off" {
		return false
	}
	t := s.Last(now)
	return t.After(last.Time) && t.After(s.Since)
}

// Mountpoint is where a share is mounted for a backup.
var Mountpoint = "/run/suplaos/share"

// command runs mount and umount; tests replace it.
var command = func(name string, args ...string) error {
	out, err := exec.Command(name, args...).CombinedOutput()
	if err != nil {
		return fmt.Errorf("%s: %s", name, strings.TrimSpace(string(out)))
	}
	return nil
}

// open makes the target ready and returns its directory and the function
// that releases it. backups is the backup directory of the device.
func (s *Settings) open(backups string) (string, func(), error) {
	if s.Target == "device" {
		return backups, func() {}, nil
	}
	if err := os.MkdirAll(Mountpoint, 0o700); err != nil {
		return "", nil, err
	}
	// Left mounted by a backup that did not finish.
	command("umount", Mountpoint)
	var err error
	switch s.Target {
	case "smb":
		err = s.mountSMB()
	case "nfs":
		// nolock: no rpcbind and rpc.statd on the device.
		err = command("mount", "-t", "nfs", "-o", "nolock,soft,timeo=150,retrans=2", s.NFS.Export, Mountpoint)
	}
	if err != nil {
		return "", nil, err
	}
	release := func() {
		if command("umount", Mountpoint) != nil {
			command("umount", "-l", Mountpoint)
		}
	}
	dir := filepath.Join(Mountpoint, s.Dir)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		release()
		return "", nil, err
	}
	return dir, release, nil
}

// mountSMB mounts the share with the user and password in a credentials
// file, so that they are not on the command line.
func (s *Settings) mountSMB() error {
	opts := []string{"uid=0", "gid=0", "file_mode=0600", "dir_mode=0700"}
	if s.SMB.Version != "" {
		opts = append(opts, "vers="+s.SMB.Version)
	}
	if s.SMB.Username == "" {
		opts = append(opts, "guest")
	} else {
		cred := filepath.Join(filepath.Dir(Mountpoint), "smb-credentials")
		content := fmt.Sprintf("username=%s\npassword=%s\n", s.SMB.Username, s.SMB.Password)
		if s.SMB.Domain != "" {
			content += "domain=" + s.SMB.Domain + "\n"
		}
		if err := writeFile(cred, []byte(content)); err != nil {
			return err
		}
		defer os.Remove(cred)
		opts = append(opts, "credentials="+cred)
	}
	err := command("mount", "-t", "cifs", "-o", strings.Join(opts, ","), s.SMB.Share, Mountpoint)
	if err != nil {
		// The usual errors of mount.cifs, in words.
		for code, msg := range map[string]string{
			"error(13)":  "the share refused the user or the password",
			"error(2)":   "there is no such share on the server",
			"error(112)": "the server does not answer",
			"error(113)": "the server cannot be reached",
			"error(115)": "the server does not answer",
			"error(95)":  "the server does not support this SMB version",
		} {
			if strings.Contains(err.Error(), code) {
				return fmt.Errorf("%s: %s", s.SMB.Share, msg)
			}
		}
	}
	return err
}

// autoName matches the names of automatic backups: only they are deleted
// when there are too many, not the ones made by hand or other files.
var autoName = regexp.MustCompile(`^suplaos-.+-auto-\d{14}\.tar\.gz(\.age)?$`)

// Name is the file name of an automatic backup.
func Name(host string, t time.Time, encrypted bool) string {
	name := fmt.Sprintf("suplaos-%s-auto-%s.tar.gz", host, t.Format("20060102150405"))
	if encrypted {
		name += ".age"
	}
	return name
}

// prune deletes the oldest automatic backups in dir above keep.
func prune(dir string, keep int, log io.Writer) error {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var names []string
	for _, e := range entries {
		if e.Type().IsRegular() && autoName.MatchString(e.Name()) {
			names = append(names, e.Name())
		}
	}
	// The time in the name sorts them, whatever the device name.
	sort.Slice(names, func(i, j int) bool { return autoTime(names[i]) < autoTime(names[j]) })
	for len(names) > keep {
		fmt.Fprintf(log, "Deleting the old backup %s\n", names[0])
		if err := os.Remove(filepath.Join(dir, names[0])); err != nil {
			return err
		}
		names = names[1:]
	}
	return nil
}

func autoTime(name string) string {
	name = strings.TrimSuffix(strings.TrimSuffix(name, ".age"), ".tar.gz")
	return name[strings.LastIndex(name, "-auto-")+len("-auto-"):]
}

// Test checks that the target can be written to.
func (s *Settings) Test(backups string) error {
	dir, release, err := s.open(backups)
	if err != nil {
		return err
	}
	defer release()
	f, err := os.CreateTemp(dir, ".suplaos-test-*")
	if err != nil {
		return err
	}
	f.Close()
	return os.Remove(f.Name())
}

// Run makes a backup to the target, deletes the old ones and saves the
// status. The stack has to be locked by the caller.
func (s *Settings) Run(st *stack.Stack, version string, log io.Writer) Status {
	status := Status{Time: time.Now()}
	err := s.run(st, version, log, &status)
	if err != nil {
		status.Error = err.Error()
	} else {
		status.OK = true
	}
	if serr := saveStatus(st.Root, status); serr != nil && err == nil {
		status.OK, status.Error = false, serr.Error()
	}
	return status
}

func (s *Settings) run(st *stack.Stack, version string, log io.Writer, status *Status) error {
	dir, release, err := s.open(st.BackupDir())
	if err != nil {
		return err
	}
	defer release()
	host, _ := os.Hostname()
	name := Name(host, status.Time, s.Password != "")
	path := filepath.Join(dir, name)
	if err := st.ExportFile(path, stack.ExportOptions{Version: version, Password: s.Password}); err != nil {
		return err
	}
	status.File = name
	if fi, err := os.Stat(path); err == nil {
		status.Size = fi.Size()
	}
	fmt.Fprintf(log, "Backup saved: %s\n", name)
	return prune(dir, s.Keep, log)
}

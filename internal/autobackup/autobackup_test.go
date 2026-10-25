package autobackup

import (
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Goral64/SuplaOS/internal/stack"
)

func TestValidate(t *testing.T) {
	ok := func(f func(s *Settings)) *Settings {
		s := Default()
		s.Schedule, s.Password = "daily", "12345678"
		f(s)
		return s
	}
	for name, s := range map[string]*Settings{
		"schedule":    ok(func(s *Settings) { s.Schedule = "hourly" }),
		"time":        ok(func(s *Settings) { s.Time = "25:00" }),
		"keep":        ok(func(s *Settings) { s.Keep = 0 }),
		"no password": ok(func(s *Settings) { s.Password = "" }),
		"short":       ok(func(s *Settings) { s.Password = "abc" }),
		"smb share":   ok(func(s *Settings) { s.Target, s.SMB.Share = "smb", `\\nas\kopie` }),
		"smb version": ok(func(s *Settings) { s.Target, s.SMB.Share, s.SMB.Version = "smb", "//nas/kopie", "4" }),
		"nfs export":  ok(func(s *Settings) { s.Target, s.NFS.Export = "nfs", "nas/kopie" }),
		"dir":         ok(func(s *Settings) { s.Target, s.NFS.Export, s.Dir = "nfs", "nas:/kopie", "../x" }),
	} {
		if s.Validate() == nil {
			t.Errorf("%s: accepted", name)
		}
	}
	for name, s := range map[string]*Settings{
		"default": Default(),
		"plain":   ok(func(s *Settings) { s.Password, s.Plain = "", true }),
		"smb":     ok(func(s *Settings) { s.Target, s.SMB.Share, s.Dir = "smb", "//192.168.1.5/Kopie zapasowe", "suplaos" }),
		"nfs":     ok(func(s *Settings) { s.Target, s.NFS.Export = "nfs", "nas.local:/volume1/kopie" }),
	} {
		if err := s.Validate(); err != nil {
			t.Errorf("%s: %v", name, err)
		}
	}
}

func TestSchedule(t *testing.T) {
	loc := time.Local
	at := func(d, hm string) time.Time {
		tm, err := time.ParseInLocation("2006-01-02 15:04", d+" "+hm, loc)
		if err != nil {
			t.Fatal(err)
		}
		return tm
	}
	s := &Settings{Schedule: "daily", Time: "03:30"}
	if got := s.Last(at("2026-10-25", "10:00")); !got.Equal(at("2026-10-25", "03:30")) {
		t.Errorf("daily last %v", got)
	}
	if got := s.Last(at("2026-10-25", "03:00")); !got.Equal(at("2026-10-24", "03:30")) {
		t.Errorf("daily last before the time %v", got)
	}
	if got := s.Next(at("2026-10-25", "10:00")); !got.Equal(at("2026-10-26", "03:30")) {
		t.Errorf("daily next %v", got)
	}
	w := &Settings{Schedule: "weekly", Time: "03:30", Weekday: 1} // Monday
	if got := w.Last(at("2026-10-25", "10:00")); !got.Equal(at("2026-10-19", "03:30")) {
		t.Errorf("weekly last %v", got)
	}
	if got := w.Next(at("2026-10-25", "10:00")); !got.Equal(at("2026-10-26", "03:30")) {
		t.Errorf("weekly next %v", got)
	}

	// Saved at 10:00: not due until the next night, then once.
	s.Since = at("2026-10-25", "10:00")
	if s.Due(at("2026-10-25", "10:01"), Status{}) {
		t.Error("due right after saving")
	}
	if !s.Due(at("2026-10-26", "03:31"), Status{}) {
		t.Error("not due at night")
	}
	if s.Due(at("2026-10-26", "03:40"), Status{Time: at("2026-10-26", "03:31")}) {
		t.Error("due again after a try")
	}
	// Off at night, on in the morning: made then.
	if !s.Due(at("2026-10-26", "08:00"), Status{}) {
		t.Error("missed backup not due")
	}
	s.Schedule = "off"
	if s.Due(at("2026-10-26", "03:31"), Status{}) || !s.Next(at("2026-10-26", "03:31")).IsZero() {
		t.Error("off")
	}
}

func TestPrune(t *testing.T) {
	dir := t.TempDir()
	for _, n := range []string{
		"suplaos-a-auto-20261020033000.tar.gz.age", "suplaos-b-auto-20261022033000.tar.gz",
		"suplaos-a-auto-20261021033000.tar.gz.age", "suplaos-a-20261001120000.tar.gz.age", "notatki.txt",
	} {
		os.WriteFile(filepath.Join(dir, n), nil, 0o600)
	}
	if err := prune(dir, 2, io.Discard); err != nil {
		t.Fatal(err)
	}
	entries, _ := os.ReadDir(dir)
	var left []string
	for _, e := range entries {
		left = append(left, e.Name())
	}
	want := []string{"notatki.txt", "suplaos-a-20261001120000.tar.gz.age", "suplaos-a-auto-20261021033000.tar.gz.age", "suplaos-b-auto-20261022033000.tar.gz"}
	if !slices.Equal(left, want) {
		t.Errorf("left %v", left)
	}
}

// fakeMount records mount and umount and lets the mountpoint be a plain
// directory.
func fakeMount(t *testing.T) *[]string {
	var calls []string
	cmd, mp := command, Mountpoint
	Mountpoint = filepath.Join(t.TempDir(), "share")
	command = func(name string, args ...string) error {
		call := name + " " + strings.Join(args, " ")
		if name == "mount" && args[1] == "cifs" {
			cred := strings.TrimPrefix(args[3][strings.Index(args[3], "credentials="):], "credentials=")
			data, _ := os.ReadFile(cred)
			call += " [" + strings.ReplaceAll(string(data), "\n", ";") + "]"
		}
		calls = append(calls, call)
		return nil
	}
	t.Cleanup(func() { command, Mountpoint = cmd, mp })
	return &calls
}

func TestShares(t *testing.T) {
	calls := fakeMount(t)
	s := &Settings{Target: "smb", SMB: SMB{Share: "//nas/kopie", Username: "jan", Password: "p,a=s", Version: "3.0"}, Dir: "suplaos"}
	must(t, s.Test(t.TempDir()))
	if _, err := os.Stat(filepath.Join(Mountpoint, "suplaos")); err != nil {
		t.Error("directory not made")
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(Mountpoint), "smb-credentials")); err == nil {
		t.Error("credentials left")
	}
	n := &Settings{Target: "nfs", NFS: NFS{Export: "nas:/kopie"}}
	must(t, n.Test(t.TempDir()))
	mp := Mountpoint
	want := []string{
		"umount " + mp,
		"mount -t cifs -o uid=0,gid=0,file_mode=0600,dir_mode=0700,vers=3.0,credentials=" + filepath.Join(filepath.Dir(mp), "smb-credentials") + " //nas/kopie " + mp + " [username=jan;password=p,a=s;]",
		"umount " + mp,
		"umount " + mp,
		"mount -t nfs -o nolock,soft,timeo=150,retrans=2 nas:/kopie " + mp,
		"umount " + mp,
	}
	if !slices.Equal(*calls, want) {
		t.Errorf("calls\n%q\nwant\n%q", *calls, want)
	}
}

func TestRun(t *testing.T) {
	fakeMount(t)
	st, err := stack.Open(t.TempDir(), "../../addons")
	must(t, err)
	s := &Settings{Schedule: "daily", Time: "03:30", Target: "nfs", NFS: NFS{Export: "nas:/kopie"}, Keep: 1, Password: "12345678"}
	for i := 0; i < 2; i++ {
		status := s.Run(st, "0.7.0", io.Discard)
		if !status.OK || !autoName.MatchString(status.File) || !strings.HasSuffix(status.File, ".age") || status.Size == 0 {
			t.Fatalf("status %+v", status)
		}
		time.Sleep(1100 * time.Millisecond) // another name
	}
	entries, _ := os.ReadDir(Mountpoint)
	if len(entries) != 1 {
		t.Errorf("kept %d backups", len(entries))
	}
	if got := LoadStatus(st.Root); !got.OK || got.File != entries[0].Name() {
		t.Errorf("saved status %+v", got)
	}
	s.NFS.Export = ""
	s.Target = "smb" // no share: mount is faked, so make it fail otherwise
	command = func(string, ...string) error { return os.ErrPermission }
	if status := s.Run(st, "0.7.0", io.Discard); status.OK || status.Error == "" {
		t.Errorf("failed backup: %+v", status)
	}
}

func TestSettingsFile(t *testing.T) {
	root := t.TempDir()
	s := Default()
	s.Password = "tajne-haslo"
	must(t, s.Save(root))
	if st, _ := os.Stat(settingsFile(root)); st.Mode().Perm() != 0o600 {
		t.Errorf("mode %v", st.Mode())
	}
	got, err := Load(root)
	must(t, err)
	if got.Password != "tajne-haslo" || got.Keep != 7 {
		t.Errorf("loaded %+v", got)
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

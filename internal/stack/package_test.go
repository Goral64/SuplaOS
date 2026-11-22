package stack

import (
	"errors"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// An update of SuplaOS changes no version of Supla.
func TestPackageNotUpdatedWithSuplaOS(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	// As installed by an older SuplaOS: no package in the state.
	s.State.Addons["supla"].Package = nil
	containerImage = func(svc, _ string) (*usedImage, error) {
		return &usedImage{Ref: "supla/" + svc + "@sha256:running"}, nil
	}
	must(t, s.Apply())
	a := s.Catalog["supla"]
	for svc, m := range a.Images {
		m.Image = m.Repo() + ":99.01@" + m.Digest()
		a.Images[svc] = m
	}
	source := a.Fetch.URL
	a.Fetch.URL = "https://example.com/newer.tar.gz"
	if p, err := s.Pending(); err != nil || len(p) != 0 {
		t.Errorf("a newer catalog is pending: %v %v", p, err)
	}
	must(t, s.Apply())
	data, err := os.ReadFile(filepath.Join(s.Dir(), pinsFile("supla")))
	must(t, err)
	if strings.Contains(string(data), "99.01") || strings.Count(string(data), "@sha256:running") != 3 {
		t.Errorf("Supla changed with SuplaOS:\n%s", data)
	}
	if s.sourceURL(a) != source {
		t.Error("files of the newer catalog")
	}
}

func TestUpgradePackage(t *testing.T) {
	for _, c := range []struct {
		name string
		fail string
		want []string
	}{
		{"works", "", []string{"dump", "config --quiet", "pull supla-cloud supla-db supla-server", "up -d --remove-orphans", "verify"}},
		{"pull fails", "pull", []string{"dump", "config --quiet", "pull supla-cloud supla-db supla-server"}},
		{"does not work after", "verify", []string{"dump", "config --quiet", "pull supla-cloud supla-db supla-server", "up -d --remove-orphans", "verify",
			"up -d --remove-orphans", "dump", "stop supla-cloud supla-server", "load", "up -d supla-cloud supla-server", "up -d --remove-orphans", "verify"}},
	} {
		s := newStack(t)
		must(t, s.Install("supla", nil))
		s.after = nil
		inst := s.State.Addons["supla"]
		old := *inst.Package
		db := old.Env["DATABASE_IMAGE"]
		// The release has a newer database in .env.default.
		env := filepath.Join(s.cacheDir("supla"), ".env.default")
		data, err := os.ReadFile(env)
		must(t, err)
		must(t, os.WriteFile(env, []byte(strings.ReplaceAll(string(data), db, "mariadb:99.9")), 0o644))

		var calls []string
		failed := false
		s.composeHook = func(in io.Reader, out io.Writer, args ...string) error {
			call := strings.Join(args, " ")
			if args[0] == "exec" {
				call = "dump"
				if in != nil {
					io.Copy(io.Discard, in)
					call = "load"
				} else {
					io.WriteString(out, "-- dump\n")
				}
			}
			calls = append(calls, call)
			if args[0] == c.fail {
				return errors.New("failed")
			}
			return nil
		}
		verify := verifyPackage
		verifyPackage = func(*Stack, string) error {
			calls = append(calls, "verify")
			if c.fail == "verify" && !failed {
				failed = true
				return errors.New("supla-server does not work")
			}
			return nil
		}
		to := Package{Tag: "v99", Source: old.Source, Images: map[string]string{
			"supla-cloud":  "supla/supla-cloud:99.02@sha256:new",
			"supla-db":     old.Images["supla-db"],
			"supla-server": old.Images["supla-server"], // the same version
		}}
		err = s.UpgradePackage("supla", to)
		verifyPackage = verify
		if !slices.Equal(calls, c.want) {
			t.Errorf("%s: compose calls\n%q\nwant\n%q", c.name, calls, c.want)
		}
		if got := readEnv(t, s, ".env")["DATABASE_IMAGE"]; got != db {
			t.Errorf("%s: database changed by itself: %s", c.name, got)
		}
		pins, rerr := os.ReadFile(filepath.Join(s.Dir(), pinsFile("supla")))
		must(t, rerr)
		again, oerr := Open(s.Root, "../../addons")
		must(t, oerr)
		saved := again.State.Addons["supla"].Package
		if c.fail == "" {
			if err != nil || saved.Tag != "v99" || !strings.Contains(string(pins), ":99.02@") {
				t.Errorf("%s: %v %+v", c.name, err, saved)
			}
		} else if err == nil || saved.Tag != old.Tag || strings.Contains(string(pins), ":99.02@") {
			t.Errorf("%s: not back: %v %+v", c.name, err, saved)
		}
	}
}

func TestUpgradePackageOnlyWhole(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	var calls []string
	s.composeHook = func(_ io.Reader, _ io.Writer, args ...string) error {
		calls = append(calls, args[0])
		return nil
	}
	if err := s.UpgradePackage("supla", Package{Images: map[string]string{"supla-cloud": "supla/supla-cloud:99"}}); err == nil || len(calls) != 0 {
		t.Errorf("a part alone was updated: %v %v", err, calls)
	}
}

// Another add-on pulling the tag of the database does not change Supla's.
func TestPackageDatabaseExact(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	if db := s.State.Addons["supla"].Package.Images["supla-db"]; !strings.Contains(db, "@sha256:") {
		t.Errorf("database by tag: %q", db)
	}
}

// A new install is the release with the database that works for new ones.
func TestPackageNewInstall(t *testing.T) {
	s := newStack(t)
	a := s.Catalog["supla"]
	u := &SuplaUpdate{Release: &SuplaRelease{Tag: "v99"}, Images: map[string]string{
		"supla-cloud": "supla/supla-cloud:99.01@sha256:c", "supla-server": "supla/supla-server:99@sha256:s",
	}}
	p := newPackage(a, u)
	if p.Tag != "v99" || !strings.HasSuffix(p.Source, "/v99.tar.gz") || len(p.Images) != 3 || p.Images["supla-db"] != a.Images["supla-db"].Image {
		t.Errorf("package %+v", p)
	}
	db := a.Images["supla-db"].Tag()
	if p.Env["DATABASE_IMAGE"] != "mariadb:"+db || p.Env["DATABASE_VERSION"] != "mariadb-"+db {
		t.Errorf("not the database of the catalog: %v", p.Env)
	}
}

// Installing again over the data left is no update.
func TestPackageSurvivesRemove(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	was := *s.State.Addons["supla"].Package
	was.Tag = "v1"
	s.State.Addons["supla"].Package.Tag = "v1"
	must(t, s.Remove("supla"))
	asked := false
	newInstallPackage = func(*Stack, *addon.Addon) *Package { asked = true; return nil }
	must(t, s.Install("supla", nil))
	if p := s.State.Addons["supla"].Package; asked || p.Tag != "v1" || p.Images["supla-cloud"] != was.Images["supla-cloud"] {
		t.Errorf("updated by installing again: %+v", p)
	}
	must(t, s.Remove("supla"))
	must(t, s.Purge("supla"))
	must(t, s.Install("supla", nil))
	if !asked {
		t.Error("no release for an install without data")
	}
}

// A new database goes with the whole package, and back with its directory.
func TestUpgradePackageDatabase(t *testing.T) {
	const pull, up = "pull supla-cloud supla-db supla-server", "up -d --remove-orphans"
	const stop = "stop supla-cloud supla-db supla-server"
	head := []string{"dump", "config --quiet", pull, stop, "copy"}
	for _, c := range []struct {
		fail string
		want []string
	}{
		{"", slices.Concat(head, []string{up, "verify", "drop"})},
		{"copy", slices.Concat(head, []string{up, "verify"})},
		{"verify", slices.Concat(head, []string{up, "verify", stop, "back", up, "verify", "drop"})},
		{"always", slices.Concat(head, []string{up, "verify", stop, "back", up, "verify"})},
	} {
		s := newStack(t)
		must(t, s.Install("supla", nil))
		s.after = nil
		old := *s.State.Addons["supla"].Package
		var calls []string
		s.composeHook = func(in io.Reader, out io.Writer, args ...string) error {
			call := strings.Join(args, " ")
			switch args[0] {
			case "exec":
				call = "dump"
				io.WriteString(out, "-- dump\n")
			case "run":
				switch {
				case slices.Contains(args, copyDatabase):
					call = "copy"
				case slices.Contains(args, restoreDatabase):
					call = "back"
				case slices.Contains(args, dropDatabaseCopy):
					call = "drop"
				}
				if args[len(args)-1] != "mysql" {
					t.Errorf("directory %v", args)
				}
			}
			calls = append(calls, call)
			if call == c.fail {
				return errors.New("failed")
			}
			return nil
		}
		verify, failed := verifyPackage, false
		verifyPackage = func(*Stack, string) error {
			calls = append(calls, "verify")
			if c.fail == "always" || c.fail == "verify" && !failed {
				failed = true
				return errors.New("does not work")
			}
			return nil
		}
		images := map[string]string{}
		for svc, ref := range old.Images {
			images[svc] = ref
		}
		images["supla-db"] = "mariadb:99.9@sha256:new"
		err := s.UpgradePackage("supla", Package{Tag: "v99", Source: old.Source, Images: images,
			Env: databaseEnv(&DatabaseVersions{Release: "mariadb:99.9"})})
		verifyPackage = verify
		if !slices.Equal(calls, c.want) {
			t.Errorf("%q: calls\n%q\nwant\n%q", c.fail, calls, c.want)
		}
		env := readEnv(t, s, ".env")
		image, version := "mariadb:99.9", "mariadb-99.9"
		if c.fail != "" {
			image, version = old.Env["DATABASE_IMAGE"], old.Env["DATABASE_VERSION"]
		}
		if (err == nil) != (c.fail == "") || env["DATABASE_IMAGE"] != image || (version != "" && env["DATABASE_VERSION"] != version) {
			t.Errorf("%q: %v, database %s %s", c.fail, err, env["DATABASE_IMAGE"], env["DATABASE_VERSION"])
		}
	}
}

// A power cut while the new package starts: it is in the state already,
// the old one is not applied again over a newer database.
func TestUpgradePackageInterrupted(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	s.after = nil
	old := *s.State.Addons["supla"].Package
	verify := verifyPackage
	verifyPackage = func(*Stack, string) error { return nil }
	defer func() { verifyPackage = verify }()
	looked := false
	s.composeHook = func(in io.Reader, out io.Writer, args ...string) error {
		switch args[0] {
		case "exec":
			io.WriteString(out, "-- dump\n")
		case "up":
			looked = true
			again, err := Open(s.Root, "../../addons")
			must(t, err)
			again.TZ = s.TZ
			p := again.State.Addons["supla"].Package
			if p.Tag != "v99" || p.Env["DATABASE_IMAGE"] != "mariadb:99.9" {
				t.Errorf("started before it was saved: %+v", p)
			}
			if pending, err := again.Pending(); err != nil || len(pending) != 0 {
				t.Errorf("a restart here would apply again: %v %v", pending, err)
			}
		}
		return nil
	}
	images := map[string]string{}
	for svc, ref := range old.Images {
		images[svc] = ref
	}
	images["supla-db"] = "mariadb:99.9@sha256:new"
	must(t, s.UpgradePackage("supla", Package{Tag: "v99", Source: old.Source, Images: images,
		Env: databaseEnv(&DatabaseVersions{Release: "mariadb:99.9"})}))
	if !looked {
		t.Error("no up")
	}
}

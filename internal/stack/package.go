package stack

import (
	"errors"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"slices"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// Supla is a package: supla-docker, Supla Cloud, supla-server and the
// database. It changes only with the procedure of supla.sh upgrade, always
// whole: backup, pull of all its services, up. Nothing else changes a
// version in it, an update of SuplaOS neither: the images in the catalog
// are only those of a new install.

// Package is the installed version of an add-on updated as a whole.
type Package struct {
	// Tag is the release of supla-docker, empty when installed before
	// SuplaOS followed releases.
	Tag string `yaml:"tag,omitempty"`
	// Source is the archive the files come from.
	Source string            `yaml:"source,omitempty"`
	Images map[string]string `yaml:"images"`
	// Env holds the version of the database: it changes with an update of
	// the package to a higher one, like .env fixed by hand after git pull,
	// never with the files alone.
	Env map[string]string `yaml:"env,omitempty"`
}

// packageEnv are the keys of .env.default used as the package has them.
var packageEnv = []string{"DATABASE_IMAGE", "DATABASE_VERSION"}

// source of the upstream files of an add-on: of its package, when it has
// one.
func (s *Stack) sourceURL(a *addon.Addon) string {
	if inst := s.State.Addons[a.ID]; inst != nil && inst.Package != nil && inst.Package.Source != "" {
		return inst.Package.Source
	}
	return a.Fetch.URL
}

// packageImages are the images to use for an add-on.
func (s *Stack) packageImages(a *addon.Addon) map[string]string {
	images := map[string]string{}
	for svc, m := range a.Images {
		if m.Image != "" {
			images[svc] = m.Image
		}
	}
	if inst := s.State.Addons[a.ID]; inst != nil {
		maps.Copy(images, inst.Images)
		if inst.Package != nil {
			maps.Copy(images, inst.Package.Images)
		}
	}
	return images
}

// adoptPackages notes the package of an add-on that has none yet: just
// installed (that of the catalog), or installed by an older SuplaOS (what
// it runs, nothing is updated by that). It tells whether the state changed.
func (s *Stack) adoptPackages() (bool, error) {
	changed := false
	for _, a := range s.Order() {
		inst := s.State.Addons[a.ID]
		if !a.Pinned() || inst.Package != nil {
			continue
		}
		p := &Package{Images: s.packageImages(a), Env: map[string]string{}}
		for svc, m := range a.Images {
			if m.Image == "" {
				continue
			}
			used, err := containerImage(svc, m.VersionEnv)
			if err != nil {
				return changed, fmt.Errorf("%s: %w", svc, err)
			}
			if used != nil {
				p.Images[svc] = used.Ref
			}
		}
		if a.Fetch != nil {
			p.Source = a.Fetch.URL
			if src, err := os.ReadFile(filepath.Join(s.cacheDir(a.ID), sourceFile)); err == nil {
				p.Source = string(src)
			}
		}
		if a.EnvBase != "" {
			data, err := s.source(a, a.EnvBase)
			if err != nil {
				return changed, err
			}
			base, err := parseEnv(data)
			if err != nil {
				return changed, fmt.Errorf("%s: %s: %w", a.ID, a.EnvBase, err)
			}
			for _, kv := range base {
				if slices.Contains(packageEnv, kv.Key) {
					p.Env[kv.Key] = kv.Value
				}
			}
		}
		inst.Package, inst.Images = p, nil
		changed = true
	}
	return changed, nil
}

// verifyPackage is Verify, replaced in tests.
var verifyPackage = func(s *Stack, id string) error { return s.Verify(id) }

// UpgradePackage is supla.sh upgrade: backup, pull of all services of the
// add-on, up. A service is never left out, also one with the same version.
// Only the services of the add-on are pulled, the project has other add-ons
// too. When it does not work afterwards, all of it goes back: files,
// images and database. A new version of the database does not go back by
// a dump: its directory is copied first, with all services stopped.
func (s *Stack) UpgradePackage(id string, to Package) error {
	a := s.Catalog[id]
	if a == nil || !s.IsInstalled(id) || !a.Pinned() {
		return fmt.Errorf("%s is not an installed package", id)
	}
	for svc, m := range a.Images {
		if m.Image != "" && to.Images[svc] == "" {
			return fmt.Errorf("%s: the package has no %s, it is updated only as a whole", id, svc)
		}
	}
	changed, err := s.adoptPackages()
	if err != nil {
		return err
	}
	if changed {
		if err := s.Save(); err != nil {
			return err
		}
	}
	inst := s.State.Addons[id]
	old := *inst.Package
	if to.Env == nil {
		to.Env = old.Env
	}
	files, err := s.cachedFiles(a)
	if err != nil {
		return err
	}

	fmt.Fprintln(s.stdout(), "Making database backup")
	backup, err := s.Backup(id)
	if err != nil {
		return err
	}
	fmt.Fprintf(s.stdout(), "Backup saved to %s\n", backup)

	newDatabase := a.Backup != nil && a.Backup.Dir != "" && to.Env["DATABASE_IMAGE"] != old.Env["DATABASE_IMAGE"]
	var stopped, copied, started bool
	err = func() error {
		inst.Package = &to
		if a.Fetch != nil {
			if err := s.fetch(a); err != nil {
				return err
			}
		}
		if err := s.Apply(); err != nil {
			return err
		}
		if err := s.Compose("config", "--quiet"); err != nil {
			return err
		}
		if err := s.Compose(append([]string{"pull"}, a.Services...)...); err != nil {
			return err
		}
		if newDatabase {
			stopped = true
			if err := s.Compose(append([]string{"stop"}, a.Services...)...); err != nil {
				return err
			}
			fmt.Fprintln(s.stdout(), "Copying the directory of the database")
			if err := s.databaseDir(a, copyDatabase); err != nil {
				return err
			}
			copied = true
		}
		// Noted before anything starts with it: after a power cut in the
		// middle the files of the old package would be applied again, an
		// older database on the directory of the newer one.
		if err := s.Save(); err != nil {
			return err
		}
		started = true
		if err := s.Up(); err != nil {
			return err
		}
		return verifyPackage(s, id)
	}()
	if err == nil {
		if copied {
			if derr := s.databaseDir(a, dropDatabaseCopy); derr != nil {
				fmt.Fprintf(s.stdout(), "The copy of the database stays: %v\n", derr)
			}
		}
		return nil
	}
	fmt.Fprintf(s.stdout(), "The update failed (%v), all of %s goes back\n", err, id)
	inst.Package = &old
	if back := s.goBack(a, files, backup, stopped, copied, started); back != nil {
		return fmt.Errorf("%w; going back failed too, the backup is %s: %w", err, backup, back)
	}
	return fmt.Errorf("the update failed, %s is as it was: %w", id, err)
}

func (s *Stack) goBack(a *addon.Addon, files map[string][]byte, backup string, stopped, copied, started bool) error {
	for name, data := range files {
		if err := s.writeFile(filepath.Join(s.cacheDir(a.ID), name), data, 0o644); err != nil {
			return err
		}
	}
	if err := s.Apply(); err != nil {
		return err
	}
	if err := s.Save(); err != nil {
		return err
	}
	if !started {
		// Nothing was recreated, the database is as it was.
		if !stopped {
			return nil
		}
		if err := s.Up(); err != nil {
			return err
		}
		return verifyPackage(s, a.ID)
	}
	if copied {
		// The directory from before, exact: no dump to load. The copy is
		// deleted only when Supla works with it.
		if err := s.Compose(append([]string{"stop"}, a.Services...)...); err != nil {
			return err
		}
		fmt.Fprintln(s.stdout(), "Putting the directory of the database back")
		if err := s.databaseDir(a, restoreDatabase); err != nil {
			return err
		}
		if err := s.Up(); err != nil {
			return err
		}
		if err := verifyPackage(s, a.ID); err != nil {
			return err
		}
		return s.databaseDir(a, dropDatabaseCopy)
	}
	// The database may be migrated already: the older parts do not start
	// with it until the backup is loaded, so the first up may fail.
	first := s.Up()
	if err := s.Restore(a.ID, backup); err != nil {
		return errors.Join(first, err)
	}
	if err := s.Up(); err != nil {
		return err
	}
	return verifyPackage(s, a.ID)
}

// Scripts for the directory of the database, $1 in /suplaos-data. They run
// in a container of the database: the files belong to its user, also for
// suplaos without root.
const (
	copyDatabase = `set -e; cd /suplaos-data; test -n "$1"; test -d "$1"
rm -rf -- "./$1.before" "./$1.before.part"
cp -a -- "./$1" "./$1.before.part"
mv -- "./$1.before.part" "./$1.before"
sync`
	restoreDatabase = `set -e; cd /suplaos-data; test -n "$1"; test -d "$1"; test -d "$1.before"
find "./$1" -mindepth 1 -delete
cp -a -- "./$1.before/." "./$1/"
sync`
	dropDatabaseCopy = `set -e; cd /suplaos-data; test -n "$1"
rm -rf -- "./$1.before"`
)

func (s *Stack) databaseDir(a *addon.Addon, script string) error {
	dir, err := filepath.Abs(filepath.Join(s.Dir(), a.Backup.Dir))
	if err != nil {
		return err
	}
	return s.Compose("run", "--rm", "--no-deps", "-T", "--user", "root", "--entrypoint", "sh",
		"-v", filepath.Dir(dir)+":/suplaos-data", a.Backup.Service, "-c", script, "sh", filepath.Base(dir))
}

// cachedFiles reads the downloaded files of an add-on, to go back without
// the internet.
func (s *Stack) cachedFiles(a *addon.Addon) (map[string][]byte, error) {
	files := map[string][]byte{}
	if a.Fetch == nil {
		return files, nil
	}
	for _, name := range slices.Concat(a.Fetch.Files, []string{sourceFile}) {
		data, err := os.ReadFile(filepath.Join(s.cacheDir(a.ID), name))
		if err != nil {
			return nil, err
		}
		files[name] = data
	}
	return files, nil
}

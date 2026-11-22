package stack

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// SuplaID is the add-on updated by the releases of supla-docker.
const SuplaID = "supla"

const suplaEnvDefault = "https://raw.githubusercontent.com/SUPLA/supla-docker/%s/.env.default"

const suplaArchive = "https://github.com/SUPLA/supla-docker/archive/refs/tags/%s.tar.gz"

// CheckSupla tells what the newest release of supla-docker means for the
// installed Supla.
func (s *Stack) CheckSupla(ctx context.Context) (*SuplaUpdate, error) {
	a := s.Catalog[SuplaID]
	if a == nil || !s.IsInstalled(SuplaID) {
		return nil, errors.New("Supla is not installed")
	}
	r, err := LatestSuplaRelease(ctx)
	if err != nil {
		return nil, err
	}
	tag := ""
	if p := s.State.Addons[SuplaID].Package; p != nil {
		tag = p.Tag
	}
	var here SuplaVersions
	for svc, v := range map[string]*string{"supla-cloud": &here.Cloud, "supla-server": &here.Server} {
		used, err := containerImage(svc, a.Images[svc].VersionEnv)
		if err != nil {
			return nil, err
		}
		if used == nil {
			return nil, fmt.Errorf("%s has no container, start Supla first", svc)
		}
		*v = used.Version
	}
	db, err := s.databaseVersions(ctx, r.Tag)
	if err != nil {
		return nil, err
	}
	return decideUpdate(ctx, lookupImage, r, tag, here, db)
}

// UpgradeSupla updates Supla to the newest release of supla-docker, the
// whole package with the procedure of supla.sh upgrade. tag, when given,
// is the release the user agreed to.
func (s *Stack) UpgradeSupla(ctx context.Context, tag string) error {
	u, err := s.CheckSupla(ctx)
	if err != nil {
		return err
	}
	if tag != "" && tag != u.Release.Tag {
		return fmt.Errorf("the newest release is %s now, not %s: check again", u.Release.Tag, tag)
	}
	switch u.State {
	case SuplaCurrent:
		fmt.Fprintf(s.stdout(), "Supla is up to date (supla-docker %s)\n", u.Release.Tag)
		return nil
	case SuplaWaiting:
		return fmt.Errorf("supla-docker %s waits for official images: %v", u.Release.Tag, u.Waiting)
	case SuplaUnreadable:
		return fmt.Errorf("the notes of supla-docker %s do not name the versions, see %s", u.Release.Tag, u.Release.URL)
	case SuplaNewer:
		return fmt.Errorf("the Supla here is newer than supla-docker %s, it is not moved back", u.Release.Tag)
	}
	// The database is a part like the others: of the version of the
	// release when that is higher, else of the installed one, pulled anyway.
	var env map[string]string
	db := u.Images["supla-db"]
	if db != "" {
		env = databaseEnv(u.Database)
		fmt.Fprintf(s.stdout(), "Database: %s -> %s\n", u.Database.Here, u.Database.Release)
	} else {
		if db, err = s.databaseImage(ctx, lookupImage); err != nil {
			return err
		}
		u.Images["supla-db"] = db
	}
	fmt.Fprintf(s.stdout(), "supla-docker %s: Supla Cloud %s, supla-server %s, database %s\n",
		u.Release.Tag, u.Release.Versions.Cloud, u.Release.Versions.Server, db)
	return s.UpgradePackage(SuplaID, Package{
		Tag:    u.Release.Tag,
		Source: fmt.Sprintf(suplaArchive, u.Release.Tag),
		Images: u.Images,
		Env:    env,
	})
}

// newInstallPackage is the package of a new install of an add-on, nil for
// that of the catalog; replaced in tests.
var newInstallPackage = (*Stack).releasePackage

// releasePackage is for a new Supla the newest release, with the database
// of the catalog: a new install works with that one, a newer one in
// .env.default of the release is a change of its own. Without a release
// to install (waiting for images, GitHub not answering) the package of the
// catalog is installed, the release is shown as an update then.
func (s *Stack) releasePackage(a *addon.Addon) *Package {
	if a.ID != SuplaID {
		return nil
	}
	db := a.Images["supla-db"]
	if db.Image == "" {
		return nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	r, err := LatestSuplaRelease(ctx)
	if err == nil {
		var u *SuplaUpdate
		if u, err = decideUpdate(ctx, lookupImage, r, "", SuplaVersions{}, nil); err == nil {
			if u.State == SuplaAvailable {
				return newPackage(a, u)
			}
			err = fmt.Errorf("supla-docker %s is %s", r.Tag, u.State)
		}
	}
	fmt.Fprintf(s.stdout(), "Not the newest release (%v): installing the package that came with SuplaOS\n", err)
	return nil
}

// newPackage is the package of a release for a new install.
func newPackage(a *addon.Addon, u *SuplaUpdate) *Package {
	db := a.Images["supla-db"]
	images := maps.Clone(u.Images)
	images["supla-db"] = db.Image
	return &Package{
		Tag:    u.Release.Tag,
		Source: fmt.Sprintf(suplaArchive, u.Release.Tag),
		Images: images,
		Env: map[string]string{
			"DATABASE_IMAGE":   db.Repo() + ":" + db.Tag(),
			"DATABASE_VERSION": db.Repo() + "-" + db.Tag(),
		},
	}
}

// databaseImage is the current image of the version of the database that
// is installed, exact.
func (s *Stack) databaseImage(ctx context.Context, look imageLookup) (string, error) {
	image, err := s.databaseHere()
	if err != nil {
		return "", err
	}
	digest, _, err := look(ctx, image, "")
	if err != nil {
		return "", fmt.Errorf("%s: %w", image, err)
	}
	return image + "@" + digest, nil
}

func (s *Stack) databaseHere() (string, error) {
	p, err := s.Plan()
	if err != nil {
		return "", err
	}
	for _, kv := range p.Env {
		if kv.Key == "DATABASE_IMAGE" && kv.Value != "" {
			return kv.Value, nil
		}
	}
	return "", errors.New("DATABASE_IMAGE is not set")
}

// databaseEnv is the fix of .env that one makes by hand for the database
// of a release.
func databaseEnv(d *DatabaseVersions) map[string]string {
	version := d.Version
	if version == "" {
		version = strings.Replace(d.Release, ":", "-", 1)
	}
	return map[string]string{"DATABASE_IMAGE": d.Release, "DATABASE_VERSION": version}
}

// databaseVersions compares the database installed with .env.default of
// the release, as one does after git pull.
func (s *Stack) databaseVersions(ctx context.Context, tag string) (*DatabaseVersions, error) {
	here, err := s.databaseHere()
	if err != nil {
		return nil, err
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, fmt.Sprintf(suplaEnvDefault, tag), nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf(".env.default of supla-docker %s: %s", tag, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, err
	}
	env, err := parseEnv(data)
	if err != nil {
		return nil, fmt.Errorf(".env.default of supla-docker %s: %w", tag, err)
	}
	db := &DatabaseVersions{Here: here}
	for _, kv := range env {
		switch kv.Key {
		case "DATABASE_IMAGE":
			db.Release = kv.Value
		case "DATABASE_VERSION":
			db.Version = kv.Value
		}
	}
	if db.Release == "" {
		return nil, nil
	}
	return db, nil
}

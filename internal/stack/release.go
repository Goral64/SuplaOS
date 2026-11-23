package stack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"regexp"
	"strings"
	"time"
)

// Supla is updated only as a whole, when supla-docker has a release: a new
// image of one part does not mean that the package is ready. The release
// notes list the versions of the parts, often several of each and not the
// same for both; the newest of each is the one of the release.

const suplaReleasesURL = "https://api.github.com/repos/SUPLA/supla-docker/releases?per_page=30"

var (
	releaseTagRe = regexp.MustCompile(`^v\d+(\.\d+)+$`)
	// A version is read from the badge, not from its link: in the notes of
	// v26.03 the badge of Cloud v26.03.02 links to v26.03.01.
	releaseBadgeRe = regexp.MustCompile(`(?i)release/SUPLA/supla-(cloud|core)\?filter=v(\d+(?:\.\d+)+)`)
)

// SuplaVersions are the versions of the parts of Supla.
type SuplaVersions struct {
	Cloud  string `json:"cloud"`
	Server string `json:"server"`
}

// SuplaRelease is a release of supla-docker. Versions are empty when its
// notes do not name both parts. Notes may have steps to do by hand.
type SuplaRelease struct {
	Tag       string        `json:"tag"`
	URL       string        `json:"url"`
	Notes     string        `json:"notes"`
	Published time.Time     `json:"published"`
	Versions  SuplaVersions `json:"versions"`
}

// releaseVersions returns the newest version of each part named in the
// notes of a release.
func releaseVersions(notes string) SuplaVersions {
	var v SuplaVersions
	for _, m := range releaseBadgeRe.FindAllStringSubmatch(notes, -1) {
		part := &v.Server
		if strings.EqualFold(m[1], "cloud") {
			part = &v.Cloud
		}
		if *part == "" || imageNewer(m[2], *part) {
			*part = m[2]
		}
	}
	if v.Cloud == "" || v.Server == "" {
		return SuplaVersions{}
	}
	return v
}

// newestRelease picks the release with the highest version: the list is
// not in that order. Drafts and pre-releases are not releases.
func newestRelease(r io.Reader) (*SuplaRelease, error) {
	var list []struct {
		Tag        string    `json:"tag_name"`
		URL        string    `json:"html_url"`
		Body       string    `json:"body"`
		Published  time.Time `json:"published_at"`
		Draft      bool      `json:"draft"`
		Prerelease bool      `json:"prerelease"`
	}
	if err := json.NewDecoder(r).Decode(&list); err != nil {
		return nil, fmt.Errorf("releases of supla-docker: %w", err)
	}
	var best *SuplaRelease
	for _, e := range list {
		if e.Draft || e.Prerelease || !releaseTagRe.MatchString(e.Tag) {
			continue
		}
		if best == nil || imageNewer(e.Tag, best.Tag) {
			best = &SuplaRelease{Tag: e.Tag, URL: e.URL, Notes: e.Body, Published: e.Published, Versions: releaseVersions(e.Body)}
		}
	}
	if best == nil {
		return nil, errors.New("supla-docker has no releases")
	}
	return best, nil
}

// LatestSuplaRelease asks GitHub for the newest release of supla-docker:
// the API, or the web pages when that does not answer (its limit).
func LatestSuplaRelease(ctx context.Context) (*SuplaRelease, error) {
	r, err := releaseFromAPI(ctx)
	if err == nil {
		return r, nil
	}
	r, werr := releaseFromWeb(ctx)
	if werr != nil {
		return nil, errors.Join(err, werr)
	}
	return r, nil
}

func releaseFromAPI(ctx context.Context) (*SuplaRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, suplaReleasesURL, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("releases of supla-docker: %s", resp.Status)
	}
	return newestRelease(io.LimitReader(resp.Body, 4<<20))
}

// States of an update of Supla.
const (
	SuplaCurrent    = "current"
	SuplaAvailable  = "available"
	SuplaWaiting    = "waiting"    // an official image of the release is not out yet
	SuplaUnreadable = "unreadable" // the notes do not name the versions
	SuplaNewer      = "newer"      // a part here is newer than in the release
)

// SuplaUpdate is what a release means for the installed Supla. Images are
// those of all parts, also of one with the same version: there is no
// update of a part alone.
type SuplaUpdate struct {
	State   string            `json:"state"`
	Release *SuplaRelease     `json:"release"`
	Images  map[string]string `json:"images,omitempty"`
	// Waiting are the images that are not out yet.
	Waiting []string `json:"waiting,omitempty"`
	// What is installed: the release and the versions of the parts.
	Tag  string        `json:"tag"`
	Here SuplaVersions `json:"here"`
	// Database is the version installed next to that in .env.default of
	// the release.
	Database *DatabaseVersions `json:"database,omitempty"`
	// Env are the changes of .env.default from the installed release to
	// this one, without the database (above).
	Env []EnvChange `json:"env,omitempty"`
}

// EnvChange is a variable of .env.default that the release adds (Here
// empty), removes (Release empty) or changes. Own: the user's own value
// is used instead.
type EnvChange struct {
	Key     string `json:"key"`
	Here    string `json:"here"`
	Release string `json:"release"`
	New     bool   `json:"new,omitempty"`
	Removed bool   `json:"removed,omitempty"`
	Own     bool   `json:"own,omitempty"`
}

// DatabaseVersions are the images of the database: installed and in
// .env.default of the release. A higher one there is an update of Supla
// like a higher version of another part, Raise; a lower one is not used.
type DatabaseVersions struct {
	Here    string `json:"here"`
	Release string `json:"release"`
	// Version is DATABASE_VERSION of the release.
	Version string `json:"-"`
	Raise   bool   `json:"raise"`
}

func (d *DatabaseVersions) higher() bool {
	repo, tag, ok := strings.Cut(d.Release, ":")
	repoHere, tagHere, okHere := strings.Cut(d.Here, ":")
	return ok && okHere && repo == repoHere && imageNewer(tag, tagHere)
}

// imageLookup returns the digest and the version inside of an image for
// this device.
type imageLookup func(ctx context.Context, ref, versionEnv string) (digest, version string, err error)

func lookupImage(ctx context.Context, ref, versionEnv string) (string, string, error) {
	digest, err := RemoteDigest(ctx, ref)
	if err != nil {
		return "", "", err
	}
	config, err := RemoteConfig(ctx, ref, digest)
	if err != nil {
		return "", "", err
	}
	return digest, config.version(versionEnv), nil
}

// officialImage finds the official image of exactly the version: the tag
// with its number, or latest when that has this version inside. Never
// another version. An empty ref: not out yet.
func officialImage(ctx context.Context, look imageLookup, repo, versionEnv, version string) (string, error) {
	digest, _, err := look(ctx, repo+":"+version, versionEnv)
	if err == nil {
		return repo + ":" + version + "@" + digest, nil
	}
	if !errors.Is(err, errNoImage) {
		return "", err
	}
	digest, inside, err := look(ctx, repo+":latest", versionEnv)
	switch {
	case errors.Is(err, errNoImage):
		return "", nil
	case err != nil:
		return "", err
	case strings.TrimPrefix(inside, "v") != version:
		return "", nil
	}
	return repo + ":latest@" + digest, nil
}

// decideUpdate compares the release with what is installed: the tag of
// supla-docker (empty when not known) and the versions of the parts. A new
// tag or a higher version of any part, also one added to the notes later,
// is an update of the whole package. Nothing goes back.
func decideUpdate(ctx context.Context, look imageLookup, r *SuplaRelease, tag string, here SuplaVersions, db *DatabaseVersions) (*SuplaUpdate, error) {
	u := &SuplaUpdate{Release: r, Tag: tag, Here: here, Database: db}
	raise := db != nil && db.higher()
	if db != nil {
		db.Raise = raise
	}
	to := r.Versions
	if to == (SuplaVersions{}) {
		u.State = SuplaUnreadable
		if tag == r.Tag {
			u.State = SuplaCurrent
		}
		return u, nil
	}
	if (tag != "" && imageNewer(tag, r.Tag)) || imageNewer(here.Cloud, to.Cloud) || imageNewer(here.Server, to.Server) {
		u.State = SuplaNewer
		return u, nil
	}
	if tag == r.Tag && !raise && !imageNewer(to.Cloud, here.Cloud) && !imageNewer(to.Server, here.Server) {
		u.State = SuplaCurrent
		return u, nil
	}
	images := map[string]string{}
	for _, part := range []struct{ service, env, version string }{
		{"supla-cloud", "CLOUD_VERSION", to.Cloud},
		{"supla-server", "SERVER_VERSION", to.Server},
	} {
		repo := "supla/" + part.service
		ref, err := officialImage(ctx, look, repo, part.env, part.version)
		if err != nil {
			return nil, fmt.Errorf("%s %s: %w", repo, part.version, err)
		}
		if ref == "" {
			u.Waiting = append(u.Waiting, repo+":"+part.version)
		}
		images[part.service] = ref
	}
	if raise {
		digest, _, err := look(ctx, db.Release, "")
		switch {
		case errors.Is(err, errNoImage):
			u.Waiting = append(u.Waiting, db.Release)
		case err != nil:
			return nil, fmt.Errorf("%s: %w", db.Release, err)
		default:
			images["supla-db"] = db.Release + "@" + digest
		}
	}
	if len(u.Waiting) > 0 {
		u.State = SuplaWaiting
		return u, nil
	}
	u.State, u.Images = SuplaAvailable, images
	return u, nil
}

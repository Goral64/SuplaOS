package stack

import (
	"context"
	"errors"
	"os"
	"strings"
	"testing"
)

// badges makes release notes like those of supla-docker, e.g. "cloud 26.11".
func badges(list ...string) string {
	var sb strings.Builder
	for _, e := range list {
		part, v, _ := strings.Cut(e, " ")
		sb.WriteString("[![x](https://img.shields.io/github/v/release/SUPLA/supla-" + part + "?filter=v" + v + ")]" +
			"(https://github.com/SUPLA/supla-" + part + "/releases/v" + v + ") <br> ")
	}
	return sb.String()
}

func TestReleaseVersions(t *testing.T) {
	for _, c := range []struct {
		name, notes string
		want        SuplaVersions
	}{
		{"v26.11", badges("cloud 26.11", "cloud 26.11.01", "core 26.11", "core 26.11.01"), SuplaVersions{"26.11.01", "26.11.01"}},
		{"v26.03", badges("cloud 26.03", "cloud 26.03.01", "cloud 26.03.02", "core 26.03", "core 26.03.01", "core 26.03.02", "core 26.03.03"), SuplaVersions{"26.03.02", "26.03.03"}},
		{"v26.02", badges("cloud 26.02", "core 25.12"), SuplaVersions{"26.02", "25.12"}},
		{"not in order", badges("core 23.12", "core 23.11.01", "cloud 23.11.02", "cloud 23.11"), SuplaVersions{"23.11.02", "23.12"}},
		{"wrong link", "[![x](https://img.shields.io/github/v/release/SUPLA/supla-cloud?filter=v26.03.02)](https://github.com/SUPLA/supla-cloud/releases/v26.03.01)" + badges("core 26.03.03"), SuplaVersions{"26.03.02", "26.03.03"}},
		{"one part only", badges("cloud 26.11"), SuplaVersions{}},
		{"v2.1.6", "SUPLA-Cloud v2.1.6 and SUPLA-core v1.8.5", SuplaVersions{}},
	} {
		if got := releaseVersions(c.notes); got != c.want {
			t.Errorf("%s: %+v, want %+v", c.name, got, c.want)
		}
	}
}

func TestNewestRelease(t *testing.T) {
	r, err := newestRelease(strings.NewReader(`[{"tag_name":"v24.12"},{"tag_name":"v26.05"},{"tag_name":"v27.01","prerelease":true},
		{"tag_name":"v26.11"},{"tag_name":"v28.01","draft":true},{"tag_name":"test"}]`))
	if err != nil || r.Tag != "v26.11" {
		t.Errorf("%+v %v", r, err)
	}
}

func TestDecideUpdate(t *testing.T) {
	const cloud, server = "supla/supla-cloud:", "supla/supla-server:"
	offline := errors.New("no network")
	r := &SuplaRelease{Tag: "v26.11", Versions: SuplaVersions{"26.11.01", "26.11.01"}}
	same := r.Versions
	older := SuplaVersions{"26.05.01", "26.05"}
	both := map[string]string{cloud + "26.11.01": "26.11.01", server + "26.11.01": "v26.11.01"}
	for _, c := range []struct {
		name   string
		tag    string
		here   SuplaVersions
		images map[string]string // ref: the version inside
		state  string
		err    bool
	}{
		{"new release", "v26.05", older, both, SuplaAvailable, false},
		{"new release, same parts", "", same, both, SuplaAvailable, false},
		{"one part higher", "v26.05", SuplaVersions{"26.11.01", "26.05"}, both, SuplaAvailable, false},
		{"version added to the notes", "v26.11", SuplaVersions{"26.11", "26.11.01"}, both, SuplaAvailable, false},
		{"current", "v26.11", same, nil, SuplaCurrent, false},
		{"part newer here", "v26.11", SuplaVersions{"26.12", "26.11.01"}, both, SuplaNewer, false},
		{"release newer here", "v26.12", same, both, SuplaNewer, false},
		{"only the Cloud is out", "v26.05", older, map[string]string{cloud + "26.11.01": "26.11.01", server + "latest": "v26.05"}, SuplaWaiting, false},
		{"latest is that version", "v26.05", older, map[string]string{cloud + "26.11.01": "26.11.01", server + "latest": "v26.11.01"}, SuplaAvailable, false},
		{"latest is newer", "v26.05", older, map[string]string{cloud + "26.11.01": "26.11.01", server + "latest": "v26.12"}, SuplaWaiting, false},
		{"registry fails", "v26.05", older, map[string]string{cloud + "26.11.01": "offline"}, "", true},
	} {
		look := func(_ context.Context, ref, _ string) (string, string, error) {
			switch v, ok := c.images[ref]; {
			case v == "offline":
				return "", "", offline
			case ok:
				return "sha256:d", v, nil
			}
			return "", "", errNoImage
		}
		u, err := decideUpdate(context.Background(), look, r, c.tag, c.here, nil)
		if c.err {
			if err == nil {
				t.Errorf("%s: no error", c.name)
			}
			continue
		}
		if err != nil || u.State != c.state {
			t.Errorf("%s: %+v %v", c.name, u, err)
			continue
		}
		// Never a part alone.
		if u.State == SuplaAvailable {
			if len(u.Images) != 2 || u.Images["supla-cloud"] == "" || u.Images["supla-server"] == "" {
				t.Errorf("%s: not the whole package: %v", c.name, u.Images)
			}
		} else if len(u.Images) != 0 {
			t.Errorf("%s: images without an update: %v", c.name, u.Images)
		}
	}
}

// TestSuplaReleaseLive reads the real release and registry, only with
// SUPLAOS_LIVE set.
func TestSuplaReleaseLive(t *testing.T) {
	if os.Getenv("SUPLAOS_LIVE") == "" {
		t.Skip("SUPLAOS_LIVE not set")
	}
	r, err := LatestSuplaRelease(context.Background())
	must(t, err)
	u, err := decideUpdate(context.Background(), lookupImage, r, "", SuplaVersions{}, nil)
	must(t, err)
	t.Logf("%s %+v: %s %v %v", r.Tag, r.Versions, u.State, u.Images, u.Waiting)
}

func TestDecideUpdateDatabase(t *testing.T) {
	r := &SuplaRelease{Tag: "v26.11", Versions: SuplaVersions{"26.11.01", "26.11.01"}}
	for _, c := range []struct {
		name, here, release string
		noImage             bool
		state               string
		raise               bool
	}{
		{"higher in the release, its tag installed", "mariadb:12.2.2", "mariadb:12.3.3", false, SuplaAvailable, true},
		{"the same", "mariadb:12.3.3", "mariadb:12.3.3", false, SuplaCurrent, false},
		{"never lower", "mariadb:12.3.3", "mariadb:12.2.2", false, SuplaCurrent, false},
		{"another database", "mysql:5.7", "mariadb:12.3.3", false, SuplaCurrent, false},
		{"its image is not out", "mariadb:12.2.2", "mariadb:12.3.3", true, SuplaWaiting, true},
	} {
		look := func(_ context.Context, ref, _ string) (string, string, error) {
			if c.noImage && strings.HasPrefix(ref, "mariadb") {
				return "", "", errNoImage
			}
			return "sha256:d", "", nil
		}
		db := &DatabaseVersions{Here: c.here, Release: c.release}
		u, err := decideUpdate(context.Background(), look, r, "v26.11", r.Versions, db)
		if err != nil || u.State != c.state || db.Raise != c.raise {
			t.Errorf("%s: %+v %v", c.name, u, err)
			continue
		}
		if u.State == SuplaAvailable && (len(u.Images) != 3 || u.Images["supla-db"] != c.release+"@sha256:d") {
			t.Errorf("%s: not the whole package: %v", c.name, u.Images)
		}
	}
}

func TestReleaseFromFeed(t *testing.T) {
	feed := `<feed xmlns="http://www.w3.org/2005/Atom">
<entry><updated>2026-11-22T17:14:32Z</updated><link rel="alternate" href="https://github.com/SUPLA/supla-docker/releases/tag/v26.11"/>
<content type="html">&lt;table&gt;&lt;td&gt;&lt;img data-canonical-src="https://img.shields.io/github/v/release/SUPLA/supla-cloud?filter=v26.11.01"&gt;
&lt;img data-canonical-src="https://img.shields.io/github/v/release/SUPLA/supla-core?filter=v26.11.02"&gt;&lt;/td&gt;&lt;/table&gt;
&lt;p&gt;In &lt;code&gt;.env&lt;/code&gt; rename:&lt;/p&gt;&lt;ul&gt;&lt;li&gt;A &amp;amp; B&lt;/li&gt;&lt;/ul&gt;</content></entry>
<entry><updated>2026-05-13T21:11:23Z</updated><link rel="alternate" href="https://github.com/SUPLA/supla-docker/releases/tag/v26.05"/><content type="html">old</content></entry>
</feed>`
	r, err := releaseFromFeed(strings.NewReader(feed), "v26.11")
	if err != nil || r.Versions != (SuplaVersions{"26.11.01", "26.11.02"}) || r.Notes != "In `.env` rename:* A & B" {
		t.Errorf("%+v %v", r, err)
	}
	if _, err := releaseFromFeed(strings.NewReader(feed), "v27.01"); err == nil {
		t.Error("a release not in the feed")
	}
}

func TestReleaseWebLive(t *testing.T) {
	if os.Getenv("SUPLAOS_LIVE") == "" {
		t.Skip("SUPLAOS_LIVE not set")
	}
	r, err := releaseFromWeb(context.Background())
	must(t, err)
	if r.Versions == (SuplaVersions{}) {
		t.Fatalf("no versions: %+v", r)
	}
	t.Logf("%s %+v", r.Tag, r.Versions)
}

package stack

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/Goral64/SuplaOS/internal/addon"
)

func TestImageSet(t *testing.T) {
	s := newStack(t)
	must(t, s.Install("supla", nil))
	a := s.Catalog["supla"]
	cloud, server := a.Images["supla-cloud"], a.Images["supla-server"]
	file := func() string {
		data, err := os.ReadFile(filepath.Join(s.Dir(), pinsFile("supla")))
		must(t, err)
		return string(data)
	}
	uses := func(ref string) bool { return strings.Contains(file(), "\n    image: "+ref+"\n") }
	running := func(cloudV, serverV string) {
		containerImage = func(svc, _ string) (*usedImage, error) {
			v := map[string]string{"supla-cloud": cloudV, "supla-server": serverV}[svc]
			return &usedImage{Ref: "supla/" + svc + "@sha256:used", Version: v}, nil
		}
	}
	if !uses(cloud.Image) || !uses(server.Image) {
		t.Fatalf("new install without the set:\n%s", file())
	}

	// Older, the same, and a mixed pair: the set.
	for _, v := range [][2]string{{"26.05.01", "v26.05"}, {cloud.Tag(), "v" + server.Tag()}, {cloud.Tag(), "v26.05"}, {"", ""}} {
		running(v[0], v[1])
		must(t, s.Apply())
		if !uses(cloud.Image) || !uses(server.Image) || len(s.State.Addons["supla"].Images) != 0 {
			t.Errorf("%v: not the set:\n%s", v, file())
		}
	}

	// One newer: all kept, also without containers for a moment.
	running("99.01", "v"+server.Tag())
	must(t, s.Apply())
	check := func(when string) {
		t.Helper()
		if !uses("supla/supla-cloud@sha256:used") || !uses("supla/supla-server@sha256:used") {
			t.Errorf("%s: older set over newer images:\n%s", when, file())
		}
	}
	check("newer")
	containerImage = func(string, string) (*usedImage, error) { return nil, nil }
	must(t, s.Apply())
	check("no containers")
	again, err := Open(s.Root, "../../addons")
	must(t, err)
	if len(again.State.Addons["supla"].Images) != 2 {
		t.Error("kept images not saved")
	}
	if p, err := s.Pending(); err != nil || len(p) != 0 {
		t.Errorf("pending with kept images: %v %v", p, err)
	}

	// A set that is not older waits to be applied and replaces them.
	next := cloud
	next.Image = cloud.Repo() + ":99.02@" + cloud.Digest()
	a.Images["supla-cloud"] = next
	if p, _ := s.Pending(); !slices.Equal(p, []string{"supla"}) {
		t.Errorf("new set not pending: %v", p)
	}
	running("99.01", "v"+server.Tag())
	must(t, s.Apply())
	if !uses(next.Image) || !uses(server.Image) || len(s.State.Addons["supla"].Images) != 0 {
		t.Errorf("new set not used:\n%s", file())
	}
}

func TestPinnedInfo(t *testing.T) {
	m := imageMeta{set: addon.Image{Image: "supla/supla-cloud:26.11.01@sha256:aaa"}}
	if i := pinnedInfo(Image{}, m, &localImage{RepoDigests: []string{"supla/supla-cloud@sha256:aaa"}}); i.Update || !i.Checked {
		t.Errorf("the set shown as an update: %+v", i)
	}
	old := &localImage{RepoDigests: []string{"supla/supla-cloud@sha256:bbb"}}
	if i := pinnedInfo(Image{}, m, old); !i.Update || i.NewVersion != "26.11.01" {
		t.Errorf("set not in use: %+v", i)
	}
	m.kept = "supla/supla-cloud@sha256:bbb"
	if i := pinnedInfo(Image{}, m, old); i.Update {
		t.Errorf("kept shown as an update: %+v", i)
	}
}

func TestNotWorking(t *testing.T) {
	svcs := []string{"db", "server"}
	for _, c := range []struct {
		state, health   string
		waiting, failed string
	}{
		{"running", "", "", ""},
		{"running", "healthy", "", ""},
		{"running", "starting", "server", ""},
		{"running", "unhealthy", "", "server"},
		{"created", "", "server", ""},
		{"exited", "", "", "server"},
	} {
		w, f := notWorking(svcs, []Container{{Service: "db", State: "running"}, {Service: "server", State: c.state, Health: c.health}})
		if w != c.waiting || f != c.failed {
			t.Errorf("%+v: %q %q", c, w, f)
		}
	}
	if w, _ := notWorking(svcs, nil); w == "" {
		t.Error("no containers taken as working")
	}
}

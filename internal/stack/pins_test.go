package stack

import (
	"testing"
)

func TestPinnedInfo(t *testing.T) {
	m := imageMeta{use: "supla/supla-cloud:26.11.01@sha256:aaa"}
	if i := pinnedInfo(Image{}, m, &localImage{RepoDigests: []string{"supla/supla-cloud@sha256:aaa"}}); i.Update || !i.Checked {
		t.Errorf("the set shown as an update: %+v", i)
	}
	old := &localImage{RepoDigests: []string{"supla/supla-cloud@sha256:bbb"}}
	if i := pinnedInfo(Image{}, m, old); !i.Update || i.NewVersion != "26.11.01" {
		t.Errorf("set not in use: %+v", i)
	}
	m.use = "supla/supla-cloud@sha256:bbb"
	if i := pinnedInfo(Image{}, m, old); i.Update {
		t.Errorf("the package shown as an update: %+v", i)
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

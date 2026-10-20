package panel

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"testing/fstest"
	"time"
)

func newServer(t *testing.T) *Server {
	t.Helper()
	return New(Config{
		Root:     t.TempDir(),
		Addons:   "../../addons",
		Settings: t.TempDir() + "/panel.yaml",
		Web: fstest.MapFS{
			"index.html":    {Data: []byte("<html>panel</html>")},
			"assets/app.js": {Data: []byte("app")},
		},
	})
}

func do(t *testing.T, h http.Handler, method, path, token string, header bool) *httptest.ResponseRecorder {
	t.Helper()
	r := httptest.NewRequest(method, path, strings.NewReader("{}"))
	if token != "" {
		r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	}
	if header {
		r.Header.Set("X-SuplaOS", "1")
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestAuth(t *testing.T) {
	s := newServer(t)
	h := s.Handler()
	if w := do(t, h, "GET", "/api/addons", "", false); w.Code != http.StatusUnauthorized {
		t.Errorf("no session: %d", w.Code)
	}
	token := s.sessions.create(false)
	if w := do(t, h, "GET", "/api/addons", token, false); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), `"id":"supla"`) {
		t.Errorf("addons: %d %s", w.Code, w.Body)
	}
	if w := do(t, h, "PUT", "/api/settings", token, false); w.Code != http.StatusForbidden {
		t.Errorf("change without X-SuplaOS: %d", w.Code)
	}
	must := s.sessions.create(true)
	if w := do(t, h, "GET", "/api/addons", must, false); w.Code != http.StatusForbidden {
		t.Errorf("default password session: %d", w.Code)
	}
	if w := do(t, h, "GET", "/api/session", must, false); !strings.Contains(w.Body.String(), `"mustChange":true`) {
		t.Errorf("session: %s", w.Body)
	}
}

func TestSessionsSurviveRestart(t *testing.T) {
	file := t.TempDir() + "/sessions.json"
	var a sessions
	a.load(file)
	token := a.create(false)
	var b sessions
	b.load(file)
	r := httptest.NewRequest("GET", "/", nil)
	r.AddCookie(&http.Cookie{Name: cookieName, Value: token})
	if b.get(r) == nil {
		t.Fatal("session lost after restart")
	}
	if data, _ := os.ReadFile(file); strings.Contains(string(data), token) {
		t.Error("token stored in plain text")
	}
	b.delete(r)
	var c sessions
	c.load(file)
	if c.get(r) != nil {
		t.Error("deleted session came back")
	}
}

func TestStatic(t *testing.T) {
	h := newServer(t).Handler()
	for _, p := range []string{"/", "/addons/supla", "/system"} {
		if w := do(t, h, "GET", p, "", false); w.Code != http.StatusOK || !strings.Contains(w.Body.String(), "panel") {
			t.Errorf("%s: %d %q", p, w.Code, w.Body)
		}
	}
	w := do(t, h, "GET", "/assets/app.js", "", false)
	if w.Body.String() != "app" || !strings.Contains(w.Header().Get("Cache-Control"), "immutable") {
		t.Errorf("asset: %q %v", w.Body, w.Header())
	}
}

func TestJobs(t *testing.T) {
	var js jobs
	release := make(chan struct{})
	j, err := js.start("first", func(j *Job) error {
		j.Write([]byte("one\rtwo\nthree"))
		<-release
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := js.start("second", func(*Job) error { return nil }); err == nil {
		t.Fatal("second job started while the first runs")
	}
	var got []string
	done := make(chan bool)
	go func() {
		done <- j.follow(context.Background(), func(l []string) { got = append(got, l...) })
	}()
	close(release)
	select {
	case ok := <-done:
		if !ok || strings.Join(got, "|") != "one|two|three" || j.State != "done" {
			t.Errorf("follow = %v %q %s", ok, got, j.State)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("follow did not end")
	}
}

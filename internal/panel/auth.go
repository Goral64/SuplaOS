package panel

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"sync"
	"time"
)

const (
	cookieName     = "suplaos_session"
	sessionTimeout = 12 * time.Hour
	// After maxFailures wrong passwords in a row logins wait failDelay,
	// twice as long after each next one, up to maxFailDelay.
	maxFailures  = 5
	failDelay    = 30 * time.Second
	maxFailDelay = time.Hour
)

type session struct {
	Expires time.Time `json:"expires"`
	// MustChange: logged in with the default password, which has to be
	// changed before anything else.
	MustChange bool `json:"mustChange"`
}

// sessions are kept in memory and in a file, so that they survive panel
// restarts (e.g. updates). They are stored by the hash of the token.
type sessions struct {
	mu    sync.Mutex
	file  string
	list  map[string]*session
	saved time.Time
	// failed are the wrong passwords in a row by address.
	failed map[string]*failures
}

type failures struct {
	count int
	last  time.Time
}

func newToken() string {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		panic(err)
	}
	return hex.EncodeToString(b)
}

func hashToken(t string) string {
	h := sha256.Sum256([]byte(t))
	return hex.EncodeToString(h[:])
}

// load reads the sessions saved by an earlier run.
func (s *sessions) load(file string) {
	s.file = file
	s.list = map[string]*session{}
	if data, err := os.ReadFile(file); err == nil {
		if err := json.Unmarshal(data, &s.list); err != nil {
			log.Printf("%s: %v", file, err)
			s.list = map[string]*session{}
		}
	}
}

// save writes the sessions without the expired ones; called with s.mu held.
func (s *sessions) save() {
	for h, ses := range s.list {
		if time.Now().After(ses.Expires) {
			delete(s.list, h)
		}
	}
	s.saved = time.Now()
	if s.file == "" {
		return
	}
	data, _ := json.Marshal(s.list)
	err := os.MkdirAll(filepath.Dir(s.file), 0o700)
	if err == nil {
		err = os.WriteFile(s.file, data, 0o600)
	}
	if err != nil {
		log.Printf("sessions: %v", err)
	}
}

func (s *sessions) create(mustChange bool) string {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.list == nil {
		s.list = map[string]*session{}
	}
	t := newToken()
	s.list[hashToken(t)] = &session{Expires: time.Now().Add(sessionTimeout), MustChange: mustChange}
	s.save()
	return t
}

// get returns the session of the request and extends it.
func (s *sessions) get(r *http.Request) *session {
	c, err := r.Cookie(cookieName)
	if err != nil {
		return nil
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	ses := s.list[hashToken(c.Value)]
	if ses == nil || time.Now().After(ses.Expires) {
		return nil
	}
	ses.Expires = time.Now().Add(sessionTimeout)
	// The extension is saved now and then, not on every request.
	if time.Since(s.saved) > 10*time.Minute {
		s.save()
	}
	return ses
}

func (s *sessions) delete(r *http.Request) {
	if c, err := r.Cookie(cookieName); err == nil {
		s.mu.Lock()
		delete(s.list, hashToken(c.Value))
		s.save()
		s.mu.Unlock()
	}
}

// passwordChanged ends the other sessions and clears mustChange.
func (s *sessions) passwordChanged(r *http.Request) {
	keep := ""
	if c, err := r.Cookie(cookieName); err == nil {
		keep = hashToken(c.Value)
	}
	s.mu.Lock()
	defer s.mu.Unlock()
	for h, ses := range s.list {
		if h != keep {
			delete(s.list, h)
		} else {
			ses.MustChange = false
		}
	}
	s.save()
}

// throttled tells whether logins from ip have to wait after wrong
// passwords. Counted by address, so that wrong passwords sent from the
// internet do not keep the owner out.
func (s *sessions) throttled(ip string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	f := s.failed[ip]
	if f == nil || f.count < maxFailures {
		return false
	}
	wait := failDelay
	for i := maxFailures; i < f.count && wait < maxFailDelay; i++ {
		wait *= 2
	}
	return time.Since(f.last) < min(wait, maxFailDelay)
}

func (s *sessions) result(ip string, ok bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if ok {
		delete(s.failed, ip)
		return
	}
	if s.failed == nil {
		s.failed = map[string]*failures{}
	}
	for k, f := range s.failed {
		if time.Since(f.last) > maxFailDelay {
			delete(s.failed, k)
		}
	}
	f := s.failed[ip]
	if f == nil {
		f = &failures{}
		s.failed[ip] = f
	}
	f.count++
	f.last = time.Now()
}

func setCookie(w http.ResponseWriter, token string, maxAge int) {
	http.SetCookie(w, &http.Cookie{
		Name: cookieName, Value: token, Path: "/", MaxAge: maxAge,
		HttpOnly: true, Secure: true, SameSite: http.SameSiteStrictMode,
	})
}

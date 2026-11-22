package panel

import (
	"context"
	"errors"
	"log"
	"net/http"
	"sync"
	"time"

	"github.com/Goral64/SuplaOS/internal/stack"
)

// Newer images of the add-ons are looked for every few hours, with the
// update check and after installing and updating add-ons; the result is
// kept in memory.
const imagesInterval = 6 * time.Hour

type imageState struct {
	mu      sync.Mutex
	images  map[string]stack.Image
	checked time.Time
	// supla is what the newest release of supla-docker means here.
	supla    *stack.SuplaUpdate
	suplaErr string
}

func (s *Server) checkImages() error {
	st, err := s.openStack()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	s.checkSupla(ctx, st)
	images, err := st.Images(ctx)
	if images != nil {
		s.images.mu.Lock()
		s.images.images, s.images.checked = images, time.Now()
		s.images.mu.Unlock()
	}
	return err
}

// checkSupla looks for a release of supla-docker: only that is an update
// of Supla, not a new image of a part.
func (s *Server) checkSupla(ctx context.Context, st *stack.Stack) {
	var u *stack.SuplaUpdate
	msg := ""
	if st.IsInstalled(stack.SuplaID) {
		var err error
		if u, err = st.CheckSupla(ctx); err != nil {
			msg = err.Error()
			log.Printf("update of Supla: %v", err)
		}
	}
	s.images.mu.Lock()
	defer s.images.mu.Unlock()
	s.images.supla, s.images.suplaErr = u, msg
}

// suplaUpdateState is the update state of the Supla add-on.
func (s *Server) suplaUpdateState() string {
	s.images.mu.Lock()
	defer s.images.mu.Unlock()
	switch {
	case s.images.supla == nil:
		return ""
	case s.images.supla.State == stack.SuplaAvailable:
		return "available"
	}
	return "none"
}

func (s *Server) getSuplaUpdate(w http.ResponseWriter, r *http.Request) {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	s.images.mu.Lock()
	u, msg := s.images.supla, s.images.suplaErr
	s.images.mu.Unlock()
	writeJSON(w, http.StatusOK, map[string]any{"installed": st.IsInstalled(stack.SuplaID), "update": u, "error": msg})
}

func (s *Server) postSuplaCheck(w http.ResponseWriter, r *http.Request) {
	s.stackJob(w, "check for an update of Supla", func(st *stack.Stack, j *Job) error {
		ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
		defer cancel()
		j.Logf("Looking at the releases of supla-docker")
		s.checkSupla(ctx, st)
		s.images.mu.Lock()
		u, msg := s.images.supla, s.images.suplaErr
		s.images.mu.Unlock()
		if msg != "" {
			return errors.New(msg)
		}
		if u != nil {
			j.Logf("supla-docker %s: %s", u.Release.Tag, u.State)
		}
		return nil
	})
}

func (s *Server) postSuplaUpgrade(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Tag string `json:"tag"`
	}
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if req.Tag == "" {
		jsonError(w, http.StatusBadRequest, errors.New("no release given"))
		return
	}
	s.stackJob(w, "update Supla", func(st *stack.Stack, j *Job) error {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
		defer cancel()
		err := st.UpgradeSupla(ctx, req.Tag)
		s.checkImagesAfter(j)
		return err
	})
}

// checkImagesAfter refreshes the images after a job changed them; a
// registry being down does not fail the job.
func (s *Server) checkImagesAfter(j *Job) {
	if err := s.checkImages(); err != nil {
		j.Logf("Checking images: %v", err)
	}
}

// WatchImages checks for newer images now and then; it does not return.
func (s *Server) WatchImages() {
	time.Sleep(time.Minute)
	for {
		if err := s.checkImages(); err != nil {
			log.Printf("images: %v", err)
		}
		time.Sleep(imagesInterval)
	}
}

// addonImages returns the known images of the services and whether there
// are newer ones: "available", "none", or "" when not all were checked.
func (s *Server) addonImages(services []string) (map[string]stack.Image, string) {
	s.images.mu.Lock()
	defer s.images.mu.Unlock()
	images := map[string]stack.Image{}
	state := "none"
	for _, svc := range services {
		img, ok := s.images.images[svc]
		if !ok {
			state = ""
			continue
		}
		images[svc] = img
		if img.Update {
			state = "available"
		} else if !img.Checked && state != "available" {
			state = ""
		}
	}
	return images, state
}

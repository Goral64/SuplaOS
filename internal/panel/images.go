package panel

import (
	"context"
	"log"
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
}

func (s *Server) checkImages() error {
	st, err := s.openStack()
	if err != nil {
		return err
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	images, err := st.Images(ctx)
	if images != nil {
		s.images.mu.Lock()
		s.images.images, s.images.checked = images, time.Now()
		s.images.mu.Unlock()
	}
	return err
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

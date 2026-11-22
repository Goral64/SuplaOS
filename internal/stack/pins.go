package stack

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"os/exec"
	"slices"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
)

// An add-on with images in addon.yaml is updated as a set: exact images
// tested together, that come with SuplaOS. The compose file with them is
// generated, so a new set is pending like any other new file of an add-on.

func pinsFile(id string) string { return "docker-compose." + id + "-images.yml" }

// pinsCompose is the compose file with the images of the set, or with
// those kept (Installed.Images). The set is in it either way, so that a
// new one is pending also when images are kept.
func pinsCompose(a *addon.Addon, kept map[string]string) []byte {
	var sb strings.Builder
	sb.WriteString(header)
	services := slices.Sorted(maps.Keys(a.Images))
	for _, svc := range services {
		if pin := a.Images[svc].Image; pin != "" {
			fmt.Fprintf(&sb, "# Set of SuplaOS: %s\n", pin)
		}
	}
	sb.WriteString("services:\n")
	for _, svc := range services {
		ref := a.Images[svc].Image
		if ref == "" {
			continue
		}
		if k := kept[svc]; k != "" {
			ref = k
		}
		fmt.Fprintf(&sb, "  %s:\n    image: %s\n", svc, ref)
	}
	return []byte(sb.String())
}

// usedImage is the image of the container of a service.
type usedImage struct {
	Ref, Version string
}

// containerImage returns the image of the container of a service, nil
// without a container or without docker.
var containerImage = func(svc, versionEnv string) (*usedImage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	out, err := exec.CommandContext(ctx, "docker", "ps", "-a", "-q",
		"--filter", "label=com.docker.compose.project="+ProjectName,
		"--filter", "label=com.docker.compose.service="+svc,
		"--filter", "label=com.docker.compose.oneoff=False").Output()
	if errors.Is(err, exec.ErrNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, fmt.Errorf("docker ps: %w", err)
	}
	ids := strings.Fields(string(out))
	if len(ids) == 0 {
		return nil, nil
	}
	if out, err = exec.CommandContext(ctx, "docker", "inspect", "--format", "{{.Image}}", ids[0]).Output(); err != nil {
		return nil, fmt.Errorf("docker inspect: %w", err)
	}
	id := strings.TrimSpace(string(out))
	img, err := inspectImage(ctx, id)
	if err != nil {
		return nil, err
	}
	u := &usedImage{Ref: id, Version: img.Config.version(versionEnv)}
	if len(img.RepoDigests) > 0 {
		u.Ref = img.RepoDigests[0]
	}
	return u, nil
}

// imageNewer compares versions of images, some have a v in front.
func imageNewer(a, b string) bool {
	return versionNewer(strings.TrimPrefix(a, "v"), strings.TrimPrefix(b, "v"))
}

// keepNewer keeps the images of an add-on that already runs newer ones
// than the set, e.g. updated before SuplaOS had sets: the database is
// migrated by then and an older set would not start with it. All of the
// set are kept, until SuplaOS brings one that is not older. It tells
// whether the state changed.
func (s *Stack) keepNewer() (bool, error) {
	changed := false
	for _, a := range s.Order() {
		inst := s.State.Addons[a.ID]
		used := map[string]string{}
		newer := false
		for svc, m := range a.Images {
			if m.Image == "" {
				continue
			}
			u, err := containerImage(svc, m.VersionEnv)
			if err != nil {
				return changed, fmt.Errorf("%s: %w", svc, err)
			}
			if u == nil {
				continue
			}
			used[svc] = u.Ref
			newer = newer || imageNewer(u.Version, m.Tag())
		}
		if len(used) == 0 {
			// No containers to compare with: as it was.
			continue
		}
		var keep map[string]string
		if newer {
			keep = used
		}
		if !maps.Equal(keep, inst.Images) {
			inst.Images = keep
			changed = true
		}
	}
	return changed, nil
}

// How long Verify waits for the containers, and how often it looks.
var (
	verifyTimeout = 8 * time.Minute
	verifyEvery   = 5 * time.Second
)

// Verify waits until the services of the add-ons updated as a set work:
// a container may run with nothing working in it (supla-server gives up
// on a database it does not know, supervisord stays). Other add-ons are
// not checked.
func (s *Stack) Verify(ids ...string) error {
	for _, id := range ids {
		a := s.Catalog[id]
		if a == nil || !s.IsInstalled(id) || !a.Pinned() {
			continue
		}
		fmt.Fprintf(s.stdout(), "Checking that %s works\n", id)
		deadline := time.Now().Add(verifyTimeout)
		for {
			ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
			list, err := s.Containers(ctx)
			cancel()
			if err != nil {
				return err
			}
			waiting, failed := notWorking(a.Services, list)
			if failed != "" {
				// The reason is in the log of the service.
				if err := s.Compose("logs", "--no-color", "--tail", "15", failed); err != nil {
					fmt.Fprintln(s.stdout(), err)
				}
				return fmt.Errorf("%s does not work, see its log above", failed)
			}
			if waiting == "" {
				break
			}
			if time.Now().After(deadline) {
				return fmt.Errorf("%s did not start in %s", waiting, verifyTimeout)
			}
			time.Sleep(verifyEvery)
		}
	}
	return nil
}

// VerifyAll is Verify for all installed add-ons.
func (s *Stack) VerifyAll() error {
	var ids []string
	for _, a := range s.Order() {
		ids = append(ids, a.ID)
	}
	return s.Verify(ids...)
}

// notWorking returns a service still starting and one that failed.
func notWorking(services []string, list []Container) (waiting, failed string) {
	for _, svc := range services {
		i := slices.IndexFunc(list, func(c Container) bool { return c.Service == svc })
		switch {
		case i < 0:
			waiting = svc
		case list[i].Health == "unhealthy", list[i].State == "exited", list[i].State == "dead":
			return "", svc
		case list[i].State != "running", list[i].Health == "starting":
			waiting = svc
		}
	}
	return waiting, ""
}

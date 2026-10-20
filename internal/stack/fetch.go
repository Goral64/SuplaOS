package stack

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
)

const sourceFile = ".source"

// fetch downloads the files of an add-on from its upstream archive into the
// cache, unless they are already there from the same URL.
func (s *Stack) fetch(a *addon.Addon) error {
	dir := s.cacheDir(a.ID)
	if src, err := os.ReadFile(filepath.Join(dir, sourceFile)); err == nil && string(src) == a.Fetch.URL {
		return nil
	}
	client := &http.Client{Timeout: 2 * time.Minute}
	resp, err := client.Get(a.Fetch.URL)
	if err != nil {
		return fmt.Errorf("%s: download: %w", a.ID, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("%s: download %s: %s", a.ID, a.Fetch.URL, resp.Status)
	}
	files, err := extract(resp.Body, a.Fetch.Files)
	if err != nil {
		return fmt.Errorf("%s: %s: %w", a.ID, a.Fetch.URL, err)
	}
	if err := os.RemoveAll(dir); err != nil {
		return err
	}
	for name, data := range files {
		if err := writeFile(filepath.Join(dir, name), data, 0o644); err != nil {
			return err
		}
	}
	return writeFile(filepath.Join(dir, sourceFile), []byte(a.Fetch.URL), 0o644)
}

// Refetch downloads again the files of installed add-ons whose upstream URL
// changed in the catalog.
func (s *Stack) Refetch() error {
	for _, a := range s.Order() {
		if a.Fetch != nil {
			if err := s.fetch(a); err != nil {
				return err
			}
		}
	}
	return nil
}

// extract reads the wanted files from a tar.gz archive, skipping the top
// directory (GitHub archives have one).
func extract(r io.Reader, want []string) (map[string][]byte, error) {
	gz, err := gzip.NewReader(r)
	if err != nil {
		return nil, err
	}
	wanted := map[string]bool{}
	for _, w := range want {
		wanted[w] = true
	}
	out := map[string][]byte{}
	tr := tar.NewReader(gz)
	for {
		h, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		_, name, ok := strings.Cut(h.Name, "/")
		if !ok || !wanted[name] || h.Typeflag != tar.TypeReg {
			continue
		}
		data, err := io.ReadAll(io.LimitReader(tr, 1<<20))
		if err != nil {
			return nil, err
		}
		out[name] = data
	}
	for _, w := range want {
		if out[w] == nil {
			return nil, fmt.Errorf("%s not found in the archive", w)
		}
	}
	return out, nil
}

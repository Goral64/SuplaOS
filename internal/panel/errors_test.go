package panel

import (
	"encoding/json"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"testing"
)

// TestErrorCatalog checks that every error message of the Go code is in the
// catalog the panel shows them translated with: translated, or kept in
// English as technical.
func TestErrorCatalog(t *testing.T) {
	data, err := os.ReadFile("../../web/src/locales/errors.pl.json")
	if err != nil {
		t.Fatal(err)
	}
	var catalog struct {
		Messages  map[string]string `json:"messages"`
		Technical []string          `json:"technical"`
	}
	if err := json.Unmarshal(data, &catalog); err != nil {
		t.Fatal(err)
	}
	known := map[string]bool{}
	for _, s := range catalog.Technical {
		known[s] = true
	}
	verb := regexp.MustCompile(`%#?[a-z]`)
	ref := regexp.MustCompile(`\{(\d)\}`)
	for en, pl := range catalog.Messages {
		known[en] = true
		n := len(verb.FindAllString(en, -1))
		for _, m := range ref.FindAllStringSubmatch(pl, -1) {
			if i, _ := strconv.Atoi(m[1]); i >= n {
				t.Errorf("%q: {%d}, the message has %d verbs", en, i, n)
			}
		}
	}
	call := regexp.MustCompile(`(?:errors\.New|fmt\.Errorf)\(\s*("(?:[^"\\]|\\.)*")`)
	filepath.WalkDir("..", func(path string, d os.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		src, err := os.ReadFile(path)
		if err != nil {
			return err
		}
		for _, m := range call.FindAllStringSubmatch(string(src), -1) {
			s, err := strconv.Unquote(m[1])
			if err == nil && !known[s] {
				t.Errorf("%s: %q is not in web/src/locales/errors.pl.json", path, s)
			}
		}
		return nil
	})
}

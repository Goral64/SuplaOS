package stack

import (
	"context"
	"encoding/xml"
	"fmt"
	"html"
	"io"
	"net/http"
	"path"
	"regexp"
	"strings"
	"time"
)

// The API of GitHub answers 60 requests an hour for an IP address without
// a token, and providers put many homes behind one. The web pages have no
// such limit: releases/latest redirects to the newest release (not a draft
// or a pre-release) and the feed has the notes of it, as HTML.

const (
	suplaLatestURL = "https://github.com/SUPLA/supla-docker/releases/latest"
	suplaFeedURL   = "https://github.com/SUPLA/supla-docker/releases.atom"
)

func releaseFromWeb(ctx context.Context) (*SuplaRelease, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, suplaLatestURL, nil)
	if err != nil {
		return nil, err
	}
	client := &http.Client{CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }}
	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	resp.Body.Close()
	tag := path.Base(resp.Header.Get("Location"))
	if !releaseTagRe.MatchString(tag) {
		return nil, fmt.Errorf("the newest release of supla-docker: %s", resp.Status)
	}
	if req, err = http.NewRequestWithContext(ctx, http.MethodGet, suplaFeedURL, nil); err != nil {
		return nil, err
	}
	if resp, err = http.DefaultClient.Do(req); err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("the feed of supla-docker releases: %s", resp.Status)
	}
	return releaseFromFeed(io.LimitReader(resp.Body, 4<<20), tag)
}

// releaseFromFeed finds the release with the tag in the feed.
func releaseFromFeed(r io.Reader, tag string) (*SuplaRelease, error) {
	var feed struct {
		Entries []struct {
			Updated time.Time `xml:"updated"`
			Link    struct {
				Href string `xml:"href,attr"`
			} `xml:"link"`
			Content string `xml:"content"`
		} `xml:"entry"`
	}
	if err := xml.NewDecoder(r).Decode(&feed); err != nil {
		return nil, fmt.Errorf("the feed of supla-docker releases: %w", err)
	}
	for _, e := range feed.Entries {
		if path.Base(e.Link.Href) == tag {
			return &SuplaRelease{Tag: tag, URL: e.Link.Href, Published: e.Updated,
				Notes: notesText(e.Content), Versions: releaseVersions(e.Content)}, nil
		}
	}
	return nil, fmt.Errorf("supla-docker %s is not in the feed of releases", tag)
}

var (
	htmlTableRe = regexp.MustCompile(`(?s)<table>.*?</table>`)
	htmlTagRe   = regexp.MustCompile(`<[^>]*>`)
)

// notesText makes text of the notes in HTML: steps to do by hand, without
// the table of versions.
func notesText(s string) string {
	s = htmlTableRe.ReplaceAllString(s, "")
	s = strings.NewReplacer("<li>", "* ", "<code>", "`", "</code>", "`").Replace(s)
	return strings.TrimSpace(html.UnescapeString(htmlTagRe.ReplaceAllString(s, "")))
}

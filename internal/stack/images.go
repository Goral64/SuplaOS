package stack

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os/exec"
	"runtime"
	"strings"
)

// Image is the image of a service of an installed add-on.
type Image struct {
	// Version of the image the container runs.
	Version string `json:"version"`
	// Update: the registry has another image for the tag, of NewVersion.
	Update     bool   `json:"update"`
	NewVersion string `json:"newVersion"`
	// Build times of the images, for new builds of the same version.
	Created    string `json:"created"`
	NewCreated string `json:"newCreated"`
	Changelog  string `json:"changelog"`
	// Checked: the registry answered.
	Checked bool `json:"checked"`
}

// Images returns the images of the containers of the installed add-ons and
// asks the registries for newer ones. Services whose registry could not be
// asked are returned unchecked, the errors are joined.
func (s *Stack) Images(ctx context.Context) (map[string]Image, error) {
	result := map[string]Image{}
	if len(s.State.Addons) == 0 {
		return result, nil
	}
	containers, err := s.Containers(ctx)
	if err != nil {
		return nil, err
	}
	service := map[string]string{}
	for _, c := range containers {
		service[c.Name] = c.Service
	}
	meta := map[string]imageMeta{}
	for _, a := range s.Order() {
		for svc, m := range a.Images {
			meta[svc] = imageMeta{m.VersionEnv, m.Changelog}
		}
	}
	out, err := s.Command(ctx, "images", "--format", "json").Output()
	if err != nil {
		return nil, fmt.Errorf("docker compose images: %w", err)
	}
	images, err := decodeList[composeImage](out)
	if err != nil {
		return nil, err
	}
	type remoteImage struct {
		digest string
		config *imageConfig
		err    error
	}
	remote := map[string]*remoteImage{}
	var errs []error
	for _, img := range images {
		svc := service[img.ContainerName]
		if svc == "" {
			continue
		}
		m := meta[svc]
		local, err := inspectImage(ctx, img.ID)
		if err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", svc, err))
			continue
		}
		info := Image{Version: local.Config.version(m.versionEnv), Created: local.Created, Changelog: m.changelog}
		// Built locally or without a tag: nothing to compare with.
		if len(local.RepoDigests) == 0 || img.Repository == "" || img.Tag == "" || img.Tag == "<none>" {
			result[svc] = info
			continue
		}
		ref := img.Repository + ":" + img.Tag
		r := remote[ref]
		if r == nil {
			r = &remoteImage{}
			r.digest, r.err = RemoteDigest(ctx, ref)
			if r.err != nil {
				errs = append(errs, fmt.Errorf("%s: %w", ref, r.err))
			}
			remote[ref] = r
		}
		if r.err == nil {
			info.Checked = true
			info.Update = true
			for _, d := range local.RepoDigests {
				if strings.HasSuffix(d, "@"+r.digest) {
					info.Update = false
				}
			}
		}
		if info.Update {
			if r.config == nil {
				if r.config, err = RemoteConfig(ctx, ref, r.digest); err != nil {
					errs = append(errs, fmt.Errorf("%s: %w", ref, err))
					r.config = &imageConfig{}
				}
			}
			info.NewVersion = r.config.version(m.versionEnv)
			info.NewCreated = r.config.Created
		}
		result[svc] = info
	}
	return result, errors.Join(errs...)
}

type imageMeta struct {
	versionEnv, changelog string
}

type composeImage struct {
	ID            string `json:"ID"`
	ContainerName string `json:"ContainerName"`
	Repository    string `json:"Repository"`
	Tag           string `json:"Tag"`
}

// imageConfig is the part of the image configuration with the version.
type imageConfig struct {
	Labels map[string]string `json:"Labels"`
	Env    []string          `json:"Env"`
	// Created is the build time, outside the configuration in the image.
	Created string `json:"-"`
}

// version reads the version from the OCI label, or from the variable env
// for images without it.
func (c *imageConfig) version(env string) string {
	if v := c.Labels["org.opencontainers.image.version"]; v != "" || env == "" {
		return v
	}
	for _, kv := range c.Env {
		if k, v, _ := strings.Cut(kv, "="); k == env {
			return v
		}
	}
	return ""
}

// decodeList reads compose JSON output: an array, or one object per line.
func decodeList[T any](out []byte) ([]T, error) {
	var list []T
	t := strings.TrimSpace(string(out))
	if strings.HasPrefix(t, "[") {
		err := json.Unmarshal([]byte(t), &list)
		return list, err
	}
	for _, line := range strings.Split(t, "\n") {
		if strings.TrimSpace(line) == "" {
			continue
		}
		var v T
		if err := json.Unmarshal([]byte(line), &v); err != nil {
			return nil, err
		}
		list = append(list, v)
	}
	return list, nil
}

type localImage struct {
	// RepoDigests are like supla/supla-cloud@sha256:...
	RepoDigests []string    `json:"RepoDigests"`
	Config      imageConfig `json:"Config"`
	Created     string      `json:"Created"`
}

func inspectImage(ctx context.Context, id string) (*localImage, error) {
	out, err := exec.CommandContext(ctx, "docker", "image", "inspect", id).Output()
	if err != nil {
		return nil, fmt.Errorf("docker image inspect: %w", err)
	}
	var list []localImage
	if err := json.Unmarshal(out, &list); err != nil {
		return nil, err
	}
	if len(list) != 1 {
		return nil, fmt.Errorf("docker image inspect: %d images", len(list))
	}
	return &list[0], nil
}

// splitRef splits an image reference into the registry host, repository
// and tag: mariadb:12 is registry-1.docker.io, library/mariadb, 12.
func splitRef(ref string) (host, repo, tag string) {
	repo, tag = ref, "latest"
	if i := strings.LastIndex(ref, ":"); i > strings.LastIndex(ref, "/") {
		repo, tag = ref[:i], ref[i+1:]
	}
	host = "registry-1.docker.io"
	if first, rest, ok := strings.Cut(repo, "/"); ok && (strings.ContainsAny(first, ".:") || first == "localhost") {
		host, repo = first, rest
	}
	if host == "registry-1.docker.io" || host == "docker.io" {
		host = "registry-1.docker.io"
		if !strings.Contains(repo, "/") {
			repo = "library/" + repo
		}
	}
	return host, repo, tag
}

var manifestTypes = strings.Join([]string{
	"application/vnd.oci.image.index.v1+json",
	"application/vnd.docker.distribution.manifest.list.v2+json",
	"application/vnd.docker.distribution.manifest.v2+json",
	"application/vnd.oci.image.manifest.v1+json",
}, ", ")

// registry talks to one repository of a registry, with an anonymous token
// when the registry wants one (Docker Hub, ghcr.io).
type registry struct {
	host, repo, token string
}

// get requests /v2/<repo>/<path>; the caller closes the body.
func (r *registry) get(ctx context.Context, method, path string) (*http.Response, error) {
	for try := 0; ; try++ {
		req, err := http.NewRequestWithContext(ctx, method, "https://"+r.host+"/v2/"+r.repo+"/"+path, nil)
		if err != nil {
			return nil, err
		}
		req.Header.Set("Accept", manifestTypes)
		if r.token != "" {
			req.Header.Set("Authorization", "Bearer "+r.token)
		}
		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			return nil, err
		}
		if resp.StatusCode == http.StatusUnauthorized && try == 0 {
			resp.Body.Close()
			if r.token, err = anonymousToken(ctx, resp.Header.Get("WWW-Authenticate")); err != nil {
				return nil, err
			}
			continue
		}
		if resp.StatusCode != http.StatusOK {
			resp.Body.Close()
			return nil, fmt.Errorf("registry: %s", resp.Status)
		}
		return resp, nil
	}
}

func (r *registry) getJSON(ctx context.Context, path string, v any) error {
	resp, err := r.get(ctx, http.MethodGet, path)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(io.LimitReader(resp.Body, 4<<20)).Decode(v)
}

// RemoteDigest asks the registry for the digest of a tag. A HEAD request
// does not count to the Docker Hub pull limits.
func RemoteDigest(ctx context.Context, ref string) (string, error) {
	host, repo, tag := splitRef(ref)
	r := &registry{host: host, repo: repo}
	resp, err := r.get(ctx, http.MethodHead, "manifests/"+tag)
	if err != nil {
		return "", err
	}
	resp.Body.Close()
	digest := resp.Header.Get("Docker-Content-Digest")
	if digest == "" {
		return "", errors.New("registry: no digest")
	}
	return digest, nil
}

// RemoteConfig reads the configuration of the image with the digest, for
// the architecture of this device.
func RemoteConfig(ctx context.Context, ref, digest string) (*imageConfig, error) {
	host, repo, _ := splitRef(ref)
	r := &registry{host: host, repo: repo}
	var m struct {
		Manifests []struct {
			Digest   string `json:"digest"`
			Platform struct {
				OS           string `json:"os"`
				Architecture string `json:"architecture"`
			} `json:"platform"`
		} `json:"manifests"`
		Config struct {
			Digest string `json:"digest"`
		} `json:"config"`
	}
	if err := r.getJSON(ctx, "manifests/"+digest, &m); err != nil {
		return nil, err
	}
	if len(m.Manifests) > 0 {
		platform := ""
		for _, p := range m.Manifests {
			if p.Platform.OS == "linux" && p.Platform.Architecture == runtime.GOARCH {
				platform = p.Digest
				break
			}
		}
		if platform == "" {
			return nil, fmt.Errorf("no image for linux/%s", runtime.GOARCH)
		}
		m.Manifests = nil
		if err := r.getJSON(ctx, "manifests/"+platform, &m); err != nil {
			return nil, err
		}
	}
	if m.Config.Digest == "" {
		return nil, errors.New("registry: manifest without config")
	}
	var c struct {
		Created string      `json:"created"`
		Config  imageConfig `json:"config"`
	}
	if err := r.getJSON(ctx, "blobs/"+m.Config.Digest, &c); err != nil {
		return nil, err
	}
	c.Config.Created = c.Created
	return &c.Config, nil
}

// anonymousToken gets a pull token from the realm in a header like
// Bearer realm="https://auth.docker.io/token",service="registry.docker.io",scope="repository:library/mariadb:pull"
func anonymousToken(ctx context.Context, challenge string) (string, error) {
	scheme, params, _ := strings.Cut(challenge, " ")
	if !strings.EqualFold(scheme, "Bearer") {
		return "", fmt.Errorf("registry: unsupported authentication %q", scheme)
	}
	p := parseChallenge(params)
	if p["realm"] == "" {
		return "", errors.New("registry: no token realm")
	}
	q := url.Values{}
	for _, k := range []string{"service", "scope"} {
		if p[k] != "" {
			q.Set(k, p[k])
		}
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, p["realm"]+"?"+q.Encode(), nil)
	if err != nil {
		return "", err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("registry token: %s", resp.Status)
	}
	var t struct {
		Token       string `json:"token"`
		AccessToken string `json:"access_token"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&t); err != nil {
		return "", err
	}
	if t.Token != "" {
		return t.Token, nil
	}
	return t.AccessToken, nil
}

// parseChallenge reads key="value" pairs separated by commas.
func parseChallenge(s string) map[string]string {
	m := map[string]string{}
	for s != "" {
		k, rest, ok := strings.Cut(s, "=")
		if !ok {
			break
		}
		k = strings.TrimSpace(k)
		var v string
		if strings.HasPrefix(rest, `"`) {
			end := strings.Index(rest[1:], `"`)
			if end < 0 {
				break
			}
			v, rest = rest[1:end+1], rest[end+2:]
		} else {
			v, rest, _ = strings.Cut(rest, ",")
		}
		m[k] = v
		s = strings.TrimPrefix(strings.TrimSpace(rest), ",")
	}
	return m
}

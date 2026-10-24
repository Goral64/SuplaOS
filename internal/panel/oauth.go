package panel

import (
	"bytes"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/domaincert"
	"github.com/Goral64/SuplaOS/internal/stack"
)

// connectScripts makes (or finds) an OAuth app of SUPLA Scripts in the
// Supla Cloud of this device, with the account given, and puts its ID and
// secret in the settings of the add-on. The password is used only for this.
func (s *Server) connectScripts(w http.ResponseWriter, r *http.Request) {
	var req suplaAccount
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if req.Email == "" || req.Password == "" {
		jsonError(w, http.StatusBadRequest, errors.New("e-mail and password are required"))
		return
	}
	s.stackJob(w, "connect SUPLA Scripts with Supla Cloud", func(st *stack.Stack, j *Job) error {
		if !st.IsInstalled("supla") || !st.IsInstalled("supla-scripts") {
			return errors.New("Supla and SUPLA Scripts have to be installed")
		}
		supla := st.Values(st.Catalog["supla"])
		scripts := st.Values(st.Catalog["supla-scripts"])
		cloud, err := s.cloudClient("https://127.0.0.1:" + supla["PORT_HTTPS"])
		if err != nil {
			return err
		}
		j.Logf("Logging in to Supla Cloud as %s", req.Email)
		if err := cloud.login(req.Email, req.Password); err != nil {
			return err
		}
		redirect := strings.TrimSuffix(scripts["SUPLA_SCRIPTS_URL"], "/") + "/authorize"
		id, secret, err := cloud.oauthApp("SUPLA Scripts", redirect)
		if err != nil {
			return err
		}
		j.Logf("OAuth app %s, redirect address %s", id, redirect)
		set := map[string]string{
			"SUPLA_SCRIPTS_OAUTH_CLOUD":  "https://" + supla["SUPLA_HOST_ADDRESS"],
			"SUPLA_SCRIPTS_OAUTH_ID":     id,
			"SUPLA_SCRIPTS_OAUTH_SECRET": secret,
		}
		dropUnchanged(st, st.Catalog["supla-scripts"], set)
		if len(set) == 0 {
			j.Logf("SUPLA Scripts already uses this app.")
			return nil
		}
		if err := st.Configure("supla-scripts", set); err != nil {
			return err
		}
		return st.Up()
	})
}

// cloudAPI talks to the API of the Supla Cloud of this device.
type cloudAPI struct {
	url    string
	client *http.Client
	token  string
}

// cloudClient talks to the Cloud at url, an address of this device. The
// Cloud has the certificate of the domain set in the panel or one from the
// CA of the device.
func (s *Server) cloudClient(url string) (*cloudAPI, error) {
	pem, err := os.ReadFile(s.caFile())
	if err != nil {
		return nil, err
	}
	cfg := &tls.Config{RootCAs: x509.NewCertPool()}
	if c, _ := domaincert.Load(s.cfg.Root, time.Now()); c != nil {
		if cfg.RootCAs, err = x509.SystemCertPool(); err != nil {
			return nil, err
		}
		cfg.ServerName = c.Domain
	}
	cfg.RootCAs.AppendCertsFromPEM(pem)
	return &cloudAPI{url: url, client: &http.Client{
		Timeout:   30 * time.Second,
		Transport: &http.Transport{TLSClientConfig: cfg},
	}}, nil
}

// call sends body as JSON and decodes the answer into out.
func (c *cloudAPI) call(method, path string, body, out any) error {
	var in io.Reader
	if body != nil {
		data, err := json.Marshal(body)
		if err != nil {
			return err
		}
		in = bytes.NewReader(data)
	}
	req, err := http.NewRequest(method, c.url+path, in)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	if c.token != "" {
		req.Header.Set("Authorization", "Bearer "+c.token)
	}
	res, err := c.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, _ := io.ReadAll(io.LimitReader(res.Body, 1<<20))
	if res.StatusCode >= 300 {
		return fmt.Errorf("Supla Cloud: %s %s: %s %s", method, path, res.Status, strings.TrimSpace(string(data)))
	}
	if out != nil {
		return json.Unmarshal(data, out)
	}
	return nil
}

func (c *cloudAPI) login(email, password string) error {
	var res struct {
		AccessToken string `json:"access_token"`
	}
	err := c.call("POST", "/api/webapp-auth", map[string]string{"username": email, "password": password}, &res)
	if err != nil {
		if strings.Contains(err.Error(), "401") {
			return errors.New("wrong e-mail or password of the Supla Cloud account")
		}
		return err
	}
	c.token = res.AccessToken
	return nil
}

// oauthApp returns the public ID and the secret of the account's OAuth app
// with this redirect address, made when there is none.
func (c *cloudAPI) oauthApp(name, redirect string) (string, string, error) {
	type app struct {
		ID           int      `json:"id"`
		PublicID     string   `json:"publicId"`
		Secret       string   `json:"secret"`
		RedirectURIs []string `json:"redirectUris"`
	}
	var apps []app
	if err := c.call("GET", "/api/oauth-clients", nil, &apps); err != nil {
		return "", "", err
	}
	id := 0
	for _, a := range apps {
		if slices.Contains(a.RedirectURIs, redirect) {
			id = a.ID
			break
		}
	}
	if id == 0 {
		var made app
		body := map[string]any{"name": name, "description": "SuplaOS", "redirectUris": []string{redirect}}
		if err := c.call("POST", "/api/oauth-clients", body, &made); err != nil {
			return "", "", err
		}
		id = made.ID
	}
	var a app
	if err := c.call("GET", fmt.Sprintf("/api/oauth-clients/%d?include=secret", id), nil, &a); err != nil {
		return "", "", err
	}
	if a.PublicID == "" || a.Secret == "" {
		return "", "", errors.New("Supla Cloud did not give the ID and the secret of the OAuth app")
	}
	return a.PublicID, a.Secret, nil
}

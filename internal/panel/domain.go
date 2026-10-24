package panel

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/acme"
	"github.com/Goral64/SuplaOS/internal/domaincert"
	"github.com/Goral64/SuplaOS/internal/stack"
)

// domainView is the card of the certificate of the domain on the System
// page. Secret values are not sent back, only which ones are saved.
type domainView struct {
	Domain    string            `json:"domain"`
	Provider  string            `json:"provider"`
	Fields    map[string]string `json:"fields"`
	Secrets   []string          `json:"secrets"`
	Email     string            `json:"email"`
	Staging   bool              `json:"staging"`
	Status    *acme.Status      `json:"status"`
	Providers []acme.Provider   `json:"providers"`
	// Addresses could be the domain now that it has a certificate.
	Addresses []addressChange `json:"addresses"`
	// ScriptsOAuth: SUPLA Scripts logs in with the Cloud, which has to be
	// connected again after the address of either changes.
	ScriptsOAuth bool `json:"scriptsOAuth"`
}

// addressChange is an option of an add-on to set to the domain.
type addressChange struct {
	Addon string `json:"addon"`
	Key   string `json:"key"`
	From  string `json:"from"`
	To    string `json:"to"`
}

// addressChanges are the addresses of Supla Cloud and SUPLA Scripts that
// are not the domain yet, once there is a certificate for it.
func (s *Server) addressChanges(st *stack.Stack) []addressChange {
	c, _ := domaincert.Load(s.cfg.Root, time.Now())
	if c == nil {
		return []addressChange{}
	}
	out := []addressChange{}
	if st.IsInstalled("supla") {
		v := st.Values(st.Catalog["supla"])
		to := c.Domain
		if port := v["PORT_HTTPS"]; port != "" && port != "443" {
			to += ":" + port
		}
		if v["SUPLA_HOST_ADDRESS"] != to {
			out = append(out, addressChange{"supla", "SUPLA_HOST_ADDRESS", v["SUPLA_HOST_ADDRESS"], to})
		}
	}
	if st.IsInstalled("supla-scripts") {
		v := st.Values(st.Catalog["supla-scripts"])
		to := "https://" + c.Domain + ":" + v["SUPLA_SCRIPTS_PORT"]
		if strings.TrimSuffix(v["SUPLA_SCRIPTS_URL"], "/") != to {
			out = append(out, addressChange{"supla-scripts", "SUPLA_SCRIPTS_URL", v["SUPLA_SCRIPTS_URL"], to})
		}
	}
	return out
}

func (s *Server) getDomain(w http.ResponseWriter, r *http.Request) {
	set, err := s.acme.Settings()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	v := domainView{Fields: map[string]string{}, Secrets: []string{}, Providers: acme.Providers}
	if set != nil {
		v.Domain, v.Provider, v.Email, v.Staging = set.Domain, set.Provider, set.Email, set.Staging
		if p := acme.FindProvider(set.Provider); p != nil {
			for _, f := range p.Fields {
				switch value := set.Fields[f.Env]; {
				case value == "":
				case f.Secret:
					v.Secrets = append(v.Secrets, f.Env)
				default:
					v.Fields[f.Env] = value
				}
			}
		}
	}
	if v.Status, err = s.acme.Status(); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	v.Addresses = s.addressChanges(st)
	v.ScriptsOAuth = st.IsInstalled("supla-scripts") && st.Values(st.Catalog["supla-scripts"])["SUPLA_SCRIPTS_OAUTH_ID"] != ""
	writeJSON(w, http.StatusOK, v)
}

// useDomain sets the addresses of the add-ons to the domain.
func (s *Server) useDomain(w http.ResponseWriter, r *http.Request) {
	s.stackJob(w, "addresses at the domain", func(st *stack.Stack, j *Job) error {
		changes := s.addressChanges(st)
		if len(changes) == 0 {
			j.Logf("The addresses are the domain already.")
			return nil
		}
		for _, c := range changes {
			j.Logf("%s: %s %s -> %s", c.Addon, c.Key, c.From, c.To)
			if err := st.Configure(c.Addon, map[string]string{c.Key: c.To}); err != nil {
				return err
			}
		}
		return st.Up()
	})
}

// putDomain saves the settings and gets the certificate.
func (s *Server) putDomain(w http.ResponseWriter, r *http.Request) {
	var req acme.Settings
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	if err := s.acme.SaveSettings(req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	s.domainJob(w, "certificate for "+strings.ToLower(strings.TrimSpace(req.Domain)), true)
}

func (s *Server) renewDomainNow(w http.ResponseWriter, r *http.Request) {
	set, err := s.acme.Settings()
	if err != nil || set == nil {
		jsonError(w, http.StatusBadRequest, errors.Join(errors.New("no domain is set"), err))
		return
	}
	s.domainJob(w, "certificate for "+set.Domain, true)
}

func (s *Server) deleteDomain(w http.ResponseWriter, r *http.Request) {
	if err := s.acme.Disable(); err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	s.domainJob(w, "certificate of the device", false)
}

// domainJob gets the certificate of the domain, when obtain is set, and
// gives the add-ons the current one. Getting it may take minutes: the stack
// is locked only after that.
func (s *Server) domainJob(w http.ResponseWriter, title string, obtain bool) {
	j, err := s.jobs.start(title, func(j *Job) error {
		if obtain {
			if err := s.acme.Obtain(j); err != nil {
				j.Logf("Error: %v", err)
				return err
			}
		}
		st, err := s.openStack()
		if err != nil {
			return err
		}
		st.Stdout, st.Stderr, st.Stdin = j, j, strings.NewReader("")
		if err := st.Lock(); err != nil {
			j.Logf("Error: %v", err)
			return err
		}
		defer st.Unlock()
		s.loadDomainCertificate()
		if err := st.RenewCertificates(); err != nil {
			j.Logf("Error: %v", err)
			return err
		}
		j.Logf("Done.")
		return nil
	})
	writeJob(w, j, err)
}

// writeJob answers a request that started j.
func writeJob(w http.ResponseWriter, j *Job, err error) {
	if err != nil {
		var busy errBusy
		if errors.As(err, &busy) {
			writeJSON(w, http.StatusConflict, map[string]any{"error": err.Error(), "job": busy.job.snapshot()})
			return
		}
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusAccepted, j.snapshot())
}

package panel

import (
	"errors"
	"net/http"
	"strings"

	"github.com/Goral64/SuplaOS/internal/acme"
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
	writeJSON(w, http.StatusOK, v)
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


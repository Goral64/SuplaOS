package panel

import (
	"strings"
	"testing"

	"github.com/Goral64/SuplaOS/internal/stack"
)

func TestReportRedact(t *testing.T) {
	s := newServer(t)
	domainCertificate(s, "supla.example.com")
	st, err := s.openStack()
	if err != nil {
		t.Fatal(err)
	}
	st.State.Addons = map[string]*stack.Installed{"supla": {Options: map[string]string{"DATABASE_PASSWORD": "dbpass1"}}}
	if err := st.Save(); err != nil {
		t.Fatal(err)
	}
	in := "pass dbpass1, mail jan.kowalski@example.com, at https://supla.example.com:9443 and suplaos.example.com, " +
		"token MzQ1YjE2OGQ5ZTk0NzYxNmVlYTc0ZTAyNjM2ZWJlZjA3YjQ4, image mariadb:12.3.3"
	got := s.redact(in)
	for _, leak := range []string{"dbpass1", "kowalski", "example.com", "MzQ1YjE2"} {
		if strings.Contains(got, leak) {
			t.Errorf("%q left in %q", leak, got)
		}
	}
	if !strings.Contains(got, "mariadb:12.3.3") || !strings.Contains(got, "[domain]:9443") {
		t.Errorf("too much left out: %q", got)
	}

	// The whole report, without docker and the rest of a real system.
	if r := s.report(t.Context()); !strings.Contains(r, "SuplaOS ") || !strings.Contains(r, "  supla") {
		t.Errorf("report:\n%s", r)
	}
}

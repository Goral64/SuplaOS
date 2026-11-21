package panel

import (
	"strings"
	"testing"
)

func TestLimitPresets(t *testing.T) {
	for name, p := range limitPresets {
		if len(p) != len(accountLimits) {
			t.Errorf("%s: %d values, want %d", name, len(p), len(accountLimits))
		}
		for _, c := range accountLimits {
			if p[c.Col] <= 0 {
				t.Errorf("%s: no %s", name, c.Col)
			}
		}
	}
	if limitPresets["max"]["limit_iodev"] != 1000 || limitPresets["max"]["limit_actions_per_schedule"] != 40 ||
		limitPresets["big"]["limit_loc"] != 20 {
		t.Errorf("presets: %v", limitPresets)
	}
}

func TestParseLimits(t *testing.T) {
	cols := accountLimits[:2]
	q := limitsQuery(cols)
	if !strings.Contains(q, "u.limit_iodev, (select count(*) from supla_iodevice t where t.user_id = u.id)") {
		t.Errorf("query: %s", q)
	}
	out := "1\ta@example.com\t100\t3\t200\t0\n2\tb@example.com\t500\t0\t1000\t-1\n"
	got, err := parseLimits(out, cols)
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Email != "a@example.com" || *got[0].Limits[0].Used != 3 || got[0].Limits[1].Limit != 200 {
		t.Errorf("got %+v", got)
	}
	if got[1].Limits[1].Used != nil || got[1].Preset != "large" || got[0].Preset != "default" {
		t.Errorf("got %+v", got[1])
	}
	if _, err := parseLimits("1\ta\t5\n", cols); err == nil {
		t.Error("short row accepted")
	}
}

func TestSetLimitsSQL(t *testing.T) {
	q := setLimitsSQL(accountLimits[:2], limitPresets["large"], 7)
	if q != "update supla_user set limit_iodev = 500, limit_client = 1000 where id = 7" {
		t.Error(q)
	}
}

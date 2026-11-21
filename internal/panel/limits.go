package panel

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/stack"
)

// Limits of the Supla Cloud accounts (columns of supla_user). The Cloud
// changes them only with supla:user:change-limits in the console, here
// they are set to one of the presets with one click.

// accountLimit is a limit column, with the table whose rows of the user
// count towards it (empty when there is no simple count).
type accountLimit struct {
	Col, Table string
}

var accountLimits = []accountLimit{
	{"limit_iodev", "supla_iodevice"},
	{"limit_client", "supla_client"},
	{"limit_loc", "supla_location"},
	{"limit_aid", "supla_accessid"},
	{"limit_channel_group", "supla_dev_channel_group"},
	{"limit_channel_per_group", ""},
	{"limit_scene", "supla_scene"},
	{"limit_operations_per_scene", ""},
	{"limit_schedule", "supla_schedule"},
	{"limit_actions_per_schedule", ""},
	{"limit_direct_link", "supla_direct_link"},
	{"limit_value_based_triggers", "supla_value_based_trigger"},
	{"limit_oauth_client", "supla_oauth_clients"},
	{"limit_push_notifications", ""},
	{"limit_push_notifications_per_hour", ""},
	{"limit_virtual_channels", ""},
}

// defaultLimits are those of a new account of the Cloud
// (User::PREDEFINED_LIMITS["default"]).
var defaultLimits = map[string]int{
	"limit_iodev": 100, "limit_client": 200, "limit_loc": 10, "limit_aid": 10,
	"limit_channel_group": 20, "limit_channel_per_group": 10, "limit_scene": 50,
	"limit_operations_per_scene": 20, "limit_schedule": 20, "limit_actions_per_schedule": 20,
	"limit_direct_link": 50, "limit_value_based_triggers": 50, "limit_oauth_client": 20,
	"limit_push_notifications": 200, "limit_push_notifications_per_hour": 20, "limit_virtual_channels": 20,
}

// hardLimits are what the Cloud allows whatever the limit: a scene runs
// at most 640 operations (with the scenes it calls), and the config of a
// schedule has to be shorter than 2040 characters, about 35 actions.
var hardLimits = map[string]int{
	"limit_operations_per_scene": 640,
	"limit_actions_per_schedule": 40,
}

// presetNames in the order of the panel, with the multiple of the default
// limits; max is 10 times them or what the Cloud allows.
var presetNames = []string{"default", "big", "large", "max"}

var limitPresets = map[string]map[string]int{
	"default": scaledLimits(1),
	"big":     scaledLimits(2),
	"large":   scaledLimits(5),
	"max":     scaledLimits(10),
}

func scaledLimits(n int) map[string]int {
	limits := map[string]int{}
	for col, v := range defaultLimits {
		limits[col] = v * n
		if hard, ok := hardLimits[col]; ok {
			limits[col] = min(limits[col], hard)
		}
	}
	return limits
}

type limitValue struct {
	Name  string `json:"name"`
	Limit int    `json:"limit"`
	Used  *int   `json:"used,omitempty"`
}

type accountView struct {
	ID     int          `json:"id"`
	Email  string       `json:"email"`
	Preset string       `json:"preset"`
	Limits []limitValue `json:"limits"`
}

// cloudSQL runs a query in the database of the Cloud and returns its rows
// without the header, columns separated by tabs.
func cloudSQL(ctx context.Context, st *stack.Stack, q string) (string, error) {
	cmd := st.Command(ctx, "exec", "-T", "supla-db", "sh", "-c",
		`exec mariadb -N -B --user=supla --password="$MYSQL_PASSWORD" supla -e "$0"`, q)
	out, err := cmd.CombinedOutput()
	if err != nil {
		return "", fmt.Errorf("database not available: %w: %s", err, strings.TrimSpace(string(out)))
	}
	return string(out), nil
}

// limitColumns returns the limits that supla_user of this Cloud has (an
// older Cloud may not have the newest ones).
func limitColumns(ctx context.Context, st *stack.Stack) ([]accountLimit, error) {
	out, err := cloudSQL(ctx, st, "select column_name from information_schema.columns "+
		"where table_schema = database() and table_name = 'supla_user'")
	if err != nil {
		return nil, err
	}
	have := strings.Fields(out)
	var cols []accountLimit
	for _, l := range accountLimits {
		if slices.Contains(have, l.Col) {
			cols = append(cols, l)
		}
	}
	return cols, nil
}

// limitsQuery selects id, email, then for every column its value and the
// count of the rows of the user (or -1).
func limitsQuery(cols []accountLimit) string {
	sel := []string{"u.id", "u.email"}
	for _, c := range cols {
		used := "-1"
		if c.Table != "" {
			used = fmt.Sprintf("(select count(*) from %s t where t.user_id = u.id)", c.Table)
		}
		sel = append(sel, "u."+c.Col, used)
	}
	return "select " + strings.Join(sel, ", ") + " from supla_user u order by u.id"
}

func parseLimits(out string, cols []accountLimit) ([]accountView, error) {
	accounts := []accountView{}
	for _, line := range strings.Split(strings.TrimSpace(out), "\n") {
		if line == "" {
			continue
		}
		f := strings.Split(line, "\t")
		if len(f) != 2+2*len(cols) {
			return nil, fmt.Errorf("unexpected row: %q", line)
		}
		id, err := strconv.Atoi(f[0])
		if err != nil {
			return nil, err
		}
		a := accountView{ID: id, Email: f[1], Limits: []limitValue{}}
		for i, c := range cols {
			limit, err := strconv.Atoi(f[2+2*i])
			if err != nil {
				return nil, err
			}
			v := limitValue{Name: c.Col, Limit: limit}
			if used, err := strconv.Atoi(f[3+2*i]); err == nil && used >= 0 {
				v.Used = &used
			}
			a.Limits = append(a.Limits, v)
		}
		a.Preset = presetOf(a.Limits)
		accounts = append(accounts, a)
	}
	return accounts, nil
}

// presetOf names the preset the limits are set to, or "" for others.
func presetOf(limits []limitValue) string {
	for _, name := range presetNames {
		if !slices.ContainsFunc(limits, func(v limitValue) bool { return limitPresets[name][v.Name] != v.Limit }) {
			return name
		}
	}
	return ""
}

func setLimitsSQL(cols []accountLimit, preset map[string]int, userID int) string {
	var set []string
	for _, c := range cols {
		set = append(set, fmt.Sprintf("%s = %d", c.Col, preset[c.Col]))
	}
	return fmt.Sprintf("update supla_user set %s where id = %d", strings.Join(set, ", "), userID)
}

func (s *Server) suplaStack(w http.ResponseWriter) *stack.Stack {
	st, err := s.openStack()
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return nil
	}
	if !st.IsInstalled("supla") {
		jsonError(w, http.StatusNotFound, errors.New("supla is not installed"))
		return nil
	}
	return st
}

func (s *Server) getLimits(w http.ResponseWriter, r *http.Request) {
	st := s.suplaStack(w)
	if st == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cols, err := limitColumns(ctx, st)
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, err)
		return
	}
	out, err := cloudSQL(ctx, st, limitsQuery(cols))
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, err)
		return
	}
	accounts, err := parseLimits(out, cols)
	if err != nil {
		jsonError(w, http.StatusInternalServerError, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"accounts": accounts, "presets": limitPresets})
}

func (s *Server) putLimits(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.Atoi(r.PathValue("id"))
	if err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	var req struct{ Preset string }
	if err := readJSON(r, &req); err != nil {
		jsonError(w, http.StatusBadRequest, err)
		return
	}
	preset, ok := limitPresets[req.Preset]
	if !ok {
		jsonError(w, http.StatusBadRequest, fmt.Errorf("unknown preset %q", req.Preset))
		return
	}
	st := s.suplaStack(w)
	if st == nil {
		return
	}
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	cols, err := limitColumns(ctx, st)
	if err != nil {
		jsonError(w, http.StatusServiceUnavailable, err)
		return
	}
	if _, err := cloudSQL(ctx, st, setLimitsSQL(cols, preset, id)); err != nil {
		jsonError(w, http.StatusServiceUnavailable, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{})
}

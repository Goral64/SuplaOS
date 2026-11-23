package panel

import (
	"context"
	"fmt"
	"net/http"
	"os"
	"os/exec"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
	"github.com/Goral64/SuplaOS/internal/stack"
	"github.com/Goral64/SuplaOS/internal/system"
)

// getReport returns a text about this device to paste when asking for
// help: versions, hardware, add-ons and their containers, the last failed
// job. Passwords, tokens, e-mail addresses and the domain are left out.
func (s *Server) getReport(w http.ResponseWriter, r *http.Request) {
	ctx, cancel := context.WithTimeout(r.Context(), 15*time.Second)
	defer cancel()
	writeJSON(w, http.StatusOK, map[string]string{"text": s.report(ctx)})
}

func (s *Server) report(ctx context.Context) string {
	var b strings.Builder
	line := func(format string, args ...any) { fmt.Fprintf(&b, format+"\n", args...) }
	info := system.ReadInfo(s.cfg.Root)
	line("SuplaOS %s", info.Version)
	line("Hardware: %s (%s), %d CPUs, memory %s, free %s", hardware(), runtime.GOARCH, info.CPUs,
		size(info.MemTotal), size(info.MemFree))
	line("System: %s, kernel %s", osName(), readTrim("/proc/sys/kernel/osrelease"))
	up := time.Duration(info.Uptime) * time.Second
	temp := ""
	if info.Temperature != nil {
		temp = fmt.Sprintf(", temperature %.0f °C", *info.Temperature)
	}
	line("Up %dd %dh, load %s%s", int(up.Hours())/24, int(up.Hours())%24, strings.Join(info.Load, " "), temp)
	line("Disk: %s, free %s", size(info.DiskTotal), size(info.DiskFree))
	if info.RebootRequired {
		line("A newer kernel waits for a restart.")
	}
	if u, err := system.ReadUpdates(); err == nil && u.State != "" {
		when := ""
		if u.Finished != nil {
			when = " " + u.Finished.Local().Format("2006-01-02 15:04")
		}
		line("Last update of the system: %s%s", u.State, when)
	}
	if out, err := exec.CommandContext(ctx, "systemctl", "--failed", "--no-legend", "--plain").Output(); err == nil {
		var units []string
		for _, l := range strings.Split(strings.TrimSpace(string(out)), "\n") {
			if f := strings.Fields(l); len(f) > 0 {
				units = append(units, f[0])
			}
		}
		if len(units) > 0 {
			line("Failed services: %s", strings.Join(units, ", "))
		}
	}
	if st, err := s.acme.Status(); err == nil && st != nil {
		switch {
		case st.LastError != "":
			line("Own domain: set, getting the certificate failed: %s", st.LastError)
		case st.NotAfter != nil:
			line("Own domain: set, certificate valid until %s", st.NotAfter.Local().Format("2006-01-02"))
		default:
			line("Own domain: set, no certificate yet")
		}
	}

	stk, err := s.openStack()
	if err != nil {
		line("Add-ons: %v", err)
	} else {
		containers, err := stk.Containers(ctx)
		if err != nil {
			line("Containers: %v", err)
		}
		line("")
		line("Add-ons:")
		for _, a := range stk.Order() {
			inst := stk.State.Addons[a.ID]
			head := "  " + a.ID
			if inst.Package != nil && inst.Package.Tag != "" {
				head += " (supla-docker " + inst.Package.Tag + ")"
			}
			if inst.OwnEnv != "" || inst.OwnCompose != "" {
				head += ", own variables or compose"
			}
			line("%s", head)
			images, _ := s.addonImages(a.Services)
			for _, svc := range a.Services {
				i := slices.IndexFunc(containers, func(c stack.Container) bool { return c.Service == svc })
				if i < 0 {
					line("    %s: no container", svc)
					continue
				}
				c := containers[i]
				state := c.State
				if c.Health != "" {
					state += ", " + c.Health
				}
				image := shortDigest(c.Image)
				if v := images[svc].Version; v != "" {
					image += ", version " + v
				}
				line("    %s: %s (%s) %s", svc, state, c.Status, image)
			}
		}
		if len(stk.State.Addons) == 0 {
			line("  none")
		}
	}

	s.jobs.mu.Lock()
	jobs := slices.Clone(s.jobs.list)
	s.jobs.mu.Unlock()
	for i := len(jobs) - 1; i >= 0; i-- {
		j := jobs[i]
		j.mu.Lock()
		failed, lines := j.State == "failed", slices.Clone(j.lines)
		j.mu.Unlock()
		if !failed {
			continue
		}
		line("")
		line("Last failed operation (%s): %s", j.Started.Local().Format("2006-01-02 15:04"), j.Title)
		line("Error: %s", j.Error)
		if len(lines) > 40 {
			lines = lines[len(lines)-40:]
		}
		for _, l := range lines {
			line("  %s", l)
		}
		break
	}
	if u, err := system.ReadUpdates(); err == nil && u.State == "failed" {
		if text, _, err := system.UpgradeLog(0); err == nil {
			lines := strings.Split(strings.TrimSpace(stripANSI(text)), "\n")
			if len(lines) > 30 {
				lines = lines[len(lines)-30:]
			}
			line("")
			line("Log of the failed update of the system:")
			for _, l := range lines {
				line("  %s", l)
			}
		}
	}
	return s.redact(b.String())
}

var (
	emailRe = regexp.MustCompile(`[A-Za-z0-9._%+-]+@[A-Za-z0-9.-]+\.[A-Za-z]{2,}`)
	// Tokens, keys and GUIDs: long runs of letters and digits.
	tokenRe = regexp.MustCompile(`\b[A-Za-z0-9_-]{32,}\b`)
)

// redact leaves out of the report what is secret or personal: values of
// passwords and generated secrets of the add-ons, the keys of the DNS
// provider, the tokens of a move, e-mail addresses, the domain and its
// names, and anything that looks like a token.
func (s *Server) redact(text string) string {
	var secrets, names []string
	if stk, err := s.openStack(); err == nil {
		for _, a := range stk.Order() {
			values := stk.Values(a)
			for _, o := range a.Options {
				if o.Type == addon.TypePassword || o.Type == addon.TypeSecret {
					secrets = append(secrets, values[o.Key])
				}
			}
		}
	}
	if set, err := s.acme.Settings(); err == nil && set != nil {
		for _, v := range set.Fields {
			secrets = append(secrets, v)
		}
		names = append(names, set.Domain)
		ifs, _ := s.webInterfaces()
		for _, i := range ifs {
			names = append(names, set.Name(i))
		}
	}
	if state, err := s.migrationStore().State(); err == nil && state != nil {
		secrets = append(secrets, state.Token, state.SourceToken)
	}
	// The longest first: a value may contain another.
	sort.Slice(secrets, func(i, j int) bool { return len(secrets[i]) > len(secrets[j]) })
	for _, v := range secrets {
		if len(v) >= 4 {
			text = strings.ReplaceAll(text, v, "[hidden]")
		}
	}
	sort.Slice(names, func(i, j int) bool { return len(names[i]) > len(names[j]) })
	for _, n := range names {
		if n != "" {
			text = strings.ReplaceAll(text, n, "[domain]")
		}
	}
	text = emailRe.ReplaceAllString(text, "[e-mail]")
	return tokenRe.ReplaceAllString(text, "[hidden]")
}

// shortDigest shortens the digest of a pinned image: enough to tell
// which one it is, too short to be taken for a token.
func shortDigest(image string) string {
	name, digest, ok := strings.Cut(image, "@sha256:")
	if ok && len(digest) > 12 {
		return name + "@sha256:" + digest[:12]
	}
	return image
}

func hardware() string {
	if m := strings.TrimRight(readTrim("/proc/device-tree/model"), "\x00"); m != "" {
		return m
	}
	vendor, product := readTrim("/sys/class/dmi/id/sys_vendor"), readTrim("/sys/class/dmi/id/product_name")
	if h := strings.TrimSpace(vendor + " " + product); h != "" {
		return h
	}
	return "unknown"
}

func osName() string {
	data, _ := os.ReadFile("/etc/os-release")
	for _, l := range strings.Split(string(data), "\n") {
		if v, ok := strings.CutPrefix(l, "PRETTY_NAME="); ok {
			return strings.Trim(v, `"`)
		}
	}
	return "unknown"
}

func readTrim(path string) string {
	data, _ := os.ReadFile(path)
	return strings.TrimSpace(string(data))
}

func size(n uint64) string {
	const gib = 1 << 30
	if n >= gib {
		return fmt.Sprintf("%.1f GiB", float64(n)/gib)
	}
	return fmt.Sprintf("%d MiB", n>>20)
}

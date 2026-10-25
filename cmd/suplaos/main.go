// Command suplaos manages the add-ons of SuplaOS: one docker compose project
// built from the add-on catalog.
package main

import (
	"errors"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/Goral64/SuplaOS/internal/addon"
	"github.com/Goral64/SuplaOS/internal/fan"
	"github.com/Goral64/SuplaOS/internal/migrate"
	"github.com/Goral64/SuplaOS/internal/stack"
	"github.com/Goral64/SuplaOS/internal/system"
)

const usage = `Usage: suplaos <command> [arguments]

Add-ons:
  list                         available add-ons
  info <add-on>                description and settings
  install <add-on> [KEY=VALUE...] [--no-start]
  remove <add-on> [--purge]    --purge also deletes its data
  purge <add-on>               delete data of a removed add-on
  config <add-on> [KEY=VALUE...] [--show]
                               show or change settings (KEY= restores the default,
                               --show shows passwords)

Containers:
  status                       containers of the installed add-ons
  start|stop|restart [add-on...]
  logs [add-on...] [-f] [--tail N]
  update [add-on...]           backup, pull new images and recreate
  backup [add-on...]           dump add-on data to /srv/suplaos/backups
  restore <add-on> <file>      load a backup (a name from backups/ or a path),
                               the current data is backed up first
  run <add-on> <command> [args...]
                               add-on command, e.g. suplaos run supla console
  apply                        apply new versions of the add-ons (after backups)
  compose [args...]            docker compose on the SuplaOS project

System:
  upgrade                      update the system and SuplaOS (as root)
  export [-o file]             full backup of the device: add-ons with their data
                               and the CA, to backups/ by default (as root);
                               encrypted with SUPLAOS_BACKUP_PASSWORD when set
  migrate fetch|prepare|status move an account of another Supla Cloud here (the
                               token of the source in SUPLAOS_SOURCE_TOKEN)
  import <file> [--yes]        restore a full backup: replaces all add-ons and
                               their data (the current state is backed up
                               first, --yes when add-ons are installed)

Environment: SUPLAOS_ROOT (default /srv/suplaos), SUPLAOS_ADDONS
(default /usr/share/suplaos/addons).
`

func main() {
	if err := run(os.Args[1:]); err != nil {
		fmt.Fprintln(os.Stderr, "suplaos:", err)
		os.Exit(1)
	}
}

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func run(args []string) error {
	if len(args) == 0 || args[0] == "-h" || args[0] == "--help" || args[0] == "help" {
		fmt.Print(usage)
		return nil
	}
	// Run by the shutdown hook of the Argon ONE case, when the data
	// directory may be gone already.
	if args[0] == "argon-poweroff" {
		return fan.ArgonPowerOff()
	}
	s, err := stack.Open(getenv("SUPLAOS_ROOT", "/srv/suplaos"), getenv("SUPLAOS_ADDONS", "/usr/share/suplaos/addons"))
	if err != nil {
		return err
	}
	cmd, args := args[0], args[1:]
	if cmd != "run" && cmd != "compose" {
		for _, a := range args {
			if a == "-h" || a == "--help" {
				fmt.Print(usage)
				return nil
			}
		}
	}
	if (cmd == "list" || cmd == "status" || cmd == "apply" || cmd == "upgrade") && len(args) > 0 {
		return fmt.Errorf("%s takes no arguments", cmd)
	}
	switch cmd {
	case "list":
		return list(s)
	case "info":
		if len(args) != 1 {
			return errors.New("usage: suplaos info <add-on>")
		}
		return info(s, args[0], false)
	case "install":
		return install(s, args)
	case "remove":
		return remove(s, args)
	case "config":
		return config(s, args)
	case "purge":
		if len(args) != 1 {
			return errors.New("usage: suplaos purge <add-on>")
		}
		if err := s.Lock(); err != nil {
			return err
		}
		return s.Purge(args[0])
	case "status":
		return s.Compose("ps")
	case "start", "stop", "restart":
		services, err := s.Services(args...)
		if err != nil {
			return err
		}
		if cmd == "start" {
			return s.Compose(append([]string{"up", "-d"}, services...)...)
		}
		return s.Compose(append([]string{cmd}, services...)...)
	case "logs":
		return logs(s, args)
	case "update":
		return update(s, args)
	case "export":
		return export(s, args)
	case "import":
		return importBackup(s, args)
	case "migrate":
		return migrateCmd(s, args)
	case "backup":
		if err := s.Lock(); err != nil {
			return err
		}
		return backup(s, args, len(args) == 0)
	case "restore":
		if len(args) != 2 {
			return errors.New("usage: suplaos restore <add-on> <file>")
		}
		if err := s.Lock(); err != nil {
			return err
		}
		path := args[1]
		if !strings.Contains(path, "/") {
			path = filepath.Join(s.BackupDir(), path)
		}
		if err := s.Restore(args[0], path); err != nil {
			return err
		}
		fmt.Printf("%s: restored from %s\n", args[0], filepath.Base(path))
		return nil
	case "run":
		if len(args) < 2 {
			return errors.New("usage: suplaos run <add-on> <command> [args...]")
		}
		return s.Run(args[0], args[1], args[2:])
	case "apply":
		if err := s.Lock(); err != nil {
			return err
		}
		pending, err := s.Pending()
		if err != nil {
			return err
		}
		if err := backup(s, pending, true); err != nil {
			return err
		}
		if err := s.Refetch(); err != nil {
			return err
		}
		if err := s.Apply(); err != nil {
			return err
		}
		return s.Up()
	case "upgrade":
		if os.Geteuid() != 0 {
			return errors.New("run it as root: sudo suplaos upgrade")
		}
		if err := system.RunUpgrade(); err != nil {
			return err
		}
		if pending, _ := s.Pending(); len(pending) > 0 {
			fmt.Printf("New versions of add-ons: %s. Apply them with: suplaos apply\n", strings.Join(pending, ", "))
		}
		return nil
	case "compose":
		return s.Compose(args...)
	}
	return fmt.Errorf("unknown command %q, see suplaos help", cmd)
}

func lang() string {
	for _, k := range []string{"LC_ALL", "LC_MESSAGES", "LANG"} {
		if v := os.Getenv(k); v != "" {
			if strings.HasPrefix(v, "pl") {
				return "pl"
			}
			return "en"
		}
	}
	return "en"
}

func list(s *stack.Stack) error {
	pending := map[string]bool{}
	if p, err := s.Pending(); err == nil {
		for _, id := range p {
			pending[id] = true
		}
	}
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintln(w, "ADD-ON\tSTATE\tDESCRIPTION")
	for _, id := range s.Catalog.IDs() {
		state := "-"
		if pending[id] {
			state = "new version (suplaos apply)"
		} else if s.IsInstalled(id) {
			state = "installed"
		}
		fmt.Fprintf(w, "%s\t%s\t%s\n", id, state, s.Catalog[id].Description.In(lang()))
	}
	return w.Flush()
}

// info prints an add-on; passwords only with show.
func info(s *stack.Stack, id string, show bool) error {
	a := s.Catalog[id]
	if a == nil {
		return fmt.Errorf("unknown add-on %q", id)
	}
	l := lang()
	fmt.Printf("%s - %s\n", a.Name.In(l), a.Description.In(l))
	if len(a.Requires) > 0 {
		fmt.Printf("Requires: %s\n", strings.Join(a.Requires, ", "))
	}
	if len(a.Recommends) > 0 {
		fmt.Printf("Works with: %s\n", strings.Join(a.Recommends, ", "))
	}
	installed := s.IsInstalled(id)
	if installed {
		fmt.Println("Installed.")
	}
	if len(a.Commands) > 0 {
		var names []string
		for n := range a.Commands {
			names = append(names, n)
		}
		sort.Strings(names)
		fmt.Println("\nCommands (suplaos run " + id + " <command>):")
		for _, n := range names {
			fmt.Printf("  %-12s %s\n", n, a.Commands[n].Help.In(l))
		}
	}
	if len(a.Options) == 0 {
		return nil
	}
	values := s.Values(a)
	fmt.Println("\nSettings:")
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	for i := range a.Options {
		o := &a.Options[i]
		if o.Type == addon.TypeSecret {
			continue
		}
		active := s.Active(o, values)
		if installed && !active {
			continue
		}
		v := values[o.Key]
		if o.Type == addon.TypePassword && v != "" && !show {
			v = "********"
		}
		if !installed {
			v = o.Default
		}
		var notes []string
		if o.Required {
			notes = append(notes, "required")
		}
		if len(o.Choices) > 0 {
			notes = append(notes, strings.Join(o.Choices, "|"))
		}
		if o.When != nil && !installed {
			notes = append(notes, "only "+describe(o.When))
		}
		label := o.Label.In(l)
		if len(notes) > 0 {
			label += " (" + strings.Join(notes, ", ") + ")"
		}
		fmt.Fprintf(w, "  %s=%s\t%s\n", o.Key, v, label)
		if h := o.Help.In(l); h != "" {
			fmt.Fprintf(w, "  \t%s\n", h)
		}
	}
	return w.Flush()
}

func describe(c *addon.Condition) string {
	var parts []string
	if c.Installed != "" {
		parts = append(parts, "with "+c.Installed)
	}
	if c.NotInstalled != "" {
		parts = append(parts, "without "+c.NotInstalled)
	}
	if c.Option != "" {
		parts = append(parts, c.Option+"="+c.Equals)
	}
	return strings.Join(parts, " and ")
}

// parseSettings splits KEY=VALUE arguments from flags.
func parseSettings(args []string) (map[string]string, []string, error) {
	set := map[string]string{}
	var rest []string
	for _, a := range args {
		if strings.HasPrefix(a, "-") {
			rest = append(rest, a)
			continue
		}
		k, v, ok := strings.Cut(a, "=")
		if !ok {
			return nil, nil, fmt.Errorf("expected KEY=VALUE, got %q", a)
		}
		set[k] = v
	}
	return set, rest, nil
}

func install(s *stack.Stack, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: suplaos install <add-on> [KEY=VALUE...] [--no-start]")
	}
	id := args[0]
	set, rest, err := parseSettings(args[1:])
	if err != nil {
		return err
	}
	fs := flag.NewFlagSet("install", flag.ContinueOnError)
	noStart := fs.Bool("no-start", false, "do not start the containers")
	if err := fs.Parse(rest); err != nil {
		return err
	}
	if err := s.Lock(); err != nil {
		return err
	}
	if err := s.Install(id, set); err != nil {
		return err
	}
	fmt.Printf("%s installed.\n", id)
	for _, r := range s.Catalog[id].Recommends {
		if !s.IsInstalled(r) {
			fmt.Printf("Works with %s: suplaos install %s\n", r, r)
		}
	}
	if *noStart {
		return nil
	}
	return s.Up()
}

func remove(s *stack.Stack, args []string) error {
	fs := flag.NewFlagSet("remove", flag.ContinueOnError)
	purge := fs.Bool("purge", false, "delete the add-on data")
	var ids []string
	for _, a := range args {
		if !strings.HasPrefix(a, "-") {
			ids = append(ids, a)
		}
	}
	if err := fs.Parse(stripArgs(args, ids)); err != nil {
		return err
	}
	if len(ids) != 1 {
		return errors.New("usage: suplaos remove <add-on> [--purge]")
	}
	id := ids[0]
	if err := s.Lock(); err != nil {
		return err
	}
	if d := s.Dependents(id); len(d) > 0 {
		return fmt.Errorf("%s is required by %s, remove it first", id, strings.Join(d, ", "))
	}
	services, err := s.Services(id)
	if err != nil {
		return err
	}
	if err := s.Compose(append([]string{"rm", "--stop", "--force"}, services...)...); err != nil {
		return err
	}
	if err := s.Remove(id); err != nil {
		return err
	}
	if len(s.State.Addons) > 0 {
		// Other add-ons may have used files of this one.
		if err := s.Up(); err != nil {
			return err
		}
	} else {
		s.RemoveNetwork()
	}
	if *purge {
		if err := s.Purge(id); err != nil {
			return err
		}
	}
	fmt.Printf("%s removed.\n", id)
	return nil
}

func stripArgs(args, drop []string) []string {
	var out []string
	for _, a := range args {
		keep := true
		for _, d := range drop {
			if a == d {
				keep = false
			}
		}
		if keep {
			out = append(out, a)
		}
	}
	return out
}

func config(s *stack.Stack, args []string) error {
	if len(args) == 0 {
		return errors.New("usage: suplaos config <add-on> [KEY=VALUE...]")
	}
	id := args[0]
	if !s.IsInstalled(id) {
		return fmt.Errorf("%s is not installed", id)
	}
	set, rest, err := parseSettings(args[1:])
	if err != nil {
		return err
	}
	show := false
	for _, f := range rest {
		if f != "--show" {
			return fmt.Errorf("unknown flag %s", f)
		}
		show = true
	}
	if len(set) == 0 {
		return info(s, id, show)
	}
	if err := s.Lock(); err != nil {
		return err
	}
	if err := s.Configure(id, set); err != nil {
		return err
	}
	return s.Up()
}

func logs(s *stack.Stack, args []string) error {
	fs := flag.NewFlagSet("logs", flag.ContinueOnError)
	follow := fs.Bool("f", false, "follow")
	tail := fs.String("tail", "100", "lines from the end")
	var ids []string
	var flags []string
	for i := 0; i < len(args); i++ {
		a := args[i]
		switch {
		case a == "--tail" && i+1 < len(args):
			flags = append(flags, a, args[i+1])
			i++
		case strings.HasPrefix(a, "-"):
			flags = append(flags, a)
		default:
			ids = append(ids, a)
		}
	}
	if err := fs.Parse(flags); err != nil {
		return err
	}
	services, err := s.Services(ids...)
	if err != nil {
		return err
	}
	cargs := []string{"logs", "--tail", *tail}
	if *follow {
		cargs = append(cargs, "-f")
	}
	return s.Compose(append(cargs, services...)...)
}

// backup dumps the given add-ons, all installed ones when ids is empty.
// With skip, add-ons without a backup definition are skipped silently.
func backup(s *stack.Stack, ids []string, skip bool) error {
	if len(ids) == 0 {
		for _, a := range s.Order() {
			ids = append(ids, a.ID)
		}
	}
	for _, id := range ids {
		a := s.Catalog[id]
		if a != nil && a.Backup == nil && skip {
			continue
		}
		path, err := s.Backup(id)
		if err != nil {
			return err
		}
		fmt.Printf("%s: backup saved to %s\n", id, path)
	}
	return nil
}

// export writes a full backup of the device.
func export(s *stack.Stack, args []string) error {
	fs := flag.NewFlagSet("export", flag.ContinueOnError)
	out := fs.String("o", "", "output file")
	if err := fs.Parse(args); err != nil {
		return err
	}
	if fs.NArg() > 0 {
		return errors.New("usage: suplaos export [-o file]")
	}
	if err := s.Lock(); err != nil {
		return err
	}
	opt := stack.ExportOptions{Version: system.Version(), Password: os.Getenv("SUPLAOS_BACKUP_PASSWORD")}
	if opt.Password == "" {
		fmt.Fprintln(os.Stderr, "Warning: SUPLAOS_BACKUP_PASSWORD is not set, the backup is not encrypted. It holds the passwords of the add-ons and the key of the CA.")
	}
	path := *out
	var err error
	if path == "" {
		path, err = s.SaveFullBackup(opt)
	} else {
		err = s.ExportFile(path, opt)
	}
	if err != nil {
		return err
	}
	fmt.Printf("Full backup saved to %s\n", path)
	return nil
}

// migrateCmd reads an account of another Supla Cloud and puts its devices
// and channels here, with their IDs.
func migrateCmd(s *stack.Stack, args []string) error {
	store := migrate.Store{Dir: filepath.Join(s.Root, "migration")}
	if len(args) != 1 {
		return errors.New("usage: suplaos migrate fetch|prepare|status")
	}
	switch args[0] {
	case "fetch":
		src, err := migrate.NewSource(os.Getenv("SUPLAOS_SOURCE_TOKEN"))
		if err != nil {
			return err
		}
		b, err := src.Fetch()
		if err != nil {
			return err
		}
		if err := store.SaveBundle(b); err != nil {
			return err
		}
		fmt.Printf("%s, %s: %d devices, %d channels, %d locations, %d access IDs\n", b.Server, b.Email,
			len(b.Devices), b.ChannelCount(), len(b.Locations), len(b.AccessIDs))
		return nil
	case "prepare":
		if err := s.Lock(); err != nil {
			return err
		}
		b, err := store.Bundle()
		if err != nil || b == nil {
			return fmt.Errorf("fetch the source first: %v", err)
		}
		st, err := store.State()
		if err != nil {
			return err
		}
		if !st.Prepared.IsZero() {
			return errors.New("already prepared")
		}
		if err := migrate.Prepare(migrate.StackDB{Stack: s}, b, os.Stdout); err != nil {
			return err
		}
		st.Prepared = time.Now().UTC()
		return store.SaveState(st)
	case "status":
		b, err := store.Bundle()
		if err != nil || b == nil {
			return fmt.Errorf("no move: %v", err)
		}
		connected, err := migrate.Connected(migrate.StackDB{Stack: s}, b)
		if err != nil {
			return err
		}
		fmt.Printf("%d of %d devices connected to this server\n", len(connected), len(b.Devices))
		return nil
	}
	return errors.New("usage: suplaos migrate fetch|prepare|status")
}

// importBackup restores a full backup.
func importBackup(s *stack.Stack, args []string) error {
	var file string
	yes := false
	for _, a := range args {
		switch {
		case a == "--yes":
			yes = true
		case file == "" && !strings.HasPrefix(a, "-"):
			file = a
		default:
			return errors.New("usage: suplaos import <file> [--yes]")
		}
	}
	if file == "" {
		return errors.New("usage: suplaos import <file> [--yes]")
	}
	if len(s.State.Addons) > 0 && !yes {
		return errors.New("this replaces all installed add-ons and their data, run again with --yes")
	}
	if err := s.Lock(); err != nil {
		return err
	}
	opt := stack.ImportOptions{Version: system.Version(), Password: os.Getenv("SUPLAOS_BACKUP_PASSWORD")}
	if err := s.Import(file, opt); err != nil {
		return err
	}
	fmt.Println("Full backup restored")
	return nil
}

func update(s *stack.Stack, ids []string) error {
	if err := s.Lock(); err != nil {
		return err
	}
	services, err := s.Services(ids...)
	if err != nil {
		return err
	}
	if err := backup(s, ids, true); err != nil {
		return err
	}
	if err := s.Compose(append([]string{"pull"}, services...)...); err != nil {
		return err
	}
	// Only these services: up on the whole project would also recreate the
	// ones depending on them (zigbee2supla on supla-server).
	return s.Compose(append([]string{"up", "-d"}, services...)...)
}

// Package addon reads the add-on catalog: one directory per add-on with
// addon.yaml and the compose files it brings.
package addon

import (
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"

	"gopkg.in/yaml.v3"
)

// Text is a translatable string: a plain string or a map of language codes,
// e.g. {en: ..., pl: ...}.
type Text map[string]string

func (t *Text) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		*t = Text{"en": n.Value}
		return nil
	}
	m := map[string]string{}
	if err := n.Decode(&m); err != nil {
		return err
	}
	*t = m
	return nil
}

// In returns the text in the given language, falling back to English.
func (t Text) In(lang string) string {
	if s, ok := t[lang]; ok {
		return s
	}
	return t["en"]
}

// Condition limits a compose file or an option to some setups. All set
// fields must match.
type Condition struct {
	Installed    string `yaml:"installed"`
	NotInstalled string `yaml:"not_installed"`
	Option       string `yaml:"option"`
	Equals       string `yaml:"equals"`
}

// Match tells whether the condition holds for the installed add-ons and the
// option values of the add-on it belongs to. A nil condition always holds.
func (c *Condition) Match(installed func(id string) bool, options map[string]string) bool {
	if c == nil {
		return true
	}
	if c.Installed != "" && !installed(c.Installed) {
		return false
	}
	if c.NotInstalled != "" && installed(c.NotInstalled) {
		return false
	}
	if c.Option != "" && options[c.Option] != c.Equals {
		return false
	}
	return true
}

// ComposeFile is a compose file of the add-on, optionally conditional.
// In addon.yaml it is a file name or {file: ..., when: {...}}.
type ComposeFile struct {
	File string     `yaml:"file"`
	When *Condition `yaml:"when"`
}

func (c *ComposeFile) UnmarshalYAML(n *yaml.Node) error {
	if n.Kind == yaml.ScalarNode {
		c.File = n.Value
		return nil
	}
	type plain ComposeFile
	return n.Decode((*plain)(c))
}

// Option is a setting the user fills in. Its value goes to the .env of the
// compose project (compose variables) or to the env file named in File.
type Option struct {
	Key      string     `yaml:"key"`
	Label    Text       `yaml:"label"`
	Help     Text       `yaml:"help"`
	Type     string     `yaml:"type"`
	Default  string     `yaml:"default"`
	Choices  []string   `yaml:"choices"`
	Required bool       `yaml:"required"`
	Generate int        `yaml:"generate"`
	File     string     `yaml:"file"`
	When     *Condition `yaml:"when"`
}

// Option types.
const (
	TypeString   = "string"
	TypeEmail    = "email"
	TypePassword = "password"
	TypeSecret   = "secret" // generated on install, not shown
	TypePort     = "port"
	TypeSelect   = "select"
	TypeSerial   = "serial" // serial device, e.g. /dev/serial/by-id/...
)

// Fetch downloads files of an add-on from an upstream archive (tar.gz) on
// install instead of keeping them in the catalog.
type Fetch struct {
	URL   string   `yaml:"url"`
	Files []string `yaml:"files"`
}

// Backup describes how to dump the add-on data: Command runs in Service and
// its output is saved, gzipped, as File ({time} is replaced by a timestamp).
type Backup struct {
	Service string   `yaml:"service"`
	Command []string `yaml:"command"`
	File    string   `yaml:"file"`
}

// Command is an add-on specific command run in one of its containers.
type Command struct {
	Help    Text     `yaml:"help"`
	Service string   `yaml:"service"`
	User    string   `yaml:"user"`
	Exec    []string `yaml:"exec"`
}

// Web is the add-on's own web interface, linked from the panel.
type Web struct {
	Scheme string `yaml:"scheme"`
	Port   string `yaml:"port"`
	Path   string `yaml:"path"`
}

type Addon struct {
	ID          string             `yaml:"id"`
	Name        Text               `yaml:"name"`
	Description Text               `yaml:"description"`
	Requires    []string           `yaml:"requires"`
	Recommends  []string           `yaml:"recommends"`
	Fetch       *Fetch             `yaml:"fetch"`
	Files       []string           `yaml:"files"`
	EnvBase     string             `yaml:"env_base"`
	EnvFile     string             `yaml:"env_file"`
	Compose     []ComposeFile      `yaml:"compose"`
	Services    []string           `yaml:"services"`
	Env         yaml.Node          `yaml:"env"`
	Options     []Option           `yaml:"options"`
	Ports       []string           `yaml:"ports"`
	Dirs        []string           `yaml:"dirs"`
	Data        []string           `yaml:"data"`
	Web         *Web               `yaml:"web"`
	Backup      *Backup            `yaml:"backup"`
	Commands    map[string]Command `yaml:"commands"`

	// Dir is the catalog directory of the add-on.
	Dir string `yaml:"-"`
	// FixedEnv is Env in file order.
	FixedEnv []KV `yaml:"-"`
}

// KV is one variable of an env file.
type KV struct {
	Key, Value string
}

// Option returns the option with the given key or nil.
func (a *Addon) Option(key string) *Option {
	for i := range a.Options {
		if a.Options[i].Key == key {
			return &a.Options[i]
		}
	}
	return nil
}

var (
	idRe  = regexp.MustCompile(`^[a-z0-9][a-z0-9-]*$`)
	keyRe = regexp.MustCompile(`^[A-Za-z_][A-Za-z0-9_]*$`)
)

// Parse reads addon.yaml.
func Parse(data []byte) (*Addon, error) {
	a := &Addon{}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(a); err != nil {
		return nil, err
	}
	if a.Env.Kind != 0 {
		if a.Env.Kind != yaml.MappingNode {
			return nil, fmt.Errorf("env: expected a mapping")
		}
		for i := 0; i+1 < len(a.Env.Content); i += 2 {
			a.FixedEnv = append(a.FixedEnv, KV{a.Env.Content[i].Value, a.Env.Content[i+1].Value})
		}
	}
	return a, a.validate()
}

func (a *Addon) validate() error {
	if !idRe.MatchString(a.ID) {
		return fmt.Errorf("invalid id %q", a.ID)
	}
	if len(a.Compose) == 0 {
		return fmt.Errorf("%s: no compose files", a.ID)
	}
	if len(a.Services) == 0 {
		return fmt.Errorf("%s: no services", a.ID)
	}
	seen := map[string]bool{}
	for _, kv := range a.FixedEnv {
		if !keyRe.MatchString(kv.Key) {
			return fmt.Errorf("%s: invalid env key %q", a.ID, kv.Key)
		}
	}
	for _, o := range a.Options {
		if !keyRe.MatchString(o.Key) {
			return fmt.Errorf("%s: invalid option key %q", a.ID, o.Key)
		}
		if seen[o.Key] {
			return fmt.Errorf("%s: duplicate option %s", a.ID, o.Key)
		}
		seen[o.Key] = true
		switch o.Type {
		case "", TypeString, TypeEmail, TypePassword, TypePort, TypeSerial:
		case TypeSelect:
			if len(o.Choices) == 0 {
				return fmt.Errorf("%s: option %s has no choices", a.ID, o.Key)
			}
		case TypeSecret:
			if o.Generate <= 0 {
				return fmt.Errorf("%s: secret %s needs generate", a.ID, o.Key)
			}
		default:
			return fmt.Errorf("%s: option %s has unknown type %q", a.ID, o.Key, o.Type)
		}
		if o.File != "" && o.File != filepath.Base(o.File) {
			return fmt.Errorf("%s: option %s: file must be a plain name", a.ID, o.Key)
		}
	}
	for _, list := range [][]string{a.Files, a.Dirs, a.Data} {
		for _, p := range list {
			if !local(p) {
				return fmt.Errorf("%s: path %q leaves the stack directory", a.ID, p)
			}
		}
	}
	if a.EnvFile != "" && a.EnvFile != filepath.Base(a.EnvFile) {
		return fmt.Errorf("%s: env_file must be a plain name", a.ID)
	}
	for _, c := range a.Compose {
		if c.File != filepath.Base(c.File) {
			return fmt.Errorf("%s: compose file must be a plain name: %q", a.ID, c.File)
		}
	}
	if a.Backup != nil && (a.Backup.Service == "" || len(a.Backup.Command) == 0 || a.Backup.File == "") {
		return fmt.Errorf("%s: backup needs service, command and file", a.ID)
	}
	return nil
}

func local(p string) bool {
	return p != "" && filepath.IsLocal(p)
}

// Catalog is the set of available add-ons.
type Catalog map[string]*Addon

// Load reads every <dir>/<id>/addon.yaml.
func Load(dir string) (Catalog, error) {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return nil, err
	}
	c := Catalog{}
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		path := filepath.Join(dir, e.Name(), "addon.yaml")
		data, err := os.ReadFile(path)
		if os.IsNotExist(err) {
			continue
		}
		if err != nil {
			return nil, err
		}
		a, err := Parse(data)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", path, err)
		}
		if a.ID != e.Name() {
			return nil, fmt.Errorf("%s: id %q does not match the directory", path, a.ID)
		}
		a.Dir = filepath.Join(dir, e.Name())
		c[a.ID] = a
	}
	for _, a := range c {
		for _, r := range append(append([]string{}, a.Requires...), a.Recommends...) {
			if c[r] == nil {
				return nil, fmt.Errorf("%s: unknown add-on %q", a.ID, r)
			}
		}
	}
	return c, nil
}

// IDs returns the add-on ids sorted.
func (c Catalog) IDs() []string {
	ids := make([]string, 0, len(c))
	for id := range c {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

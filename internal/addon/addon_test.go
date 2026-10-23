package addon

import "testing"

func TestLoadCatalog(t *testing.T) {
	c, err := Load("../../addons")
	if err != nil {
		t.Fatal(err)
	}
	for _, id := range []string{"supla", "mosquitto", "zigbee2mqtt", "zigbee2supla"} {
		if c[id] == nil {
			t.Errorf("missing add-on %s", id)
		}
	}
	if got := c["supla"].Description.In("pl"); got == "" || got == c["supla"].Description.In("en") {
		t.Errorf("no Polish description: %q", got)
	}
	if got := c["mosquitto"].Name.In("pl"); got != "Mosquitto" {
		t.Errorf("plain name in pl = %q", got)
	}
}

func TestParse(t *testing.T) {
	a, err := Parse([]byte(`
id: test
name: Test
compose:
  - a.yml
  - file: b.yml
    when: {option: MODE, equals: b}
services: [test]
env:
  B: "2"
  A: "1"
options:
  - key: MODE
    type: select
    choices: [a, b]
`))
	if err != nil {
		t.Fatal(err)
	}
	if a.Compose[0].File != "a.yml" || a.Compose[1].When.Equals != "b" {
		t.Errorf("compose = %+v", a.Compose)
	}
	if len(a.FixedEnv) != 2 || a.FixedEnv[0] != (KV{"B", "2"}) {
		t.Errorf("env order = %v", a.FixedEnv)
	}
}

func TestParseErrors(t *testing.T) {
	for name, src := range map[string]string{
		"unknown field": "id: x\ncompose: [a.yml]\nservices: [x]\nfoo: 1\n",
		"bad id":        "id: X\ncompose: [a.yml]\nservices: [x]\n",
		"no services":   "id: x\ncompose: [a.yml]\n",
		"path escape":   "id: x\ncompose: [a.yml]\nservices: [x]\ndata: [../etc]\n",
		"compose dir":   "id: x\ncompose: [sub/a.yml]\nservices: [x]\n",
		"select":        "id: x\ncompose: [a.yml]\nservices: [x]\noptions: [{key: A, type: select}]\n",
		"secret":        "id: x\ncompose: [a.yml]\nservices: [x]\noptions: [{key: A, type: secret}]\n",
		"proxy service": "id: x\ncompose: [a.yml]\nservices: [x]\nweb: {port: \"1\", proxy: \"y:80\"}\n",
		"proxy port":    "id: x\ncompose: [a.yml]\nservices: [x]\nweb: {port: \"1\", proxy: x}\n",
	} {
		if _, err := Parse([]byte(src)); err == nil {
			t.Errorf("%s: no error", name)
		}
	}
}

func TestConditionIn(t *testing.T) {
	c := &Condition{Option: "M", In: []string{"a", "b"}}
	none := func(string) bool { return false }
	if !c.Match(none, map[string]string{"M": "b"}) || c.Match(none, map[string]string{"M": "c"}) {
		t.Error("in")
	}
}

package system

import (
	"reflect"
	"testing"
)

func TestParseTerse(t *testing.T) {
	got := parseTerse("*:Dom\\:5G:72:WPA2\n:back\\\\slash::\n")
	want := [][]string{{"*", "Dom:5G", "72", "WPA2"}, {"", "back\\slash", "", ""}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseTerse = %q", got)
	}
}

func TestParseScan(t *testing.T) {
	rows := parseTerse(`:Dom:40:WPA2
*:Dom:72:WPA2
::90:WPA2
:Kawiarnia:55:--
:Sasiad:72:WPA1 WPA2
`)
	got := parseScan(rows)
	want := []WifiNetwork{
		{SSID: "Dom", Signal: 72, Security: "WPA2", InUse: true},
		{SSID: "Sasiad", Signal: 72, Security: "WPA1 WPA2"},
		{SSID: "Kawiarnia", Signal: 55},
	}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("parseScan = %+v", got)
	}
}

func TestWifiConnectValidate(t *testing.T) {
	for _, c := range []WifiConnect{{SSID: ""}, {SSID: "Dom", Password: "short"}, {SSID: "123456789012345678901234567890123"}} {
		if c.Validate() == nil {
			t.Errorf("%+v accepted", c)
		}
	}
	for _, c := range []WifiConnect{{SSID: "Kawiarnia"}, {SSID: "Dom", Password: "12345678"}} {
		if err := c.Validate(); err != nil {
			t.Errorf("%+v: %v", c, err)
		}
	}
}

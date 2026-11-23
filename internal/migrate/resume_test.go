package migrate

import (
	"io"
	"slices"
	"testing"
)

func TestCheckResume(t *testing.T) {
	ch := func(id, n int) Channel { return Channel{ID: id, Number: n} }
	b := &Bundle{Email: "a@b.c", Devices: []Device{
		{ID: 1, GUID: "00000000-0000-0000-0000-000000000001", Channels: []Channel{ch(11, 0), ch(12, 1)}},
		{ID: 2, GUID: "00000000-0000-0000-0000-000000000002", Channels: []Channel{ch(21, 0)}},
		{ID: 3, GUID: "00000000-0000-0000-0000-000000000003"},
		{ID: 4, GUID: "00000000-0000-0000-0000-000000000004"},
		{ID: 5, GUID: "00000000-0000-0000-0000-000000000005"},
		{ID: 6, GUID: "00000000-0000-0000-0000-000000000006"},
	}}
	db := &fakeDB{rows: map[string][][]string{
		"SELECT id FROM supla_user": {{"3"}},
		"SELECT id, HEX(guid)": {
			{"1", "00000000000000000000000000000001", "3", "0"},  // waiting
			{"2", "00000000000000000000000000000002", "3", "1"},  // here
			{"4", "00000000000000000000000000000009", "3", "1"},  // another GUID
			{"5", "00000000000000000000000000000005", "8", "0"},  // another account
			{"77", "00000000000000000000000000000006", "3", "1"}, // the GUID under another ID
		},
		// Channel 12 is here with another number.
		"SELECT id, iodevice_id": {{"11", "1", "0"}, {"12", "1", "5"}, {"21", "2", "0"}},
	}}
	r, err := CheckResume(db, b)
	if err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(r.Waiting, []int{1}) || !slices.Equal(r.Here, []int{2}) || !slices.Equal(r.Missing, []int{3}) ||
		!slices.Equal(r.Foreign, []int{4, 5, 6}) || !slices.Equal(r.SkipChannels, []int{12}) || !r.Possible() {
		t.Errorf("%+v", r)
	}
	devices := r.Start([]int{2})
	if devices[1].Status != "waiting" || devices[2].Status != "waiting" || devices[3].Status != "missing" || devices[4].Status != "foreign" {
		t.Errorf("start with overwrite: %v %v %v %v", devices[1], devices[2], devices[3], devices[4])
	}
	if r.Start(nil)[2].Status != "kept" {
		t.Error("a device here is overwritten without asking")
	}
	s := &State{Resumed: true, Devices: r.Start(nil), SkipChannels: r.SkipChannels}
	m := s.Moved(b)
	if len(m.Devices) != 2 || m.Devices[0].ID != 1 || len(m.Devices[0].Channels) != 1 || m.Devices[0].Channels[0].ID != 11 ||
		len(b.Devices[0].Channels) != 2 {
		t.Errorf("moved: %+v", m.Devices)
	}
	// Apply sets only the waiting devices.
	cloud := &fakeCloud{}
	db.rows["SELECT id FROM supla_iodevice WHERE last_connected"] = [][]string{{"1"}, {"2"}}
	if _, err := Apply(db, cloud, m, s, io.Discard); err != nil {
		t.Fatal(err)
	}
	if s.Devices[1].Status != "done" || s.Devices[2].Status != "kept" {
		t.Errorf("apply: %v %v", s.Devices[1], s.Devices[2])
	}

	// No account with the e-mail: nothing to resume.
	r, _ = CheckResume(&fakeDB{rows: map[string][][]string{"SELECT id, HEX(guid)": db.rows["SELECT id, HEX(guid)"]}}, b)
	if r.Possible() {
		t.Errorf("without the account: %+v", r)
	}
	// A move that was not resumed moves all.
	if (&State{}).Moved(b) != b {
		t.Error("a move not resumed filtered")
	}
}

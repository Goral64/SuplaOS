package migrate

import (
	"fmt"
	"slices"
	"strconv"
	"strings"
)

// Resume compares the devices of the bundle with this server, for a move
// that was ended before all of them were switched. A device is matched
// when it is here with its ID and GUID on the account of the e-mail of the
// source.
type Resume struct {
	// Waiting: matched, not connected here yet.
	Waiting []int `json:"waiting"`
	// Here: matched and connected here; its settings may have been changed
	// here since.
	Here []int `json:"here"`
	// Missing: not here, e.g. deleted here or new in the source.
	Missing []int `json:"missing"`
	// Foreign: its ID or GUID belongs to another device or account here.
	Foreign []int `json:"foreign"`
	// SkipChannels are channels of the matched devices without the same
	// ID, device and number here.
	SkipChannels []int `json:"-"`
}

// Possible tells whether there is a device to go on with.
func (r *Resume) Possible() bool { return len(r.Waiting)+len(r.Here) > 0 }

// CheckResume compares the bundle with the database of this server.
func CheckResume(db DB, b *Bundle) (*Resume, error) {
	r := &Resume{Waiting: []int{}, Here: []int{}, Missing: []int{}, Foreign: []int{}}
	user := 0
	rows, err := db.Query("SELECT id FROM supla_user WHERE email = " + str(b.Email) + ";")
	if err != nil {
		return nil, err
	}
	if len(rows) > 0 {
		user, _ = strconv.Atoi(rows[0][0])
	}
	var guids, chans []string
	for _, d := range b.Devices {
		g, err := guidHex(d.GUID)
		if err != nil {
			return nil, err
		}
		guids = append(guids, "X'"+g+"'")
		for _, c := range d.Channels {
			chans = append(chans, strconv.Itoa(c.ID))
		}
	}
	if len(guids) == 0 {
		return r, nil
	}
	type local struct {
		guid      string
		user      int
		connected bool
	}
	byID, byGUID := map[int]local{}, map[string]bool{}
	rows, err = db.Query(fmt.Sprintf("SELECT id, HEX(guid), user_id, last_connected IS NOT NULL FROM supla_iodevice WHERE id IN (%s) OR guid IN (%s);",
		ids(b.Devices, func(d Device) int { return d.ID }), strings.Join(guids, ",")))
	if err != nil {
		return nil, err
	}
	for _, row := range rows {
		if len(row) < 4 {
			continue
		}
		id, _ := strconv.Atoi(row[0])
		u, _ := strconv.Atoi(row[2])
		byID[id] = local{guid: strings.ToUpper(row[1]), user: u, connected: row[3] == "1"}
		byGUID[strings.ToUpper(row[1])] = true
	}
	type channel struct{ device, number int }
	here := map[int]channel{}
	if len(chans) > 0 {
		rows, err = db.Query("SELECT id, iodevice_id, channel_number FROM supla_dev_channel WHERE id IN (" + strings.Join(chans, ",") + ");")
		if err != nil {
			return nil, err
		}
		for _, row := range rows {
			if len(row) < 3 {
				continue
			}
			id, _ := strconv.Atoi(row[0])
			dev, _ := strconv.Atoi(row[1])
			num, _ := strconv.Atoi(row[2])
			here[id] = channel{dev, num}
		}
	}
	for _, d := range b.Devices {
		g, _ := guidHex(d.GUID)
		l, ok := byID[d.ID]
		switch {
		case ok && l.guid == g && user > 0 && l.user == user:
			if l.connected {
				r.Here = append(r.Here, d.ID)
			} else {
				r.Waiting = append(r.Waiting, d.ID)
			}
			for _, c := range d.Channels {
				if h, ok := here[c.ID]; !ok || h.device != d.ID || h.number != c.Number {
					r.SkipChannels = append(r.SkipChannels, c.ID)
				}
			}
		case ok || byGUID[g]:
			r.Foreign = append(r.Foreign, d.ID)
		default:
			r.Missing = append(r.Missing, d.ID)
		}
	}
	return r, nil
}

// Start returns the state of a resumed move: the matched devices wait for
// their settings unless they are here already and not in overwrite, the
// others are left alone.
func (r *Resume) Start(overwrite []int) map[int]*DeviceState {
	out := map[int]*DeviceState{}
	for _, id := range r.Waiting {
		out[id] = &DeviceState{Status: "waiting"}
	}
	for _, id := range r.Here {
		if slices.Contains(overwrite, id) {
			out[id] = &DeviceState{Status: "waiting"}
		} else {
			out[id] = &DeviceState{Status: "kept"}
		}
	}
	for _, id := range r.Missing {
		out[id] = &DeviceState{Status: "missing"}
	}
	for _, id := range r.Foreign {
		out[id] = &DeviceState{Status: "foreign"}
	}
	return out
}

// Moved returns the bundle without the devices and channels the move
// leaves alone: those not here or of another device, in a resumed move.
func (s *State) Moved(b *Bundle) *Bundle {
	if !s.Resumed {
		return b
	}
	out := *b
	out.Devices = nil
	for _, d := range b.Devices {
		if ds := s.Devices[d.ID]; ds != nil && (ds.Status == "missing" || ds.Status == "foreign") {
			continue
		}
		d.Channels = slices.DeleteFunc(slices.Clone(d.Channels), func(c Channel) bool {
			return slices.Contains(s.SkipChannels, c.ID)
		})
		out.Devices = append(out.Devices, d)
	}
	return &out
}

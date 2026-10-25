package migrate

import (
	"encoding/hex"
	"fmt"
	"strconv"
	"strings"
)

// str is a string literal for MariaDB: as hex, so that nothing in it can
// break the statement.
func str(s string) string {
	if s == "" {
		return "''"
	}
	return "_utf8mb4 X'" + hex.EncodeToString([]byte(s)) + "'"
}

func strp(s *string) string {
	if s == nil {
		return "NULL"
	}
	return str(*s)
}

func intp(i *int) string {
	if i == nil {
		return "NULL"
	}
	return strconv.Itoa(*i)
}

func boolean(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// guidHex is the GUID as stored in supla_iodevice.guid, without dashes.
func guidHex(guid string) (string, error) {
	h := strings.ReplaceAll(strings.ToUpper(guid), "-", "")
	if _, err := hex.DecodeString(h); err != nil || len(h) != 32 {
		return "", fmt.Errorf("invalid GUID %q", guid)
	}
	return h, nil
}

func ids[T any](list []T, id func(T) int) string {
	var s []string
	for _, x := range list {
		s = append(s, strconv.Itoa(id(x)))
	}
	if len(s) == 0 {
		return "NULL"
	}
	return strings.Join(s, ",")
}

// ConflictSQL finds objects of this server in the way of the bundle: the
// same IDs or a device with the same GUID. One line per conflict: kind and
// ID.
func ConflictSQL(b *Bundle) (string, error) {
	var channels []Channel
	var guids []string
	for _, d := range b.Devices {
		channels = append(channels, d.Channels...)
		g, err := guidHex(d.GUID)
		if err != nil {
			return "", err
		}
		guids = append(guids, "X'"+g+"'")
	}
	if len(guids) == 0 {
		guids = []string{"NULL"}
	}
	return fmt.Sprintf(`SELECT 'location', id FROM supla_location WHERE id IN (%s)
UNION ALL SELECT 'accessid', id FROM supla_accessid WHERE id IN (%s)
UNION ALL SELECT 'device', id FROM supla_iodevice WHERE id IN (%s) OR guid IN (%s)
UNION ALL SELECT 'channel', id FROM supla_dev_channel WHERE id IN (%s);
`,
		ids(b.Locations, func(l Location) int { return l.ID }),
		ids(b.AccessIDs, func(a AccessID) int { return a.ID }),
		ids(b.Devices, func(d Device) int { return d.ID }), strings.Join(guids, ","),
		ids(channels, func(c Channel) int { return c.ID })), nil
}

// LimitsSQL raises the limits of the local account so that the objects
// of the bundle fit with room to spare: an account of the public Cloud may
// have higher limits than a new one, and on an own server they only get
// in the way. The skeleton goes past them in the database, and a device
// added later would be refused. Limits are never lowered.
func LimitsSQL(b *Bundle, userID int) string {
	perGroup, perScene := 0, 0
	for _, g := range b.ChannelGroups {
		perGroup = max(perGroup, len(g.ChannelIDs))
	}
	for _, s := range b.Scenes {
		perScene = max(perScene, len(s.Operations))
	}
	limits := []struct {
		col        string
		have, base int
	}{
		{"limit_iodev", len(b.Devices), 500},
		{"limit_loc", len(b.Locations), 100},
		{"limit_aid", len(b.AccessIDs), 100},
		{"limit_channel_group", len(b.ChannelGroups), 200},
		{"limit_channel_per_group", perGroup, 50},
		{"limit_scene", len(b.Scenes), 200},
		{"limit_operations_per_scene", perScene, 50},
		{"limit_schedule", len(b.Schedules), 200},
		{"limit_direct_link", len(b.DirectLinks), 200},
		{"limit_value_based_triggers", len(b.Reactions), 200},
	}
	var set []string
	for _, l := range limits {
		set = append(set, fmt.Sprintf("%s = GREATEST(%s, %d)", l.col, l.col, max(l.base, 2*l.have)))
	}
	return fmt.Sprintf("UPDATE supla_user SET %s WHERE id = %d;\n", strings.Join(set, ", "), userID)
}

// SkeletonSQL puts the locations, access IDs, devices and channels of the
// bundle in the database of this server with their IDs, for the user.
// Devices get no auth key: supla-server lets the device in with any key
// once and stores it. The settings of channels that need the device (their
// configuration) are set later through the Cloud.
func SkeletonSQL(b *Bundle, userID int) (string, error) {
	var sb strings.Builder
	w := func(format string, args ...any) { fmt.Fprintf(&sb, format, args...) }
	w("START TRANSACTION;\n")
	sb.WriteString(LimitsSQL(b, userID))
	for _, l := range b.Locations {
		w("INSERT INTO supla_location (id, user_id, password, caption, enabled) VALUES (%d, %d, %s, %s, %s);\n",
			l.ID, userID, str(l.Password), strp(l.Caption), boolean(l.Enabled))
	}
	for _, a := range b.AccessIDs {
		w("INSERT INTO supla_accessid (id, user_id, password, caption, enabled) VALUES (%d, %d, %s, %s, %s);\n",
			a.ID, userID, str(a.Password), strp(a.Caption), boolean(a.Enabled))
	}
	for _, l := range b.Locations {
		for _, a := range l.AccessIDs {
			w("INSERT INTO supla_rel_aidloc (location_id, access_id) VALUES (%d, %d);\n", l.ID, a.ID)
		}
	}
	for _, d := range b.Devices {
		g, err := guidHex(d.GUID)
		if err != nil {
			return "", err
		}
		w("INSERT INTO supla_iodevice (id, location_id, original_location_id, user_id, guid, name, enabled, comment, "+
			"reg_date, software_version, protocol_version, product_id, auth_key) VALUES "+
			"(%d, %d, %s, %d, X'%s', %s, %s, %s, UTC_TIMESTAMP(), %s, 0, %s, NULL);\n",
			d.ID, d.LocationID, intp(d.OriginalLocationID), userID, g, strp(d.Name), boolean(d.Enabled),
			strp(d.Comment), strp(d.SoftwareVersion), intp(d.ProductID))
		for _, c := range d.Channels {
			loc := "NULL"
			if !c.InheritedLocation {
				loc = strconv.Itoa(c.LocationID)
			}
			w("INSERT INTO supla_dev_channel (id, iodevice_id, user_id, channel_number, caption, type, func, "+
				"param1, param2, param3, alt_icon, hidden, location_id, sub_device_id) VALUES "+
				"(%d, %d, %d, %d, %s, %d, %d, 0, 0, 0, %d, %s, %s, %d);\n",
				c.ID, d.ID, userID, c.Number, strp(c.Caption), c.TypeID, c.FunctionID,
				c.AltIcon, boolean(c.Hidden), loc, c.SubDeviceID)
		}
	}
	w("COMMIT;\n")
	return sb.String(), nil
}

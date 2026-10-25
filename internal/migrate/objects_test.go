package migrate

import (
	"encoding/json"
	"errors"
	"io"
	"strings"
	"testing"
)

// idCloud answers every POST with a new ID, from 100.
type idCloud struct {
	next  int
	calls []string
	fail  string
}

func (c *idCloud) Call(method, path string, body, out any) error {
	data, _ := json.Marshal(body)
	c.calls = append(c.calls, method+" "+path+" "+string(data))
	if c.fail != "" && strings.Contains(path, c.fail) {
		return errors.New("refused")
	}
	if out != nil {
		c.next++
		json.Unmarshal([]byte(`{"id":`+itoa(99+c.next)+`,"slug":"abc"}`), out)
	}
	return nil
}

func itoa(i int) string { b, _ := json.Marshal(i); return string(b) }

func ip(i int) *int { return &i }

func TestCreateObjects(t *testing.T) {
	b := bundle()
	b.Devices[0].Channels[0].UserIconID = ip(7)
	b.UserIcons = []UserIcon{{ID: 7, Function: "POWERSWITCH", Images: []string{"a", "b"}, ImagesDark: []string{"c", ""}}}
	b.AccessIDs[0].ActiveHours = json.RawMessage(`{"1":[8,9]}`)
	b.ChannelGroups = []ChannelGroup{{ID: 30, Caption: sp("Grupa"), ChannelIDs: []int{9001}, UserIconID: ip(7)}}
	ch, grp, scene := "channel", "channelGroup", "scene"
	b.Scenes = []Scene{
		// Runs the scene below, made first.
		{ID: 41, Caption: sp("Wszystko"), Operations: []Operation{{SubjectType: &scene, SubjectID: ip(40), ActionID: ip(3000)}}},
		{ID: 40, Caption: sp("Wieczór"), Operations: []Operation{
			{SubjectType: &ch, SubjectID: ip(9001), ActionID: ip(60)}, {DelayMs: 2000},
			{SubjectType: &grp, SubjectID: ip(30), ActionID: ip(70)}}},
	}
	b.Schedules = []Schedule{{ID: 50, Mode: "daily", SubjectType: "scene", SubjectID: 40, Config: json.RawMessage(`[{"crontab":"30 6 * * *"}]`)}}
	b.Reactions = []Reaction{{ID: 60, OwningChannelID: 9002, SubjectType: "channelGroup", SubjectID: 30, ActionID: 60,
		Trigger: json.RawMessage(`{"on_change_to":{"eq":"open"}}`)}}
	b.DirectLinks = []DirectLink{{ID: 70, Caption: sp("Link"), SubjectType: "channel", SubjectID: 9001, AllowedActions: []string{"READ", "TURN_ON"}}}

	cloud := &idCloud{}
	db := &fakeDB{}
	o := CreateObjects(cloud, db, b, io.Discard)
	if len(o.Errors) > 0 {
		t.Fatalf("errors %v", o.Errors)
	}
	all := strings.Join(cloud.calls, "\n")
	for _, want := range []string{
		`POST /api/v3/user-icons.base64 {"function":"POWERSWITCH","image1":"a","image2":"b","imageDark1":"c"}`,
		`PUT /api/v3/accessids/60 {"activeFrom":null,"activeHours":{"1":[8,9]},"activeTo":null,"caption":"Telefon","enabled":true,"locationsIds":[50]}`,
		`POST /api/v3/channel-groups {"altIcon":0,"caption":"Grupa","channelsIds":[9001],"hidden":false,"locationId":0,"userIconId":100}`,
		`"operations":[{"actionId":60,"actionParam":null,"delayMs":0,"subjectId":9001,"subjectType":"channel","waitForCompletion":false},{"delayMs":2000},{"actionId":70,"actionParam":null,"delayMs":0,"subjectId":101,"subjectType":"channelGroup","waitForCompletion":false}]`,
		`"operations":[{"actionId":3000,"actionParam":null,"delayMs":0,"subjectId":102,"subjectType":"scene","waitForCompletion":false}]`,
		`POST /api/v3/schedules {"caption":null,"config":[{"crontab":"30 6 * * *"}],"dateEnd":null,"dateStart":null,"enabled":false,"mode":"daily","retry":false,"subjectId":102,"subjectType":"scene"}`,
		`POST /api/v3/channels/9002/reactions {"actionId":60,"actionParam":null,"activeFrom":null,"activeHours":null,"activeTo":null,"activityConditions":null,"enabled":false,"subjectId":101,"subjectType":"channelGroup","trigger":{"on_change_to":{"eq":"open"}}}`,
		`"allowedActions":["read","turn-on"]`,
	} {
		if !strings.Contains(all, want) {
			t.Errorf("missing %s in\n%s", want, all)
		}
	}
	if len(db.ran) != 1 || db.ran[0] != "UPDATE supla_dev_channel SET user_icon_id = 100 WHERE id = 9001;" {
		t.Errorf("icons of channels: %v", db.ran)
	}
	if o.Counts["scenes"] != 2 || len(o.Links) != 1 || o.Links[0].Slug != "abc" {
		t.Errorf("counts %v links %v", o.Counts, o.Links)
	}

	// A group that fails: the scenes, the schedule and the reaction that
	// depend on it are listed,
	// the rest is made.
	o = CreateObjects(&idCloud{fail: "channel-groups"}, &fakeDB{}, b, io.Discard)
	if len(o.Errors) != 5 || o.Counts["directLinks"] != 1 {
		t.Errorf("errors %v counts %v", o.Errors, o.Counts)
	}
}

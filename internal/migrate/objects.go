package migrate

import (
	"encoding/json"
	"fmt"
	"io"
	"slices"
	"strings"
)

// Objects is what CreateObjects made: the new IDs of icons, groups and
// scenes, which the Cloud gives anew, the new addresses of direct links
// and what could not be made.
type Objects struct {
	Icons  map[int]int    `json:"icons"`
	Groups map[int]int    `json:"groups"`
	Scenes map[int]int    `json:"scenes"`
	Counts map[string]int `json:"counts"`
	Links  []NewLink      `json:"links"`
	Errors []string       `json:"errors"`
}

// NewLink is a direct link made anew: its new address replaces the old one.
type NewLink struct {
	Caption string `json:"caption"`
	OldID   int    `json:"oldId"`
	ID      int    `json:"id"`
	Slug    string `json:"slug"`
}

// subject maps the subject of an action to this server: channels keep
// their IDs, groups and scenes get new ones. ok is false when the subject
// was not made.
func (o *Objects) subject(kind string, id int) (int, bool) {
	switch kind {
	case "channelGroup":
		n, ok := o.Groups[id]
		return n, ok
	case "scene":
		n, ok := o.Scenes[id]
		return n, ok
	}
	return id, true
}

func (o *Objects) icon(id *int) *int {
	if id == nil {
		return nil
	}
	if n, ok := o.Icons[*id]; ok {
		return &n
	}
	return nil
}

func (o *Objects) fail(format string, args ...any) {
	o.Errors = append(o.Errors, fmt.Sprintf(format, args...))
}

// raw passes JSON on as it is, null when empty.
func raw(r json.RawMessage) json.RawMessage {
	if len(r) == 0 {
		return json.RawMessage("null")
	}
	return r
}

// CreateObjects makes in the local Cloud what refers to the channels put
// in the database by Prepare: icons, active hours of access IDs, channel
// groups, scenes, schedules, reactions and direct links. The channels need
// not be connected. An object that fails is listed in Errors, the rest are
// made anyway.
func CreateObjects(cloud Cloud, db DB, b *Bundle, log io.Writer) *Objects {
	o := &Objects{Icons: map[int]int{}, Groups: map[int]int{}, Scenes: map[int]int{}, Counts: map[string]int{}}
	count := func(kind string) { o.Counts[kind]++ }

	for _, icon := range b.UserIcons {
		body := map[string]any{"function": icon.Function}
		for i, img := range icon.Images {
			body[fmt.Sprintf("image%d", i+1)] = img
		}
		for i, img := range icon.ImagesDark {
			if img != "" {
				body[fmt.Sprintf("imageDark%d", i+1)] = img
			}
		}
		var made struct {
			ID int `json:"id"`
		}
		if err := cloud.Call("POST", "/api/v3/user-icons.base64", body, &made); err != nil {
			o.fail("icon %d: %v", icon.ID, err)
			continue
		}
		o.Icons[icon.ID] = made.ID
		count("icons")
	}
	var sets []string
	for _, d := range b.Devices {
		for _, c := range d.Channels {
			if n := o.icon(c.UserIconID); n != nil {
				sets = append(sets, fmt.Sprintf("UPDATE supla_dev_channel SET user_icon_id = %d WHERE id = %d;", *n, c.ID))
			}
		}
	}
	if len(sets) > 0 {
		if _, err := db.Query(strings.Join(sets, "\n")); err != nil {
			o.fail("icons of channels: %v", err)
		}
	}

	locations := map[int][]int{}
	for _, l := range b.Locations {
		for _, a := range l.AccessIDs {
			locations[a.ID] = append(locations[a.ID], l.ID)
		}
	}
	for _, a := range b.AccessIDs {
		if a.ActiveFrom == nil && a.ActiveTo == nil && emptyConfig(a.ActiveHours) {
			continue
		}
		body := map[string]any{
			"caption": a.Caption, "enabled": a.Enabled, "locationsIds": nonNil(locations[a.ID]),
			"activeFrom": a.ActiveFrom, "activeTo": a.ActiveTo, "activeHours": raw(a.ActiveHours),
		}
		if err := cloud.Call("PUT", fmt.Sprintf("/api/v3/accessids/%d", a.ID), body, nil); err != nil {
			o.fail("access ID %d: %v", a.ID, err)
			continue
		}
		count("accessIdHours")
	}

	for _, g := range b.ChannelGroups {
		body := map[string]any{
			"caption": g.Caption, "channelsIds": nonNil(g.ChannelIDs), "locationId": g.LocationID,
			"altIcon": g.AltIcon, "hidden": g.Hidden, "userIconId": o.icon(g.UserIconID),
		}
		var made struct {
			ID int `json:"id"`
		}
		if err := cloud.Call("POST", "/api/v3/channel-groups", body, &made); err != nil {
			o.fail("channel group %d: %v", g.ID, err)
			continue
		}
		o.Groups[g.ID] = made.ID
		count("channelGroups")
	}

	// A scene may run other scenes: made after them.
	left := b.Scenes
	for len(left) > 0 {
		var next []Scene
		for _, sc := range left {
			ops, ready, err := o.operations(sc, b)
			if err != nil {
				o.fail("scene %d: %v", sc.ID, err)
				continue
			}
			if !ready {
				next = append(next, sc)
				continue
			}
			body := map[string]any{
				"caption": sc.Caption, "enabled": sc.Enabled, "hidden": sc.Hidden, "locationId": sc.LocationID,
				"altIcon": sc.AltIcon, "userIconId": o.icon(sc.UserIconID), "operations": ops,
			}
			var made struct {
				ID int `json:"id"`
			}
			if err := cloud.Call("POST", "/api/v3/scenes", body, &made); err != nil {
				o.fail("scene %d: %v", sc.ID, err)
				continue
			}
			o.Scenes[sc.ID] = made.ID
			count("scenes")
		}
		if len(next) == len(left) {
			for _, sc := range next {
				o.fail("scene %d: it runs a scene that was not made", sc.ID)
			}
			break
		}
		left = next
	}

	for _, s := range b.Schedules {
		id, ok := o.subject(s.SubjectType, s.SubjectID)
		if !ok {
			o.fail("schedule %d: its %s was not made", s.ID, s.SubjectType)
			continue
		}
		body := map[string]any{
			"caption": s.Caption, "mode": s.Mode, "config": raw(s.Config), "subjectType": s.SubjectType,
			"subjectId": id, "dateStart": s.DateStart, "dateEnd": s.DateEnd, "retry": s.Retry, "enabled": s.Enabled,
		}
		if err := cloud.Call("POST", "/api/v3/schedules", body, nil); err != nil {
			o.fail("schedule %d: %v", s.ID, err)
			continue
		}
		count("schedules")
	}

	for _, r := range b.Reactions {
		id, ok := o.subject(r.SubjectType, r.SubjectID)
		if !ok {
			o.fail("reaction %d: its %s was not made", r.ID, r.SubjectType)
			continue
		}
		body := map[string]any{
			"subjectType": r.SubjectType, "subjectId": id, "actionId": r.ActionID, "actionParam": raw(r.ActionParam),
			"trigger": raw(r.Trigger), "enabled": r.Enabled, "activeFrom": r.ActiveFrom, "activeTo": r.ActiveTo,
			"activeHours": raw(r.ActiveHours), "activityConditions": raw(r.ActivityConditions),
		}
		if err := cloud.Call("POST", fmt.Sprintf("/api/v3/channels/%d/reactions", r.OwningChannelID), body, nil); err != nil {
			o.fail("reaction %d: %v", r.ID, err)
			continue
		}
		count("reactions")
	}

	for _, l := range b.DirectLinks {
		id, ok := o.subject(l.SubjectType, l.SubjectID)
		if !ok {
			o.fail("direct link %d: its %s was not made", l.ID, l.SubjectType)
			continue
		}
		actions := []string{}
		for _, a := range l.AllowedActions {
			actions = append(actions, strings.ReplaceAll(strings.ToLower(a), "_", "-"))
		}
		body := map[string]any{
			"caption": l.Caption, "subjectType": l.SubjectType, "subjectId": id, "allowedActions": actions,
			"activeDateRange": raw(l.ActiveDateRange), "executionsLimit": l.ExecutionsLimit,
			"enabled": l.Enabled, "disableHttpGet": l.DisableHTTPGet,
		}
		var made struct {
			ID   int    `json:"id"`
			Slug string `json:"slug"`
		}
		if err := cloud.Call("POST", "/api/v3/direct-links", body, &made); err != nil {
			o.fail("direct link %d: %v", l.ID, err)
			continue
		}
		caption := ""
		if l.Caption != nil {
			caption = *l.Caption
		}
		o.Links = append(o.Links, NewLink{Caption: caption, OldID: l.ID, ID: made.ID, Slug: made.Slug})
		count("directLinks")
	}
	for _, e := range o.Errors {
		fmt.Fprintln(log, "Not moved:", e)
	}
	return o
}

// operations maps the operations of a scene; ready is false while a scene
// it runs is not made yet.
func (o *Objects) operations(sc Scene, b *Bundle) ([]map[string]any, bool, error) {
	var ops []map[string]any
	for _, op := range sc.Operations {
		if op.SubjectType == nil || op.SubjectID == nil {
			ops = append(ops, map[string]any{"delayMs": op.DelayMs})
			continue
		}
		id, ok := o.subject(*op.SubjectType, *op.SubjectID)
		if !ok {
			if *op.SubjectType == "scene" && sceneIn(b, *op.SubjectID) {
				return nil, false, nil
			}
			return nil, false, fmt.Errorf("its %s %d was not made", *op.SubjectType, *op.SubjectID)
		}
		ops = append(ops, map[string]any{
			"subjectType": *op.SubjectType, "subjectId": id, "actionId": op.ActionID,
			"actionParam": raw(op.ActionParam), "delayMs": op.DelayMs, "waitForCompletion": op.WaitForCompletion,
		})
	}
	return ops, true, nil
}

func sceneIn(b *Bundle, id int) bool {
	for _, sc := range b.Scenes {
		if sc.ID == id {
			return true
		}
	}
	return false
}

func nonNil(ids []int) []int {
	if ids == nil {
		return []int{}
	}
	return ids
}

// RemoveDefaults deletes the locations and access IDs of the local account
// that are not from the source: those a new account gets.
func RemoveDefaults(cloud Cloud, b *Bundle) {
	var list []struct {
		ID int `json:"id"`
	}
	if cloud.Call("GET", "/api/v3/accessids", nil, &list) == nil {
		for _, a := range list {
			if !slices.ContainsFunc(b.AccessIDs, func(x AccessID) bool { return x.ID == a.ID }) {
				cloud.Call("DELETE", fmt.Sprintf("/api/v3/accessids/%d", a.ID), nil, nil)
			}
		}
	}
	list = nil
	if cloud.Call("GET", "/api/v3/locations", nil, &list) == nil {
		for _, l := range list {
			if !slices.ContainsFunc(b.Locations, func(x Location) bool { return x.ID == l.ID }) {
				cloud.Call("DELETE", fmt.Sprintf("/api/v3/locations/%d", l.ID), nil, nil)
			}
		}
	}
}

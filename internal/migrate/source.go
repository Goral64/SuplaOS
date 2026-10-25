// Package migrate moves an account of another Supla Cloud (the public one,
// or another own server) to the Supla of this device. Devices and channels
// keep their IDs: they are put in the database before the devices are
// switched to this server, and supla-server lets a device in whose GUID is
// there without an auth key, keeping the channels it finds by number.
package migrate

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// Location of the source account; the password is the one devices of old
// firmware register with.
type Location struct {
	ID        int     `json:"id"`
	Caption   *string `json:"caption"`
	Password  string  `json:"password"`
	Enabled   bool    `json:"enabled"`
	AccessIDs []struct {
		ID int `json:"id"`
	} `json:"accessIds"`
}

// AccessID is an access identifier of the mobile apps.
type AccessID struct {
	ID          int             `json:"id"`
	Caption     *string         `json:"caption"`
	Password    string          `json:"password"`
	Enabled     bool            `json:"enabled"`
	ActiveFrom  *string         `json:"activeFrom"`
	ActiveTo    *string         `json:"activeTo"`
	ActiveHours json.RawMessage `json:"activeHours"`
}

// Channel of a device with its settings.
type Channel struct {
	ID                int             `json:"id"`
	Number            int             `json:"channelNumber"`
	TypeID            int             `json:"typeId"`
	FunctionID        int             `json:"functionId"`
	Caption           *string         `json:"caption"`
	AltIcon           int             `json:"altIcon"`
	Hidden            bool            `json:"hidden"`
	LocationID        int             `json:"locationId"`
	InheritedLocation bool            `json:"inheritedLocation"`
	UserIconID        *int            `json:"userIconId"`
	SubDeviceID       int             `json:"subDeviceId"`
	Config            json.RawMessage `json:"config"`
}

// Device of the source account.
type Device struct {
	ID                 int             `json:"id"`
	GUID               string          `json:"gUIDString"`
	Name               *string         `json:"name"`
	Comment            *string         `json:"comment"`
	Enabled            bool            `json:"enabled"`
	LocationID         int             `json:"locationId"`
	OriginalLocationID *int            `json:"originalLocationId"`
	SoftwareVersion    *string         `json:"softwareVersion"`
	ProductID          *int            `json:"productId"`
	Config             json.RawMessage `json:"config"`
	LastConnected      *string         `json:"lastConnected"`
	Channels           []Channel       `json:"channels"`
}

// UserIcon is an icon of the user, its images in base64.
type UserIcon struct {
	ID         int      `json:"id"`
	Function   string   `json:"function"`
	Images     []string `json:"images"`
	ImagesDark []string `json:"imagesDark"`
}

// ChannelGroup is a group of channels of the same function.
type ChannelGroup struct {
	ID         int     `json:"id"`
	Caption    *string `json:"caption"`
	AltIcon    int     `json:"altIcon"`
	Hidden     bool    `json:"hidden"`
	LocationID int     `json:"locationId"`
	UserIconID *int    `json:"userIconId"`
	ChannelIDs []int   `json:"channelIds"`
}

// Operation of a scene; a delay only without a subject.
type Operation struct {
	SubjectType       *string         `json:"subjectType"`
	SubjectID         *int            `json:"subjectId"`
	ActionID          *int            `json:"actionId"`
	ActionParam       json.RawMessage `json:"actionParam"`
	DelayMs           int             `json:"delayMs"`
	WaitForCompletion bool            `json:"waitForCompletion"`
}

// Scene of the source account.
type Scene struct {
	ID         int         `json:"id"`
	Caption    *string     `json:"caption"`
	Enabled    bool        `json:"enabled"`
	Hidden     bool        `json:"hidden"`
	LocationID int         `json:"locationId"`
	AltIcon    int         `json:"altIcon"`
	UserIconID *int        `json:"userIconId"`
	Operations []Operation `json:"operations"`
}

// Schedule runs an action on a subject at set times.
type Schedule struct {
	ID          int             `json:"id"`
	Caption     *string         `json:"caption"`
	Mode        string          `json:"mode"`
	Config      json.RawMessage `json:"config"`
	SubjectType string          `json:"subjectType"`
	SubjectID   int             `json:"subjectId"`
	DateStart   *string         `json:"dateStart"`
	DateEnd     *string         `json:"dateEnd"`
	Retry       bool            `json:"retry"`
	Enabled     bool            `json:"enabled"`
}

// Reaction runs an action when the state of a channel changes.
type Reaction struct {
	ID                 int             `json:"id"`
	OwningChannelID    int             `json:"owningChannelId"`
	SubjectType        string          `json:"subjectType"`
	SubjectID          int             `json:"subjectId"`
	ActionID           int             `json:"actionId"`
	ActionParam        json.RawMessage `json:"actionParam"`
	Trigger            json.RawMessage `json:"trigger"`
	Enabled            bool            `json:"enabled"`
	ActiveFrom         *string         `json:"activeFrom"`
	ActiveTo           *string         `json:"activeTo"`
	ActiveHours        json.RawMessage `json:"activeHours"`
	ActivityConditions json.RawMessage `json:"activityConditions"`
}

// DirectLink is a link that runs actions without logging in. It gets a new
// address on the new server.
type DirectLink struct {
	ID              int             `json:"id"`
	Caption         *string         `json:"caption"`
	SubjectType     string          `json:"subjectType"`
	SubjectID       int             `json:"subjectId"`
	AllowedActions  []string        `json:"allowedActions"`
	ActiveDateRange json.RawMessage `json:"activeDateRange"`
	ExecutionsLimit *int            `json:"executionsLimit"`
	Enabled         bool            `json:"enabled"`
	DisableHTTPGet  bool            `json:"disableHttpGet"`
}

// Bundle is what is read from the source account.
type Bundle struct {
	Server   string    `json:"server"`
	Fetched  time.Time `json:"fetched"`
	Email    string    `json:"email"`
	Timezone string    `json:"timezone"`

	Locations     []Location     `json:"locations"`
	AccessIDs     []AccessID     `json:"accessIds"`
	Devices       []Device       `json:"devices"`
	UserIcons     []UserIcon     `json:"userIcons"`
	ChannelGroups []ChannelGroup `json:"channelGroups"`
	Scenes        []Scene        `json:"scenes"`
	Schedules     []Schedule     `json:"schedules"`
	Reactions     []Reaction     `json:"reactions"`
	DirectLinks   []DirectLink   `json:"directLinks"`
}

// Source is the API of the source Cloud.
type Source struct {
	Server string
	token  string
	client *http.Client
}

// NewSource reads the address of the server from a personal access token:
// its part after the dot is the address in base64.
func NewSource(token string) (*Source, error) {
	token = strings.TrimSpace(token)
	_, enc, ok := strings.Cut(token, ".")
	if !ok {
		return nil, errors.New("not a personal access token of Supla Cloud")
	}
	addr, err := base64.StdEncoding.DecodeString(enc)
	if err != nil {
		addr, err = base64.RawURLEncoding.DecodeString(enc)
	}
	u, perr := url.Parse(string(addr))
	if err != nil || perr != nil || u.Scheme != "https" || u.Host == "" {
		return nil, errors.New("not a personal access token of Supla Cloud")
	}
	return &Source{Server: u.Scheme + "://" + u.Host, token: token,
		client: &http.Client{Timeout: time.Minute}}, nil
}

// errGone: the object is not in the source (any more).
var errGone = errors.New("not in the source")

func (s *Source) get(path string, out any) error {
	req, err := http.NewRequest("GET", s.Server+"/api/v3/"+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+s.token)
	req.Header.Set("Accept", "application/json")
	res, err := s.client.Do(req)
	if err != nil {
		return err
	}
	defer res.Body.Close()
	data, err := io.ReadAll(io.LimitReader(res.Body, 64<<20))
	if err != nil {
		return err
	}
	switch {
	case res.StatusCode == http.StatusUnauthorized:
		return errors.New("the source Cloud refused the token")
	case res.StatusCode == http.StatusForbidden:
		return fmt.Errorf("the token may not read %s: give it the read scopes", strings.Split(path, "?")[0])
	case res.StatusCode == http.StatusNotFound:
		return fmt.Errorf("source Cloud: GET %s: %w", path, errGone)
	case res.StatusCode >= 300:
		return fmt.Errorf("source Cloud: GET %s: %s", path, res.Status)
	}
	return json.Unmarshal(data, out)
}

// Fetch reads the account.
func (s *Source) Fetch() (*Bundle, error) {
	b := &Bundle{Server: s.Server, Fetched: time.Now().UTC()}
	var user struct {
		Email    string `json:"email"`
		Timezone string `json:"timezone"`
	}
	if err := s.get("users/current", &user); err != nil {
		return nil, err
	}
	b.Email, b.Timezone = user.Email, user.Timezone
	if err := s.get("locations?include=accessids,password", &b.Locations); err != nil {
		return nil, err
	}
	if err := s.get("accessids?include=password", &b.AccessIDs); err != nil {
		return nil, err
	}
	if err := s.get("iodevices?include=channels", &b.Devices); err != nil {
		return nil, err
	}
	var icons []struct {
		ID int `json:"id"`
	}
	if err := s.get("user-icons", &icons); err != nil {
		return nil, err
	}
	for _, i := range icons {
		var icon struct {
			UserIcon
			Function struct {
				Name string `json:"name"`
			} `json:"function"`
		}
		if err := s.get(fmt.Sprintf("user-icons/%d?include=images", i.ID), &icon); err != nil {
			return nil, err
		}
		icon.UserIcon.Function = icon.Function.Name
		b.UserIcons = append(b.UserIcons, icon.UserIcon)
	}
	var groups []ChannelGroup
	if err := s.get("channel-groups", &groups); err != nil {
		return nil, err
	}
	for _, g := range groups {
		var full struct {
			Channels []struct {
				ID int `json:"id"`
			} `json:"channels"`
		}
		if err := s.get(fmt.Sprintf("channel-groups/%d?include=channels", g.ID), &full); err != nil {
			return nil, err
		}
		for _, c := range full.Channels {
			g.ChannelIDs = append(g.ChannelIDs, c.ID)
		}
		b.ChannelGroups = append(b.ChannelGroups, g)
	}
	var scenes []Scene
	if err := s.get("scenes", &scenes); err != nil {
		return nil, err
	}
	for _, sc := range scenes {
		var full Scene
		if err := s.get(fmt.Sprintf("scenes/%d?include=operations", sc.ID), &full); err != nil {
			return nil, err
		}
		b.Scenes = append(b.Scenes, full)
	}
	for path, out := range map[string]any{"schedules": &b.Schedules, "reactions": &b.Reactions, "direct-links": &b.DirectLinks} {
		if err := s.get(path, out); err != nil {
			return nil, err
		}
	}
	return b, nil
}

// ChannelCount is the number of channels of all devices.
func (b *Bundle) ChannelCount() int {
	n := 0
	for _, d := range b.Devices {
		n += len(d.Channels)
	}
	return n
}

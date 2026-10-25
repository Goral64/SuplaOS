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

// Bundle is what is read from the source account.
type Bundle struct {
	Server  string    `json:"server"`
	Fetched time.Time `json:"fetched"`
	Email   string    `json:"email"`

	Locations []Location `json:"locations"`
	AccessIDs []AccessID `json:"accessIds"`
	Devices   []Device   `json:"devices"`
	// Not moved yet, only counted: groups, scenes, schedules and the rest.
	Other map[string]int `json:"other"`
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
	case res.StatusCode >= 300:
		return fmt.Errorf("source Cloud: GET %s: %s", path, res.Status)
	}
	return json.Unmarshal(data, out)
}

// Fetch reads the account.
func (s *Source) Fetch() (*Bundle, error) {
	b := &Bundle{Server: s.Server, Fetched: time.Now().UTC(), Other: map[string]int{}}
	var user struct {
		Email string `json:"email"`
	}
	if err := s.get("users/current", &user); err != nil {
		return nil, err
	}
	b.Email = user.Email
	if err := s.get("locations?include=accessids,password", &b.Locations); err != nil {
		return nil, err
	}
	if err := s.get("accessids?include=password", &b.AccessIDs); err != nil {
		return nil, err
	}
	if err := s.get("iodevices?include=channels", &b.Devices); err != nil {
		return nil, err
	}
	for name, path := range map[string]string{
		"channelGroups": "channel-groups", "scenes": "scenes", "schedules": "schedules",
		"directLinks": "direct-links",
	} {
		var list []json.RawMessage
		if err := s.get(path, &list); err == nil && len(list) > 0 {
			b.Other[name] = len(list)
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

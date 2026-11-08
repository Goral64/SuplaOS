package panel

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestPersonalToken(t *testing.T) {
	var deleted []string
	var made map[string]string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer webapp" {
			http.Error(w, "no token", http.StatusUnauthorized)
			return
		}
		switch r.Method + " " + r.URL.Path {
		case "GET /api/oauth-personal-tokens":
			w.Write([]byte(`[{"id":3,"name":"Node-RED (SuplaOS)"},{"id":4,"name":"mine"},{"id":7,"name":"Node-RED (SuplaOS)"}]`))
		case "DELETE /api/oauth-personal-tokens/3", "DELETE /api/oauth-personal-tokens/7":
			deleted = append(deleted, r.URL.Path)
			w.WriteHeader(http.StatusNoContent)
		case "POST /api/oauth-personal-tokens":
			json.NewDecoder(r.Body).Decode(&made)
			w.WriteHeader(http.StatusCreated)
			w.Write([]byte(`{"id":8,"name":"Node-RED (SuplaOS)","token":"abc.aHR0cHM6Ly9leGFtcGxl"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	c := &cloudAPI{url: srv.URL, client: srv.Client(), token: "webapp"}
	token, n, err := c.personalToken(nodeRedTokenName, nodeRedTokenScope)
	if err != nil {
		t.Fatal(err)
	}
	if token != "abc.aHR0cHM6Ly9leGFtcGxl" || n != 2 || len(deleted) != 2 {
		t.Fatalf("token %q, deleted %d %v", token, n, deleted)
	}
	if made["name"] != nodeRedTokenName || made["scope"] != nodeRedTokenScope {
		t.Fatalf("made %v", made)
	}
}

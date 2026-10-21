// Command suplaos-panel is the web panel of SuplaOS.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Goral64/SuplaOS/internal/panel"
	"github.com/Goral64/SuplaOS/web"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	listen := flag.String("listen", ":80", "address to listen on")
	settings := flag.String("settings", "/etc/suplaos/panel.yaml", "panel settings file")
	sessions := flag.String("sessions", "/var/lib/suplaos/panel-sessions.json", "login sessions file")
	flag.Parse()
	log.SetFlags(0)

	s := panel.New(panel.Config{
		Root:     getenv("SUPLAOS_ROOT", "/srv/suplaos"),
		Addons:   getenv("SUPLAOS_ADDONS", "/usr/share/suplaos/addons"),
		Settings: *settings,
		Sessions: *sessions,
		Web:      web.Files(),
	})
	go s.WatchImages()
	srv := &http.Server{
		Addr:              *listen,
		Handler:           s.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s", *listen)
	log.Fatal(srv.ListenAndServe())
}

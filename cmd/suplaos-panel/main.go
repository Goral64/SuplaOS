// Command suplaos-panel is the web panel of SuplaOS.
package main

import (
	"flag"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/Goral64/SuplaOS/internal/panel"
	"github.com/Goral64/SuplaOS/internal/system"
	"github.com/Goral64/SuplaOS/web"
)

func getenv(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	listen := flag.String("listen", ":"+panel.Port, "HTTPS address to listen on")
	redirect := flag.String("redirect", ":80", "HTTP address redirecting to HTTPS, empty: none")
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
	tlsConfig, err := s.TLSConfig()
	if err != nil {
		log.Fatal(err)
	}
	go s.WatchImages()
	go s.WatchCertificates()
	go s.WatchBackups()
	go s.WatchFan()
	go s.WatchMigration()
	go func() {
		if err := system.ApplyWifiCountry(); err != nil {
			log.Printf("wifi country: %v", err)
		}
	}()
	go system.WatchHotspot()
	go s.WatchProxies(tlsConfig)
	go s.ApplyPending()
	if *redirect != "" {
		go func() {
			srv := &http.Server{Addr: *redirect, Handler: panel.Redirect(), ReadHeaderTimeout: 10 * time.Second}
			log.Fatal(srv.ListenAndServe())
		}()
	}
	srv := &http.Server{
		Addr:              *listen,
		Handler:           s.Handler(),
		TLSConfig:         tlsConfig,
		ReadHeaderTimeout: 10 * time.Second,
	}
	log.Printf("listening on %s", *listen)
	log.Fatal(srv.ListenAndServeTLS("", ""))
}

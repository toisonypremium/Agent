// web-console is deliberately separate from the scheduler. It exposes only the
// typed read-model API; it does not import exchange execution paths.
package main

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"time"

	"btc-agent/internal/storage"
	"btc-agent/internal/webconsole"
)

func main() {
	path := os.Getenv("BTC_AGENT_WEB_DB_PATH")
	if path == "" {
		log.Fatal("BTC_AGENT_WEB_DB_PATH required")
	}
	addr := os.Getenv("BTC_AGENT_WEB_LISTEN_ADDR")
	if addr == "" {
		addr = "127.0.0.1:8787"
	}
	addr, err := loopbackTCPAddr(addr)
	if err != nil {
		log.Fatal(err)
	}
	db, err := storage.OpenReadOnly(path)
	if err != nil {
		log.Fatal(err)
	}
	defer db.Close()
	haltDB, err := storage.OpenWritableExisting(path)
	if err != nil {
		log.Fatal(err)
	}
	defer haltDB.Close()
	service, err := webconsole.NewService(db, time.Now)
	if err != nil {
		log.Fatal(err)
	}
	service.SetHaltDB(haltDB)
	api := webconsole.NewAPI(service, time.Now)
	if err := api.ConfigureAccess(os.Getenv("BTC_AGENT_CF_ACCESS_TEAM_DOMAIN"), os.Getenv("BTC_AGENT_CF_ACCESS_AUD"), os.Getenv("BTC_AGENT_WEB_PUBLIC_ORIGIN")); err != nil {
		log.Fatal(err)
	}
	staticDir := os.Getenv("BTC_AGENT_WEB_STATIC_DIR")
	server := &http.Server{Addr: addr, Handler: api.App(staticDir), ReadHeaderTimeout: 5 * time.Second, IdleTimeout: 60 * time.Second, BaseContext: func(net.Listener) context.Context { return context.Background() }}
	fmt.Printf("web-console read-only listening on %s\n", addr)
	log.Fatal(server.ListenAndServe())
}

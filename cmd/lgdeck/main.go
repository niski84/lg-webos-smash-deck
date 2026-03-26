package main

import (
	"context"
	"fmt"
	"io/fs"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/niski84/lg-webos-smash-deck/internal/lgdeck"
	lgweb "github.com/niski84/lg-webos-smash-deck/web"
)

func main() {
	dataDir := lgdeck.DataDir()
	settingsPath := lgdeck.DefaultSettingsPath()

	cfg := lgdeck.LoadAppConfig(settingsPath)
	port := cfg.Port
	if port == "" {
		port = "8088"
	}

	log.Printf("[lgdeck] data directory : %s", dataDir)
	log.Printf("[lgdeck] settings file  : %s", settingsPath)
	if cfg.TVIP != "" {
		log.Printf("[lgdeck] tv ip          : %s", cfg.TVIP)
	} else {
		log.Printf("[lgdeck] tv ip          : (not configured — use the Settings dialog)")
	}

	// Strip the "lgdeck/" prefix so index.html is served at /.
	webFS, err := fs.Sub(lgweb.FS, "lgdeck")
	if err != nil {
		fmt.Fprintf(os.Stderr, "embed FS error: %v\n", err)
		os.Exit(1)
	}

	srv := lgdeck.NewHTTPServer(cfg)
	handler := srv.Routes(webFS)

	httpSrv := &http.Server{
		Addr:         net.JoinHostPort("", port),
		Handler:      handler,
		ReadTimeout:  30 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  90 * time.Second,
	}

	go func() {
		log.Printf("[lgdeck] listening on :%s  →  http://localhost:%s", port, port)
		if err := httpSrv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			fmt.Fprintf(os.Stderr, "server error: %v\n", err)
			os.Exit(1)
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = httpSrv.Shutdown(ctx)
	log.Println("[lgdeck] shutdown complete")
}

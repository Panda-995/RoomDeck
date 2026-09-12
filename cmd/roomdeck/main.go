package main

import (
	"context"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"roomdeck/internal/server"
)

func main() {
	health := flag.Bool("healthcheck", false, "Check local HTTP readiness")
	flag.Parse()
	addr := env("LISTEN_ADDR", ":8080")
	if *health {
		client := http.Client{Timeout: 3 * time.Second}
		res, err := client.Get("http://127.0.0.1:8080/readyz")
		if err != nil || res.StatusCode != 200 {
			os.Exit(1)
		}
		res.Body.Close()
		return
	}
	app, err := server.New(server.Config{DataDir: env("DATA_DIR", "data"), WebDir: env("WEB_DIR", "web/dist"), BaseURL: os.Getenv("BASE_URL"), MediaURL: os.Getenv("LIVEKIT_INTERNAL_URL"), MediaKey: os.Getenv("LIVEKIT_API_KEY"), MediaSecret: os.Getenv("LIVEKIT_API_SECRET")})
	if err != nil {
		log.Fatal(err)
	}
	defer app.Close()
	httpServer := &http.Server{Addr: addr, Handler: app.Handler(), ReadHeaderTimeout: 10 * time.Second, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	go func() {
		log.Printf("RoomDeck listening on %s", addr)
		if err := httpServer.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatal(err)
		}
	}()
	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	_ = httpServer.Shutdown(ctx)
}
func env(key, fallback string) string {
	if value := os.Getenv(key); value != "" {
		return value
	}
	return fallback
}

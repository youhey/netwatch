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

	"github.com/youhey/netwatch/internal/api"
	"github.com/youhey/netwatch/internal/collector"
	"github.com/youhey/netwatch/internal/config"
	"github.com/youhey/netwatch/internal/probe"
	"github.com/youhey/netwatch/internal/storage"
)

var version = "dev"

func main() {
	configPath := flag.String("config", "configs/netwatch.example.json", "path to netwatch config JSON")
	flag.Parse()

	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("load config failed: %v", err)
	}

	jsonl := storage.NewJSONL(cfg.DataPath)
	state := collector.NewState()

	samples, err := jsonl.Load()
	if err != nil {
		log.Fatalf("load samples failed: %v", err)
	}
	state.Load(samples)
	log.Printf("loaded %d samples", len(samples))

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	c := collector.New(cfg, probe.Fping{}, probe.DNS{}, probe.HTTP{}, jsonl, state)
	go c.Run(rootCtx)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.New(state, version).Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("http server failed: %v", err)
		}
	}()

	<-rootCtx.Done()
	log.Print("shutdown requested")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := server.Shutdown(shutdownCtx); err != nil {
		log.Printf("http shutdown failed: %v", err)
	}
}

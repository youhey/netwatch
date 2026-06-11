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
	"github.com/youhey/netwatch/internal/speedprobe"
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
	if cfg.DataDir != "" {
		jsonl = storage.NewRotatingJSONL(cfg.DataDir, cfg.DataFilePattern, cfg.RetentionDays)
	}
	state := collector.NewState()

	samples, err := jsonl.Load()
	if err != nil {
		log.Fatalf("load samples failed: %v", err)
	}
	state.Load(samples)
	log.Printf("loaded %d samples", len(samples))

	rootCtx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	httpProbe := probe.NewHTTP(cfg.HTTPDisableKeepAlive, cfg.HTTPMaxBodyBytes)
	downloadProbe := probe.NewDownload()
	statusPageProbe := probe.NewStatusPage()
	speedprobeClient := speedprobe.NewClient()
	c := collector.New(cfg, probe.Fping{}, probe.DNS{}, httpProbe, downloadProbe, jsonl, state, statusPageProbe).WithSpeedprobe(speedprobeClient)
	go c.Run(rootCtx)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           api.New(state, version, cfg.Targets).WithDownloadProbes(cfg.EnabledDownloadProbes()).WithRemoteSpeedProbes(cfg.EnabledRemoteSpeedProbes()).WithStatusPages(cfg.StatusPages).WithMonitoringThresholds(cfg.MonitoringThresholds).WithExportStorage(cfg.DataPath, cfg.DataDir, cfg.DataFilePattern).Routes(),
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

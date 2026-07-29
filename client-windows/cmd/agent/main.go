// Command agent is the Windows reporter client for cyberstalk-me.
//
// It collects the foreground window, idle time, battery and network type from
// Win32, sanitizes them through the mapping rules, and posts the result to the
// server at a fixed interval. The raw window title never leaves the process:
// collect exposes it only as a lazy getter that mapping calls solely when a
// rule needs it, and the report payload carries only already-sanitized fields.
//
// Flags:
//
//	-config <path>   config file (default: config.yaml next to the executable)
//	-dry-run         print the sanitized payload once and exit, no network
//	-v               debug logging
package main

import (
	"context"
	"encoding/json"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	"cyberstalk.me/client-windows/internal/collect"
	"cyberstalk.me/client-windows/internal/config"
	"cyberstalk.me/client-windows/internal/mapping"
	"cyberstalk.me/client-windows/internal/report"
	"cyberstalk.me/shared"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (default: next to the executable)")
	dryRun := flag.Bool("dry-run", false, "collect and map one cycle, print the sanitized payload, and exit without reporting")
	verbose := flag.Bool("v", false, "verbose (debug) logging")
	flag.Parse()

	level := slog.LevelInfo
	if *verbose {
		level = slog.LevelDebug
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: level})))

	if *configPath == "" {
		p, err := config.DefaultPath()
		if err != nil {
			fatal("locate default config: %v", err)
		}
		*configPath = p
	}
	cfg, err := config.Load(*configPath)
	if err != nil {
		fatal("load config: %v", err)
	}
	mapper, err := mapping.New(cfg.MapperOptions())
	if err != nil {
		fatal("build mapper: %v", err)
	}

	// next produces one already-sanitized payload. collect -> mapping happens
	// here, in main, so the report package never imports collect or mapping.
	// The raw title is reachable only through snap.Title, which mapping calls
	// lazily and only when a rule needs it.
	next := func() shared.ReportPayload {
		snap := collect.Collect()
		act := mapper.Resolve(snap.Process, snap.Title, snap.IdleSeconds)
		// "" means the network lookup failed (unknown); report it as nil so the
		// site shows nothing rather than a wrong guess. "offline" is a real
		// state and is sent through.
		var net *shared.NetworkType
		if snap.Network != "" {
			n := snap.Network
			net = &n
		}
		return shared.ReportPayload{
			DeviceID:   cfg.DeviceID,
			DeviceName: cfg.DeviceName,
			DeviceType: "windows",
			Activity:   act,
			Battery:    snap.Battery,
			Network:    net,
		}
	}

	if *dryRun {
		p := next()
		p.ReportedAt = time.Now().UTC()
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		if err := enc.Encode(p); err != nil {
			fatal("encode dry-run payload: %v", err)
		}
		return
	}

	slog.Info("agent starting",
		"server_url", cfg.ServerURL,
		"interval", cfg.Interval,
		"rules", len(cfg.Rules),
		"expose_title", len(cfg.ExposeTitle),
	)
	if len(cfg.ExposeTitle) > 0 {
		slog.Warn("expose_title is non-empty: the raw window titles of these processes will be reported verbatim",
			"processes", cfg.ExposeTitle)
	}

	client := report.NewClient(cfg.ReportURL(), cfg.Token)
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	loop := &report.Loop{
		Client:   client,
		Interval: cfg.Interval,
		Next:     next,
	}
	if err := loop.Run(ctx); err != nil {
		slog.Info("agent stopped", "reason", err)
	}
}

// fatal logs the error and exits 1. It is the only place the agent exits
// non-zero; panic/log.Fatal are forbidden elsewhere (see
// .trellis/spec/backend/quality-guidelines.md).
func fatal(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

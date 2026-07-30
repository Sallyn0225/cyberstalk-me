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
//	-setup           open the local configuration UI in a browser, then exit
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
	"cyberstalk.me/client-windows/internal/winsetup"
	"cyberstalk.me/shared"
)

func main() {
	configPath := flag.String("config", "", "path to config.yaml (default: next to the executable)")
	setupMode := flag.Bool("setup", false, "open the local configuration UI in a browser, write config.yaml, and exit without reporting")
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

	if *setupMode {
		// Both flags mean "do one thing and exit", but different things.
		// Silently picking one would leave the user guessing which.
		if *dryRun {
			fatal("-setup and -dry-run cannot be used together")
		}
		ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
		defer stop()
		// Note the config file is NOT loaded here: setup mode has to start even
		// when the file is missing or broken, because that is what it is for.
		// setupIndex serves the embedded UI; see webui.go.
		if err := winsetup.Run(ctx, winsetup.Options{
			ConfigPath: *configPath,
			Index:      setupIndex,
			Assets:     setupAssets(),
		}); err != nil {
			fatal("setup: %v", err)
		}
		return
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
		act := resolve(mapper, snap)
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

// resolve turns a raw snapshot into a sanitized activity for the reporting loop
// and -dry-run. The -setup preview reaches the same mapping.Resolve through
// setup.Server, from a mapper compiled out of the same rules, so the two cannot
// disagree about what a given window would be reported as.
//
// It lives in main because collect and mapping do not import each other: collect
// is Win32-only, mapping is pure, and this is the seam between them.
func resolve(m *mapping.Mapper, snap collect.Snapshot) shared.Activity {
	return m.Resolve(snap.Process, snap.Title, snap.IdleSeconds)
}

// fatal logs the error and exits 1. It is the only place the agent exits
// non-zero; panic/log.Fatal are forbidden elsewhere (see
// .trellis/spec/backend/quality-guidelines.md).
func fatal(format string, args ...any) {
	slog.Error(fmt.Sprintf(format, args...))
	os.Exit(1)
}

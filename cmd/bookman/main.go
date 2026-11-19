// Command bookman is a self-hosted server for PDF and comic libraries.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/n25tech/bookman/internal/api"
	"github.com/n25tech/bookman/internal/config"
	"github.com/n25tech/bookman/internal/db"
	"github.com/n25tech/bookman/internal/formats"
	"github.com/n25tech/bookman/internal/library"
	"github.com/n25tech/bookman/internal/thumbs"
	"github.com/n25tech/bookman/web"
)

var version = "dev"

func main() {
	configPath := flag.String("config", os.Getenv("BOOKMAN_CONFIG"), "path to bookman.toml (env BOOKMAN_CONFIG)")
	logLevel := flag.String("log-level", envOr("BOOKMAN_LOG_LEVEL", "info"), "debug, info, warn or error")
	showVersion := flag.Bool("version", false, "print the version and exit")
	flag.Parse()
	if *showVersion {
		fmt.Println(version)
		return
	}

	var level slog.Level
	if err := level.UnmarshalText([]byte(*logLevel)); err != nil {
		level = slog.LevelInfo
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stderr, &slog.HandlerOptions{Level: level})))

	if err := run(*configPath); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func envOr(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func run(configPath string) error {
	cfg, err := config.Load(configPath)
	if err != nil {
		return err
	}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	slog.Info("starting bookman", "version", version, "listen", cfg.Listen, "media_roots", strings.Join(cfg.MediaRoots, ", "))

	database, err := db.Open(ctx, cfg.DatabaseURL)
	if err != nil {
		return err
	}
	defer database.Close()

	formats.SetupPDF(cfg.ThumbWorkers)
	covers := thumbs.New(database, cfg.ThumbWorkers)
	scanner := library.NewScanner(database, covers)
	if err := scanner.Init(ctx, cfg.Libraries); err != nil {
		return fmt.Errorf("loading libraries: %w", err)
	}
	for _, l := range scanner.Libraries() {
		slog.Info("library", "name", l.Name, "path", l.RootPath, "skip", strings.Join(l.Exclude, ", "))
	}

	scanAll := func() {
		scanner.ScanAll(ctx)
		covers.EnqueueMissing(ctx)
	}

	srv := &api.Server{
		DB:      database,
		Scanner: scanner,
		Thumbs:  covers,
		Pages:   formats.NewPageCache(cfg.PageCacheMB << 20),
		Web:     web.Dist(),
		ScanAll: scanAll,
		ScanLibrary: func(id int64) {
			if err := scanner.ScanLibrary(ctx, id); err == nil {
				covers.EnqueueMissing(ctx)
			}
		},
		MediaRoots: cfg.MediaRoots,
	}
	httpSrv := &http.Server{
		Addr:              cfg.Listen,
		Handler:           srv.Handler(),
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	// Background work: covers, the startup scan, watching and periodic rescans.
	go covers.Run(ctx)
	go func() {
		scanAll()
		if cfg.Watch {
			go func() {
				if err := scanner.Watch(ctx); err != nil {
					slog.Error("watcher stopped", "err", err)
				}
			}()
		}
		if cfg.RescanInterval.Duration <= 0 {
			return
		}
		t := time.NewTicker(cfg.RescanInterval.Duration)
		defer t.Stop()
		for {
			select {
			case <-ctx.Done():
				return
			case <-t.C:
				scanAll()
				database.DeleteExpiredSessions(ctx)
			}
		}
	}()

	errc := make(chan error, 1)
	go func() {
		slog.Info("listening", "addr", cfg.Listen)
		errc <- httpSrv.ListenAndServe()
	}()
	select {
	case err := <-errc:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	slog.Info("shutting down")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}

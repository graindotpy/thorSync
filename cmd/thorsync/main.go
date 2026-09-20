package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"

	"github.com/apgul/thorsync/internal/api"
	"github.com/apgul/thorsync/internal/archive"
	"github.com/apgul/thorsync/internal/auth"
	"github.com/apgul/thorsync/internal/broker"
	"github.com/apgul/thorsync/internal/config"
	"github.com/apgul/thorsync/internal/events"
	"github.com/apgul/thorsync/internal/store"
	"github.com/apgul/thorsync/internal/syncthing"
	"github.com/apgul/thorsync/internal/webui"
)

var (
	version = "dev"
	commit  = "unknown"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	slog.SetDefault(logger)
	if err := run(); err != nil {
		slog.Error("ThorSync stopped", "error", err)
		os.Exit(1)
	}
}

func run() error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	for _, dir := range []string{cfg.DataDir, cfg.ArchiveDir, cfg.ThorDir, cfg.WindowsDir, cfg.ArtworkDir()} {
		if err := os.MkdirAll(dir, 0o750); err != nil {
			return err
		}
		probe := filepath.Join(dir, ".thorsync-write-probe")
		if err := os.WriteFile(probe, []byte("ok"), 0o600); err != nil {
			return errors.New("required directory is not writable: " + dir)
		}
		if err := os.Remove(probe); err != nil {
			return errors.New("required directory cannot remove probe file: " + dir)
		}
	}
	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()
	db, err := store.Open(ctx, cfg)
	if err != nil {
		return err
	}
	defer db.Close()
	archiveStore := archive.New(cfg.ArchiveDir, cfg.SoftQuotaBytes, cfg.ReserveBytes)
	if err := archiveStore.Ensure(); err != nil {
		return err
	}
	hub := events.New()
	syncClient := syncthing.New(cfg.SyncthingURL, cfg.SyncthingAPIKey)
	brokerService := broker.New(db, archiveStore, hub)
	defer brokerService.Close()
	runner := syncthing.NewRunner(syncClient, db, brokerService, hub, []syncthing.EndpointConfig{{ID: "thor", FolderID: cfg.SyncthingThorFolderID, Root: cfg.ThorDir}, {ID: "windows", FolderID: cfg.SyncthingWindowsFolderID, Root: cfg.WindowsDir}}, cfg.ReconcileInterval)
	runner.Run(ctx)

	var authenticator *auth.Authenticator
	if cfg.AuthMode == "cloudflare" {
		authenticator, err = auth.New(auth.Config{Issuer: cfg.CloudflareIssuer, Audience: cfg.CloudflareAudience, AdminEmail: cfg.AdminEmail, JWKSURL: cfg.CloudflareJWKSURL, HealthPaths: []string{"/health/live", "/health/ready"}})
		if err != nil {
			return err
		}
	}
	apiHandler := api.New(db, archiveStore, brokerService, syncClient, hub, cfg.ArtworkDir(), cfg.SyncthingThorFolderID, cfg.SyncthingWindowsFolderID, []string{cfg.DataDir, cfg.ArchiveDir, cfg.ThorDir, cfg.WindowsDir})
	if authenticator != nil {
		apiHandler.SetAuthenticationCheck(authenticator.Health)
	}
	apiRoutes := apiHandler.Routes()
	root := http.NewServeMux()
	root.Handle("/health/", apiRoutes)
	root.Handle("/api/", apiRoutes)
	root.Handle("/artwork/", http.StripPrefix("/artwork/", http.FileServer(http.Dir(cfg.ArtworkDir()))))
	root.Handle("/", webui.Handler())
	var handler http.Handler = auth.SameOriginMutations(securityHeaders(root))
	if authenticator != nil {
		handler = authenticator.Middleware(handler)
	} else {
		slog.Warn("Cloudflare authentication is disabled; use only for local development")
	}

	server := &http.Server{Addr: cfg.ListenAddr, Handler: handler, ReadHeaderTimeout: 10 * time.Second, ReadTimeout: 30 * time.Second, WriteTimeout: 0, IdleTimeout: 90 * time.Second, MaxHeaderBytes: 1 << 20}
	errCh := make(chan error, 1)
	go func() {
		slog.Info("ThorSync started", "address", cfg.ListenAddr, "version", version, "commit", commit)
		errCh <- server.ListenAndServe()
	}()
	select {
	case <-ctx.Done():
		shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer shutdownCancel()
		return server.Shutdown(shutdownCtx)
	case err := <-errCh:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	}
}

func securityHeaders(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Permissions-Policy", "camera=(), microphone=(), geolocation=()")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; img-src 'self' data:; style-src 'self' 'unsafe-inline'; script-src 'self'; connect-src 'self'; object-src 'none'; base-uri 'self'; frame-ancestors 'none'")
		next.ServeHTTP(w, r)
	})
}

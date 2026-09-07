// Datei startet die Bearstack-Anwendung, laedt die Konfiguration und verdrahtet Repository, Services und HTTP-Server.
package main

import (
	"context"
	"crypto/tls"
	"errors"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"bearstack/internal/config"
	"bearstack/internal/repository"
	"bearstack/internal/server"
	"bearstack/internal/storage"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelInfo}))
	if err := run(logger); err != nil {
		logger.Error("startup failed", "error", err)
		os.Exit(1)
	}
}

const shutdownTimeout = 60 * time.Second

func run(logger *slog.Logger) (runErr error) {
	cfg, err := config.Load()
	if err != nil {
		return err
	}

	store, err := storage.New(cfg.StorageDir)
	if err != nil {
		return err
	}

	ctx := context.Background()
	repo, err := repository.Open(ctx, cfg.DBPath)
	if err != nil {
		return err
	}
	closeRepository := true
	defer func() {
		if closeRepository {
			runErr = errors.Join(runErr, repo.Close())
		}
	}()

	app, err := server.New(cfg, repo, store, logger)
	if err != nil {
		return err
	}
	appCtx, cancelApp := context.WithCancel(context.Background())
	var httpServer, redirectServer *http.Server
	var samePortMux *tlsHTTPMux
	defer func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), shutdownTimeout)
		defer cancel()
		cancelApp()
		app.StopBackgroundJobs()
		var httpErr error
		for _, srv := range []*http.Server{redirectServer, httpServer} {
			if srv == nil {
				continue
			}
			if err := srv.Shutdown(shutdownCtx); err != nil {
				httpErr = errors.Join(httpErr, err)
				_ = srv.Close()
			}
		}
		if samePortMux != nil {
			httpErr = errors.Join(httpErr, samePortMux.Close())
		}
		if httpErr != nil {
			closeRepository = false
			runErr = errors.Join(runErr, httpErr, app.WaitBackgroundJobs(shutdownCtx))
			return
		}
		if err := app.Shutdown(shutdownCtx); err != nil {
			closeRepository = false
			runErr = errors.Join(runErr, err)
		}
	}()
	tlsCertFile, tlsKeyFile := "", ""
	if cfg.TLS.Enabled {
		tlsCertFile, tlsKeyFile, err = tlsCertificateFiles(cfg)
		if err != nil {
			return err
		}
	}

	httpServer = &http.Server{
		Addr:              cfg.Addr,
		Handler:           app.Handler(),
		TLSConfig:         serverTLSConfig(),
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       2 * time.Minute,
		WriteTimeout:      10 * time.Minute,
		IdleTimeout:       2 * time.Minute,
	}
	if cfg.TLS.Enabled {
		samePortMux, err = newTLSHTTPMux(cfg.Addr)
		if err != nil {
			return err
		}
		redirectServer = &http.Server{
			Handler:           httpToHTTPSRedirectHandler(cfg.Addr),
			ReadHeaderTimeout: 5 * time.Second,
			ReadTimeout:       30 * time.Second,
			WriteTimeout:      30 * time.Second,
			IdleTimeout:       2 * time.Minute,
		}
	}

	app.BackgroundWorkers().Start(appCtx)

	errs := make(chan error, 1)
	go func() {
		scheme := "http"
		if cfg.TLS.Enabled {
			scheme = "https"
		}
		logger.Info("BearStack listening", "scheme", scheme, "addr", cfg.Addr, "db_path", cfg.DBPath, "storage_dir", store.Root(), "max_upload_bytes", cfg.MaxUploadBytes, "auth_enabled", app.AuthEnabled(), "tls_enabled", cfg.TLS.Enabled, "photos_enabled", cfg.Photos.Active(), "photos_dir", cfg.Photos.RootDir, "photos_data_dir", cfg.Photos.DataDir, "photos_db_path", cfg.Photos.DBPath)
		if cfg.TLS.Enabled {
			logger.Info("HTTP redirect listening", "scheme", "http", "addr", cfg.Addr, "redirect_to", cfg.Addr, "same_port", true)
		}
		if cfg.TLS.Enabled {
			errs <- httpServer.ServeTLS(samePortMux.TLSListener(), tlsCertFile, tlsKeyFile)
			return
		}
		errs <- httpServer.ListenAndServe()
	}()
	if redirectServer != nil {
		go func() {
			err := redirectServer.Serve(samePortMux.HTTPListener())
			if err != nil && !errors.Is(err, http.ErrServerClosed) && !errors.Is(err, net.ErrClosed) {
				logger.Warn("HTTP redirect listener stopped", "addr", cfg.Addr, "error", err)
			}
		}()
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	defer signal.Stop(stop)

	select {
	case sig := <-stop:
		logger.Info("shutdown requested", "signal", sig.String())
	case err := <-errs:
		if !errors.Is(err, http.ErrServerClosed) {
			cancelApp()
			return err
		}
		return nil
	}

	return nil
}

func serverTLSConfig() *tls.Config {
	return &tls.Config{MinVersion: tls.VersionTLS12}
}

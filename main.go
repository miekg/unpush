package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	logLevel := slog.LevelInfo
	if v := os.Getenv("LOG_LEVEL"); v != "" {
		if err := logLevel.UnmarshalText([]byte(v)); err != nil {
			slog.Error("Invalid LOG_LEVEL", "value", v)
			os.Exit(1)
		}
	}
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: logLevel,
	})))
	slog.Info("Log level set", "level", logLevel)

	cfg, err := loadAppConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	db, err := openState(cfg.StateDB)
	if err != nil {
		slog.Error("Failed to open state database", "error", err, "path", cfg.StateDB)
		os.Exit(1)
	}
	defer db.Close()
	slog.Info("State database opened", "path", cfg.StateDB)

	mux := http.NewServeMux()

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

	for _, t := range cfg.Targets {
		d := newDeployer(t, db)
		if t.PollInterval != "" {
			interval, _ := time.ParseDuration(t.PollInterval) // already validated by loadFileConfig
			slog.Info("Registered poll target", "name", t.Name, "interval", interval, "branch", t.Branch, "compose_file", t.ComposeFile)
			go d.startPoller(ctx)
		} else {
			if t.WebhookSecret == "" {
				slog.Warn("Target has no webhook_secret, signatures will not be verified", "target", t.Name)
			}
			path := "/webhook/" + t.Name
			slog.Info("Registered webhook target", "name", t.Name, "path", path, "branch", t.Branch, "compose_file", t.ComposeFile)
			mux.HandleFunc("POST "+path, d.handleWebhook)
		}
	}

	mux.HandleFunc("GET /healthz", handleHealthz)

	srv := &http.Server{
		Addr:         cfg.ListenAddr,
		Handler:      mux,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 10 * time.Second,
	}

	go func() {
		slog.Info("Starting webhook server", "addr", cfg.ListenAddr)
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			slog.Error("Server failed", "error", err)
			os.Exit(1)
		}
	}()

	<-ctx.Done()
	slog.Info("Shutting down...")
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer shutdownCancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		slog.Error("Shutdown error", "error", err)
	}
}

func handleHealthz(w http.ResponseWriter, _ *http.Request) {
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write([]byte("ok"))
}

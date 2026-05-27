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
	slog.SetDefault(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	})))

	cfg, err := loadAppConfig()
	if err != nil {
		slog.Error("Failed to load config", "error", err)
		os.Exit(1)
	}

	mux := http.NewServeMux()

	for _, t := range cfg.Targets {
		d := newDeployer(t)
		if t.Name == "" {
			// Single-target env-var mode: register at /webhook for backward compatibility.
			if t.WebhookSecret == "" {
				slog.Warn("DEPLOYER_WEBHOOK_SECRET is not set, webhook signatures will not be verified")
			}
			slog.Info("Registered target", "path", "/webhook", "branch", t.Branch, "compose_file", t.ComposeFile)
			mux.HandleFunc("POST /webhook", d.handleWebhook)
		} else {
			if t.WebhookSecret == "" {
				slog.Warn("Target has no webhook_secret, signatures will not be verified", "target", t.Name)
			}
			path := "/webhook/" + t.Name
			slog.Info("Registered target", "name", t.Name, "path", path, "branch", t.Branch, "compose_file", t.ComposeFile)
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

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer cancel()

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

package main

import (
	"context"
	"database/sql"
	"log/slog"
	"os/exec"
	"path/filepath"
	"time"
)

type Deployer struct {
	cfg   TargetConfig
	db    *sql.DB
	queue chan pushEvent
}

func newDeployer(cfg TargetConfig, db *sql.DB) *Deployer {
	d := &Deployer{
		cfg:   cfg,
		db:    db,
		queue: make(chan pushEvent, 1),
	}
	go d.deployLoop()
	return d
}

func (d *Deployer) triggerDeploy(event pushEvent) {
	select {
	case d.queue <- event:
	default:
		slog.Warn("Deploy already queued, dropping incoming event", "target", d.cfg.Name, "commit", shortCommit(event.HeadCommit.ID))
	}
}

func (d *Deployer) deployLoop() {
	for event := range d.queue {
		d.runDeploy(event)
	}
}

func (d *Deployer) runDeploy(event pushEvent) {
	commitID := event.HeadCommit.ID
	shortID := shortCommit(commitID)

	start := time.Now()
	succeeded := false
	defer func() {
		if err := recordDeploy(d.db, d.cfg.Name, commitID, start, succeeded); err != nil {
			slog.Warn("Failed to record deploy outcome", "target", d.cfg.Name, "commit", shortID, "error", err)
		}
	}()

	slog.Info("Starting deploy", "target", d.cfg.Name, "commit", shortID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	composeFile := d.cfg.ComposeFile
	if d.cfg.RepoURL != "" {
		if err := prepareRepo(ctx, d.cfg.WorkDir, d.cfg.RepoURL, d.cfg.RepoToken, event.HeadCommit.ID); err != nil {
			slog.Error("Failed to prepare repository", "target", d.cfg.Name, "error", err, "commit", shortID)
			return
		}
		if !filepath.IsAbs(composeFile) {
			composeFile = filepath.Join(d.cfg.WorkDir, composeFile)
		}
	}

	slog.Info("Executing deployment via uc deploy", "target", d.cfg.Name, "commit", shortID)

	args := []string{"deploy", "-f", composeFile}
	if d.cfg.ForceRecreate {
		args = append(args, "--recreate")
	}

	cmd := exec.CommandContext(ctx, "/uc", args...)
	output, err := cmd.CombinedOutput()
	if err != nil {
		slog.Error("Deployment failed", "target", d.cfg.Name, "error", err, "output", string(output), "commit", shortID)
		return
	}

	slog.Info("Deployment completed", "target", d.cfg.Name, "commit", shortID, "output", string(output), "duration", time.Since(start))
	succeeded = true
}

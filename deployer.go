package main

import (
	"context"
	"database/sql"
	"log/slog"
	"path/filepath"
	"time"

	composecli "github.com/compose-spec/compose-go/v2/cli"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
	"github.com/psviderski/uncloud/pkg/client/connector"
	"github.com/psviderski/uncloud/pkg/client/deploy"
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

	slog.Info("Loading compose file", "target", d.cfg.Name, "commit", shortID, "file", composeFile)

	conn := connector.NewUnixConnector(d.cfg.SocketPath)
	cli, err := client.New(ctx, conn)
	if err != nil {
		slog.Error("Failed to connect to uncloud socket", "target", d.cfg.Name, "error", err, "socket", d.cfg.SocketPath)
		return
	}
	defer cli.Close()

	project, err := compose.LoadProject(ctx, []string{composeFile}, composecli.WithOsEnv)
	if err != nil {
		slog.Error("Failed to load compose file", "target", d.cfg.Name, "error", err, "file", composeFile)
		return
	}

	if d.cfg.RepoURL != "" {
		slog.Info("Building and pushing images", "target", d.cfg.Name, "commit", shortID)
		if err := buildAndPush(ctx, project, cli); err != nil {
			slog.Error("Failed to build and push images", "target", d.cfg.Name, "error", err, "commit", shortID)
			return
		}
	}

	strategy := &deploy.RollingStrategy{
		ForceRecreate: d.cfg.ForceRecreate,
	}
	deployment, err := compose.NewDeploymentWithStrategy(ctx, cli, project, strategy)
	if err != nil {
		slog.Error("Failed to create deployment", "target", d.cfg.Name, "error", err)
		return
	}

	plan, err := deployment.Plan(ctx)
	if err != nil {
		slog.Error("Failed to plan deployment", "target", d.cfg.Name, "error", err)
		return
	}

	if plan.IsEmpty() {
		succeeded = true
		slog.Info("Services are up to date, nothing to deploy", "target", d.cfg.Name, "commit", shortID)
		return
	}

	slog.Info("Executing deployment plan", "target", d.cfg.Name, "commit", shortID)
	if err := plan.Execute(ctx, cli); err != nil {
		slog.Error("Deployment failed", "target", d.cfg.Name, "error", err, "duration", time.Since(start))
		return
	}

	succeeded = true
	slog.Info("Deployment completed", "target", d.cfg.Name, "commit", shortID, "duration", time.Since(start))
}

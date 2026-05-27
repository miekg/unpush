package main

import (
	"context"
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
	queue chan pushEvent
}

func newDeployer(cfg TargetConfig) *Deployer {
	d := &Deployer{
		cfg:   cfg,
		queue: make(chan pushEvent, 1),
	}
	go d.deployLoop()
	return d
}

func (d *Deployer) triggerDeploy(event pushEvent) {
	select {
	case d.queue <- event:
	default:
		commitID := event.HeadCommit.ID
		if len(commitID) > 8 {
			commitID = commitID[:8]
		}
		slog.Warn("Deploy already queued, dropping incoming event", "commit", commitID)
	}
}

func (d *Deployer) deployLoop() {
	for event := range d.queue {
		d.runDeploy(event)
	}
}

func (d *Deployer) runDeploy(event pushEvent) {
	commitID := event.HeadCommit.ID
	if len(commitID) > 8 {
		commitID = commitID[:8]
	}

	start := time.Now()
	slog.Info("Starting deploy", "commit", commitID)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Minute)
	defer cancel()

	composeFile := d.cfg.ComposeFile
	if d.cfg.RepoURL != "" {
		if err := prepareRepo(ctx, d.cfg.WorkDir, d.cfg.RepoURL, d.cfg.RepoToken, event.HeadCommit.ID); err != nil {
			slog.Error("Failed to prepare repository", "error", err, "commit", commitID)
			return
		}
		if !filepath.IsAbs(composeFile) {
			composeFile = filepath.Join(d.cfg.WorkDir, composeFile)
		}
	}

	slog.Info("Loading compose file", "commit", commitID, "file", composeFile)

	conn := connector.NewUnixConnector(d.cfg.SocketPath)
	cli, err := client.New(ctx, conn)
	if err != nil {
		slog.Error("Failed to connect to uncloud socket", "error", err, "socket", d.cfg.SocketPath)
		return
	}
	defer cli.Close()

	project, err := compose.LoadProject(ctx, []string{composeFile}, composecli.WithOsEnv)
	if err != nil {
		slog.Error("Failed to load compose file", "error", err, "file", composeFile)
		return
	}

	if d.cfg.RepoURL != "" {
		if err := buildAndPush(ctx, project, cli); err != nil {
			slog.Error("Failed to build and push images", "error", err, "commit", commitID)
			return
		}
	}

	strategy := &deploy.RollingStrategy{
		ForceRecreate: d.cfg.ForceRecreate,
	}
	deployment, err := compose.NewDeploymentWithStrategy(ctx, cli, project, strategy)
	if err != nil {
		slog.Error("Failed to create deployment", "error", err)
		return
	}

	plan, err := deployment.Plan(ctx)
	if err != nil {
		slog.Error("Failed to plan deployment", "error", err)
		return
	}

	if plan.IsEmpty() {
		slog.Info("Services are up to date, nothing to deploy", "commit", commitID)
		return
	}

	slog.Info("Executing deployment plan", "commit", commitID)
	if err := plan.Execute(ctx, cli); err != nil {
		slog.Error("Deployment failed", "error", err, "duration", time.Since(start))
		return
	}

	slog.Info("Deployment completed", "commit", commitID, "duration", time.Since(start))
}

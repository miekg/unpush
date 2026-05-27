package main

import (
	"context"
	"fmt"
	"log/slog"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	composeapi "github.com/docker/compose/v2/pkg/api"
	composev2 "github.com/docker/compose/v2/pkg/compose"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
)

// buildAndPush builds images for all services with a build directive and pushes them to cluster machines.
// The push target for each service is determined by its x-machines extension; if absent, the image is
// pushed to all machines in the cluster.
//
// This replicates the logic from github.com/psviderski/uncloud/internal/cli.BuildServices because the
// internal package is not importable from outside the uncloud module.
// TODO(option 2): Once uncloud exposes a public pkg/ API for building (e.g. pkg/build), replace this
// with a call to that API. Track at https://github.com/psviderski/uncloud.
func buildAndPush(ctx context.Context, project *composetypes.Project, cli *client.Client) error {
	services := servicesThatNeedBuild(project)
	if len(services) == 0 {
		return nil
	}

	dockerCli, err := command.NewDockerCli()
	if err != nil {
		return fmt.Errorf("create docker client: %w", err)
	}
	if err = dockerCli.Initialize(flags.NewClientOptions()); err != nil {
		return fmt.Errorf("initialise docker client: %w", err)
	}

	names := make([]string, len(services))
	for i, s := range services {
		names[i] = s.Name
	}
	slog.Info("Building images", "services", names)

	composeService := composev2.NewComposeService(dockerCli)
	if err = composeService.Build(ctx, project, composeapi.BuildOptions{Deps: true}); err != nil {
		return fmt.Errorf("build images: %w", err)
	}

	for _, s := range services {
		if s.Image == "" {
			continue
		}
		var pushOpts client.PushImageOptions
		if machines, ok := s.Extensions[compose.MachinesExtensionKey].(compose.MachinesSource); ok {
			pushOpts.Machines = machines
		}
		if len(pushOpts.Machines) == 0 {
			pushOpts.AllMachines = true
		}
		slog.Info("Pushing image to cluster", "service", s.Name, "image", s.Image)
		if err = cli.PushImage(ctx, s.Image, pushOpts); err != nil {
			return fmt.Errorf("push image %q for service %q: %w", s.Image, s.Name, err)
		}
	}
	return nil
}

func servicesThatNeedBuild(project *composetypes.Project) []composetypes.ServiceConfig {
	var result []composetypes.ServiceConfig
	for _, s := range project.Services {
		if s.Build != nil {
			result = append(result, s)
		}
	}
	return result
}

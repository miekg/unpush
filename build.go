package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"os"
	"strconv"

	composetypes "github.com/compose-spec/compose-go/v2/types"
	"github.com/docker/cli/cli/command"
	"github.com/docker/cli/cli/flags"
	composeapi "github.com/docker/compose/v2/pkg/api"
	composev2 "github.com/docker/compose/v2/pkg/compose"
	"github.com/google/go-containerregistry/pkg/crane"
	"github.com/psviderski/uncloud/pkg/api"
	"github.com/psviderski/uncloud/pkg/client"
	"github.com/psviderski/uncloud/pkg/client/compose"
)

// unregistryPort is the port the embedded unregistry listens on for each cluster machine.
const unregistryPort = 5000

// buildAndPush builds images for all services with a build directive and pushes them to remote cluster machines.
// The local machine is skipped because the image was just built by its Docker daemon, so it is already present
// in the containerd store that the local unregistry serves from.
//
// For remote machines the image is pushed directly from this process over WireGuard using plain HTTP, which
// avoids the proxy-based mechanism in client.PushImage that requires the pushing process and the Docker daemon
// to share a network namespace.
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

	// Identify the local machine so we can skip pushing to it. The image was just built by its Docker
	// daemon and is already in the containerd store that the local unregistry serves from.
	localMachineID := ""
	if localInfo, lerr := cli.MachineClient.Inspect(ctx, nil); lerr == nil {
		localMachineID = localInfo.Id
	} else {
		slog.Warn("Could not determine local machine ID, will attempt push to all machines", "error", lerr)
	}

	for _, s := range services {
		if s.Image == "" {
			continue
		}

		var filter *api.MachineFilter
		if machines, ok := s.Extensions[compose.MachinesExtensionKey].(compose.MachinesSource); ok && len(machines) > 0 {
			filter = &api.MachineFilter{NamesOrIDs: machines}
		}
		members, err := cli.ListMachines(ctx, filter)
		if err != nil {
			return fmt.Errorf("list machines: %w", err)
		}

		// Check if there are any remote machines before doing the export.
		hasRemote := false
		for _, member := range members {
			if member.Machine.Id != localMachineID {
				hasRemote = true
				break
			}
		}
		if !hasRemote {
			slog.Info("No remote machines to push to, skipping push", "service", s.Name)
			continue
		}

		// Export image once from Docker daemon to a temp file, then push to each remote machine.
		tmpPath, err := saveImageToTempFile(ctx, dockerCli, s.Image)
		if err != nil {
			return fmt.Errorf("export image %q: %w", s.Image, err)
		}
		defer os.Remove(tmpPath)

		for _, member := range members {
			if member.Machine.Id == localMachineID {
				slog.Info("Skipping push to local machine (image already in containerd)", "service", s.Name, "machine", member.Machine.Name)
				continue
			}
			if member.Machine.Network == nil {
				slog.Warn("Machine has no network info, skipping push", "machine", member.Machine.Name)
				continue
			}

			subnet, err := member.Machine.Network.Subnet.ToPrefix()
			if err != nil {
				return fmt.Errorf("parse subnet for machine %q: %w", member.Machine.Name, err)
			}
			machineIP := subnet.Masked().Addr().Next()
			registryAddr := net.JoinHostPort(machineIP.String(), strconv.Itoa(unregistryPort))

			slog.Info("Pushing image to cluster", "service", s.Name, "image", s.Image, "machine", member.Machine.Name)
			if err := pushImageToRegistry(ctx, tmpPath, s.Image, registryAddr); err != nil {
				return fmt.Errorf("push image %q for service %q: %w", s.Image, s.Name, err)
			}
		}
	}
	return nil
}

// saveImageToTempFile exports a Docker image as a tar archive to a temporary file and returns its path.
// The caller is responsible for deleting the file when done.
func saveImageToTempFile(ctx context.Context, dockerCli *command.DockerCli, imageName string) (string, error) {
	reader, err := dockerCli.Client().ImageSave(ctx, []string{imageName})
	if err != nil {
		return "", fmt.Errorf("save image: %w", err)
	}
	defer reader.Close()

	tmpFile, err := os.CreateTemp("", "unpush-*.tar")
	if err != nil {
		return "", fmt.Errorf("create temp file: %w", err)
	}
	defer tmpFile.Close()

	if _, err = io.Copy(tmpFile, reader); err != nil {
		os.Remove(tmpFile.Name())
		return "", fmt.Errorf("write image tar: %w", err)
	}
	return tmpFile.Name(), nil
}

// pushImageToRegistry pushes an image from a tar archive to an OCI-compatible registry over plain HTTP.
// The destination reference is constructed as registryAddr/imageName.
func pushImageToRegistry(ctx context.Context, tarPath, imageName, registryAddr string) error {
	img, err := crane.Load(tarPath)
	if err != nil {
		return fmt.Errorf("load image from tar: %w", err)
	}
	destRef := registryAddr + "/" + imageName
	if err := crane.Push(img, destRef, crane.Insecure, crane.WithContext(ctx)); err != nil {
		return fmt.Errorf("push to registry: %w", err)
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

package main

import (
	"fmt"
	"os"
	"path/filepath"
	"time"

	"gopkg.in/yaml.v3"
)

// AppConfig is the top-level application configuration.
type AppConfig struct {
	ListenAddr string
	// StateDB is the path to the SQLite database that records every deploy attempt and its outcome.
	StateDB string
	Targets []TargetConfig
}

// TargetConfig holds the configuration for a single deploy target.
type TargetConfig struct {
	// Name identifies the target and is used in the webhook path (/webhook/<name>).
	Name          string `yaml:"name"`
	WebhookSecret string `yaml:"webhook_secret"`
	Branch        string `yaml:"branch"`
	// ComposeFile is the path to the Compose file to deploy. When RepoURL is set and ComposeFile is a
	// relative path, it is resolved relative to WorkDir.
	ComposeFile   string `yaml:"compose_file"`
	ForceRecreate bool   `yaml:"force_recreate"`
	// RepoURL is the HTTPS URL of the GitHub repository to clone and check out before deploying. When set,
	// the deployer clones the repository at the push commit and uses it as the build context for services
	// with a build directive. Requires the Docker socket to be mounted at /var/run/docker.sock.
	RepoURL string `yaml:"repo_url"`
	// RepoToken is a GitHub personal access token for cloning private repositories.
	// Requires at least Contents: read permission on the repository.
	RepoToken string `yaml:"repo_token"`
	// WorkDir is the local directory where the repository is cloned when RepoURL is set.
	WorkDir string `yaml:"work_dir"`
	// PollInterval enables poll mode: the deployer checks the remote branch HEAD at this interval and
	// triggers a deploy when a new commit is detected. Requires RepoURL. Uses Go duration format, e.g. "5m", "1h".
	PollInterval string `yaml:"poll_interval"`
	// EnableWebhook controls whether the /webhook/<name> HTTP endpoint is registered for this target.
	// Defaults to true. Set to false to disable it (requires poll_interval to be set).
	EnableWebhook *bool `yaml:"enable_webhook"`
	// PassEnv lists env var names to read from the deployer's own environment and inject into every
	// service at deploy time. Use this to supply secrets without storing them in the repo.
	PassEnv []string `yaml:"pass_env"`
	// SocketPath is the path to the Uncloud daemon Unix socket. Inherited from global config; not set
	// per-target in YAML.
	SocketPath string `yaml:"-"`
}

// fileConfig is the on-disk YAML structure.
type fileConfig struct {
	ListenAddr string         `yaml:"listen_addr"`
	SocketPath string         `yaml:"socket_path"`
	StateDB    string         `yaml:"state_db"`
	Targets    []TargetConfig `yaml:"targets"`
}

// loadAppConfig reads the YAML config file at the path given by UNPUSH_CONFIG,
// defaulting to /deploy/config.yaml.
func loadAppConfig() (AppConfig, error) {
	path := os.Getenv("UNPUSH_CONFIG")
	if path == "" {
		path = "/deploy/config.yaml"
	}
	return loadFileConfig(path)
}

func loadFileConfig(path string) (AppConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AppConfig{}, fmt.Errorf("read config file: %w", err)
	}

	var fc fileConfig
	if err := yaml.Unmarshal(data, &fc); err != nil {
		return AppConfig{}, fmt.Errorf("parse config file: %w", err)
	}

	if fc.ListenAddr == "" {
		fc.ListenAddr = ":8080"
	}
	if fc.SocketPath == "" {
		fc.SocketPath = "/run/uncloud/uncloud.sock"
	}
	if v := os.Getenv("UNPUSH_STATE_DB"); v != "" {
		fc.StateDB = v
	} else if fc.StateDB == "" {
		fc.StateDB = "/deploy/state.db"
	}

	globalRepoToken := os.Getenv("UNPUSH_REPO_TOKEN")

	seen := make(map[string]bool)
	for i := range fc.Targets {
		t := &fc.Targets[i]
		if t.Name == "" {
			return AppConfig{}, fmt.Errorf("target %d: name is required", i)
		}
		if seen[t.Name] {
			return AppConfig{}, fmt.Errorf("duplicate target name %q", t.Name)
		}
		seen[t.Name] = true

		if t.EnableWebhook == nil {
			b := true
			t.EnableWebhook = &b
		}
		if !*t.EnableWebhook && t.PollInterval == "" {
			return AppConfig{}, fmt.Errorf("target %q: enable_webhook: false requires poll_interval (target would have no trigger)", t.Name)
		}
		if t.PollInterval != "" && t.RepoURL == "" {
			return AppConfig{}, fmt.Errorf("target %q: poll_interval requires repo_url", t.Name)
		}
		if t.PollInterval != "" {
			if _, err := time.ParseDuration(t.PollInterval); err != nil {
				return AppConfig{}, fmt.Errorf("target %q: invalid poll_interval %q: %w", t.Name, t.PollInterval, err)
			}
		}

		if t.Branch == "" {
			t.Branch = "main"
		}
		if t.WorkDir == "" {
			t.WorkDir = filepath.Join("/deploy/work", t.Name)
		}
		if t.ComposeFile == "" {
			if t.RepoURL != "" {
				t.ComposeFile = "compose.yaml"
			} else {
				t.ComposeFile = "/deploy/compose.yaml"
			}
		}
		t.SocketPath = fc.SocketPath
		if t.RepoToken == "" {
			t.RepoToken = globalRepoToken
		}
	}

	return AppConfig{
		ListenAddr: fc.ListenAddr,
		StateDB:    fc.StateDB,
		Targets:    fc.Targets,
	}, nil
}

package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// AppConfig is the top-level application configuration.
type AppConfig struct {
	ListenAddr string
	Targets    []TargetConfig
}

// TargetConfig holds the configuration for a single deploy target.
type TargetConfig struct {
	// Name identifies the target and is used in the webhook path (/webhook/<name>).
	// Empty in single-target env-var mode, where the path is /webhook.
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
	// SocketPath is the path to the Uncloud daemon Unix socket. Inherited from global config; not set
	// per-target in YAML.
	SocketPath string `yaml:"-"`
}

// fileConfig is the on-disk YAML structure.
type fileConfig struct {
	ListenAddr string         `yaml:"listen_addr"`
	SocketPath string         `yaml:"socket_path"`
	Targets    []TargetConfig `yaml:"targets"`
}

// loadAppConfig loads configuration from a YAML file if DEPLOYER_CONFIG is set,
// otherwise falls back to environment variables.
func loadAppConfig() (AppConfig, error) {
	if path := os.Getenv("DEPLOYER_CONFIG"); path != "" {
		return loadFileConfig(path)
	}
	return loadEnvConfig(), nil
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
	}

	return AppConfig{
		ListenAddr: fc.ListenAddr,
		Targets:    fc.Targets,
	}, nil
}

func loadEnvConfig() AppConfig {
	repoURL := os.Getenv("DEPLOYER_REPO")

	// Default compose file path depends on the mode: when a repo is configured, use a relative path resolved
	// inside the work dir; otherwise use an absolute path for the baked-in compose file.
	defaultComposeFile := "/deploy/compose.yaml"
	if repoURL != "" {
		defaultComposeFile = "compose.yaml"
	}

	return AppConfig{
		ListenAddr: getEnv("DEPLOYER_LISTEN_ADDR", ":8080"),
		Targets: []TargetConfig{{
			SocketPath:    getEnv("DEPLOYER_SOCKET_PATH", "/run/uncloud/uncloud.sock"),
			WebhookSecret: os.Getenv("DEPLOYER_WEBHOOK_SECRET"),
			ComposeFile:   getEnv("DEPLOYER_COMPOSE_FILE", defaultComposeFile),
			Branch:        getEnv("DEPLOYER_BRANCH", "main"),
			ForceRecreate: os.Getenv("DEPLOYER_FORCE_RECREATE") == "true",
			RepoURL:       repoURL,
			RepoToken:     os.Getenv("DEPLOYER_REPO_TOKEN"),
			WorkDir:       getEnv("DEPLOYER_WORK_DIR", "/deploy/work"),
		}},
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

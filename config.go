package main

import "os"

type Config struct {
	ListenAddr    string
	SocketPath    string
	WebhookSecret string
	// ComposeFile is the path to the Compose file to deploy. When RepoURL is set and ComposeFile is a relative
	// path, it is resolved relative to WorkDir.
	ComposeFile   string
	Branch        string
	ForceRecreate bool
	// RepoURL is the HTTPS URL of the GitHub repository to clone and check out before deploying. When set,
	// the deployer clones the repository at the push commit and uses it as the build context for services with
	// a build directive. Requires the Docker socket to be mounted at /var/run/docker.sock.
	RepoURL string
	// RepoToken is a GitHub personal access token used to authenticate when cloning a private repository.
	// Requires at least Contents: read permission on the repository.
	RepoToken string
	// WorkDir is the local directory where the repository is cloned when RepoURL is set.
	WorkDir string
}

func loadConfig() Config {
	repoURL := os.Getenv("DEPLOYER_REPO")

	// Default compose file path depends on the mode: when a repo is configured, use a relative path resolved
	// inside the work dir; otherwise use an absolute path for the baked-in compose file.
	defaultComposeFile := "/deploy/compose.yaml"
	if repoURL != "" {
		defaultComposeFile = "compose.yaml"
	}

	return Config{
		ListenAddr:    getEnv("DEPLOYER_LISTEN_ADDR", ":8080"),
		SocketPath:    getEnv("DEPLOYER_SOCKET_PATH", "/run/uncloud/uncloud.sock"),
		WebhookSecret: os.Getenv("DEPLOYER_WEBHOOK_SECRET"),
		ComposeFile:   getEnv("DEPLOYER_COMPOSE_FILE", defaultComposeFile),
		Branch:        getEnv("DEPLOYER_BRANCH", "main"),
		ForceRecreate: os.Getenv("DEPLOYER_FORCE_RECREATE") == "true",
		RepoURL:       repoURL,
		RepoToken:     os.Getenv("DEPLOYER_REPO_TOKEN"),
		WorkDir:       getEnv("DEPLOYER_WORK_DIR", "/deploy/work"),
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

package main

import "os"

type Config struct {
	ListenAddr    string
	SocketPath    string
	WebhookSecret string
	ComposeFile   string
	Branch        string
	ForceRecreate bool
}

func loadConfig() Config {
	return Config{
		ListenAddr:    getEnv("DEPLOYER_LISTEN_ADDR", ":8080"),
		SocketPath:    getEnv("DEPLOYER_SOCKET_PATH", "/run/uncloud/uncloud.sock"),
		WebhookSecret: os.Getenv("DEPLOYER_WEBHOOK_SECRET"),
		ComposeFile:   getEnv("DEPLOYER_COMPOSE_FILE", "/deploy/compose.yaml"),
		Branch:        getEnv("DEPLOYER_BRANCH", "main"),
		ForceRecreate: os.Getenv("DEPLOYER_FORCE_RECREATE") == "true",
	}
}

func getEnv(key, defaultVal string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return defaultVal
}

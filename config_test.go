package main

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadConfig_Defaults(t *testing.T) {
	t.Setenv("DEPLOYER_WEBHOOK_SECRET", "")
	t.Setenv("DEPLOYER_LISTEN_ADDR", "")
	t.Setenv("DEPLOYER_SOCKET_PATH", "")
	t.Setenv("DEPLOYER_COMPOSE_FILE", "")
	t.Setenv("DEPLOYER_BRANCH", "")
	t.Setenv("DEPLOYER_FORCE_RECREATE", "")

	cfg := loadConfig()

	assert.Equal(t, ":8080", cfg.ListenAddr)
	assert.Equal(t, "/run/uncloud/uncloud.sock", cfg.SocketPath)
	assert.Equal(t, "", cfg.WebhookSecret)
	assert.Equal(t, "/deploy/compose.yaml", cfg.ComposeFile)
	assert.Equal(t, "main", cfg.Branch)
	assert.False(t, cfg.ForceRecreate)
}

func TestLoadConfig_Overrides(t *testing.T) {
	t.Setenv("DEPLOYER_WEBHOOK_SECRET", "topsecret")
	t.Setenv("DEPLOYER_LISTEN_ADDR", ":9090")
	t.Setenv("DEPLOYER_SOCKET_PATH", "/tmp/uncloud.sock")
	t.Setenv("DEPLOYER_COMPOSE_FILE", "/app/compose.yaml")
	t.Setenv("DEPLOYER_BRANCH", "production")
	t.Setenv("DEPLOYER_FORCE_RECREATE", "true")

	cfg := loadConfig()

	assert.Equal(t, ":9090", cfg.ListenAddr)
	assert.Equal(t, "/tmp/uncloud.sock", cfg.SocketPath)
	assert.Equal(t, "topsecret", cfg.WebhookSecret)
	assert.Equal(t, "/app/compose.yaml", cfg.ComposeFile)
	assert.Equal(t, "production", cfg.Branch)
	assert.True(t, cfg.ForceRecreate)
}

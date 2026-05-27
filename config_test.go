package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestLoadEnvConfig_Defaults(t *testing.T) {
	t.Setenv("DEPLOYER_CONFIG", "")
	t.Setenv("DEPLOYER_WEBHOOK_SECRET", "")
	t.Setenv("DEPLOYER_LISTEN_ADDR", "")
	t.Setenv("DEPLOYER_SOCKET_PATH", "")
	t.Setenv("DEPLOYER_COMPOSE_FILE", "")
	t.Setenv("DEPLOYER_BRANCH", "")
	t.Setenv("DEPLOYER_FORCE_RECREATE", "")
	t.Setenv("DEPLOYER_REPO", "")

	cfg, err := loadAppConfig()
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.ListenAddr)
	require.Len(t, cfg.Targets, 1)

	tgt := cfg.Targets[0]
	assert.Equal(t, "/run/uncloud/uncloud.sock", tgt.SocketPath)
	assert.Equal(t, "", tgt.WebhookSecret)
	assert.Equal(t, "/deploy/compose.yaml", tgt.ComposeFile)
	assert.Equal(t, "main", tgt.Branch)
	assert.False(t, tgt.ForceRecreate)
}

func TestLoadEnvConfig_RepoMode(t *testing.T) {
	t.Setenv("DEPLOYER_CONFIG", "")
	t.Setenv("DEPLOYER_REPO", "https://github.com/org/app")
	t.Setenv("DEPLOYER_COMPOSE_FILE", "")
	t.Setenv("DEPLOYER_WORK_DIR", "")

	cfg, err := loadAppConfig()
	require.NoError(t, err)

	tgt := cfg.Targets[0]
	// compose_file defaults to relative "compose.yaml" resolved inside work_dir when repo is set
	assert.Equal(t, "compose.yaml", tgt.ComposeFile)
	assert.Equal(t, "/deploy/work", tgt.WorkDir)
}

func TestLoadEnvConfig_Overrides(t *testing.T) {
	t.Setenv("DEPLOYER_CONFIG", "")
	t.Setenv("DEPLOYER_WEBHOOK_SECRET", "topsecret")
	t.Setenv("DEPLOYER_LISTEN_ADDR", ":9090")
	t.Setenv("DEPLOYER_SOCKET_PATH", "/tmp/uncloud.sock")
	t.Setenv("DEPLOYER_COMPOSE_FILE", "/app/compose.yaml")
	t.Setenv("DEPLOYER_BRANCH", "production")
	t.Setenv("DEPLOYER_FORCE_RECREATE", "true")

	cfg, err := loadAppConfig()
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.ListenAddr)
	require.Len(t, cfg.Targets, 1)

	tgt := cfg.Targets[0]
	assert.Equal(t, "/tmp/uncloud.sock", tgt.SocketPath)
	assert.Equal(t, "topsecret", tgt.WebhookSecret)
	assert.Equal(t, "/app/compose.yaml", tgt.ComposeFile)
	assert.Equal(t, "production", tgt.Branch)
	assert.True(t, tgt.ForceRecreate)
}

func TestLoadFileConfig(t *testing.T) {
	yaml := `
listen_addr: :9090
socket_path: /tmp/uc.sock

targets:
  - name: app
    webhook_secret: s3cr3t
    branch: production
    repo_url: https://github.com/org/app
    repo_token: ghp_xxx

  - name: infra
    compose_file: /deploy/infra.yaml
`
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(yaml)
	require.NoError(t, err)
	f.Close()

	t.Setenv("DEPLOYER_CONFIG", f.Name())

	cfg, err := loadAppConfig()
	require.NoError(t, err)

	assert.Equal(t, ":9090", cfg.ListenAddr)
	require.Len(t, cfg.Targets, 2)

	app := cfg.Targets[0]
	assert.Equal(t, "app", app.Name)
	assert.Equal(t, "s3cr3t", app.WebhookSecret)
	assert.Equal(t, "production", app.Branch)
	assert.Equal(t, "https://github.com/org/app", app.RepoURL)
	assert.Equal(t, "ghp_xxx", app.RepoToken)
	assert.Equal(t, "/tmp/uc.sock", app.SocketPath)
	// compose_file defaults to "compose.yaml" when repo_url is set
	assert.Equal(t, "compose.yaml", app.ComposeFile)
	// work_dir defaults to /deploy/work/<name>
	assert.Equal(t, filepath.Join("/deploy/work", "app"), app.WorkDir)

	infra := cfg.Targets[1]
	assert.Equal(t, "infra", infra.Name)
	assert.Equal(t, "main", infra.Branch)
	assert.Equal(t, "/deploy/infra.yaml", infra.ComposeFile)
	assert.Equal(t, "/tmp/uc.sock", infra.SocketPath)
	// work_dir defaults to /deploy/work/<name> even without repo_url
	assert.Equal(t, filepath.Join("/deploy/work", "infra"), infra.WorkDir)
}

func TestLoadFileConfig_Defaults(t *testing.T) {
	// A target with no repo_url, compose_file, branch, or work_dir should get sensible defaults.
	yaml := "targets:\n  - name: app\n    webhook_secret: s3cr3t\n"
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(yaml)
	require.NoError(t, err)
	f.Close()

	t.Setenv("DEPLOYER_CONFIG", f.Name())

	cfg, err := loadAppConfig()
	require.NoError(t, err)

	tgt := cfg.Targets[0]
	assert.Equal(t, "main", tgt.Branch)
	assert.Equal(t, "/deploy/compose.yaml", tgt.ComposeFile)
	assert.Equal(t, filepath.Join("/deploy/work", "app"), tgt.WorkDir)
	assert.Equal(t, "/run/uncloud/uncloud.sock", tgt.SocketPath)
	assert.Equal(t, ":8080", cfg.ListenAddr)
}

func TestLoadFileConfig_Validation(t *testing.T) {
	tests := []struct {
		name    string
		yaml    string
		wantErr string
	}{
		{
			name:    "missing target name",
			yaml:    "targets:\n  - branch: main\n",
			wantErr: "name is required",
		},
		{
			name:    "duplicate target name",
			yaml:    "targets:\n  - name: app\n  - name: app\n",
			wantErr: `duplicate target name "app"`,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
			require.NoError(t, err)
			_, err = f.WriteString(tt.yaml)
			require.NoError(t, err)
			f.Close()

			t.Setenv("DEPLOYER_CONFIG", f.Name())

			_, err = loadAppConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

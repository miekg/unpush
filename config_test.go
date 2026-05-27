package main

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func writeConfig(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp(t.TempDir(), "config-*.yaml")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	f.Close()
	return f.Name()
}

func TestLoadFileConfig(t *testing.T) {
	path := writeConfig(t, `
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
`)
	t.Setenv("DEPLOYER_CONFIG", path)

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
	assert.Equal(t, "compose.yaml", app.ComposeFile)                   // defaults to relative when repo_url is set
	assert.Equal(t, filepath.Join("/deploy/work", "app"), app.WorkDir) // defaults to /deploy/work/<name>

	infra := cfg.Targets[1]
	assert.Equal(t, "infra", infra.Name)
	assert.Equal(t, "main", infra.Branch) // defaults to main
	assert.Equal(t, "/deploy/infra.yaml", infra.ComposeFile)
	assert.Equal(t, "/tmp/uc.sock", infra.SocketPath)
	assert.Equal(t, filepath.Join("/deploy/work", "infra"), infra.WorkDir) // defaults to /deploy/work/<name>
}

func TestLoadFileConfig_Defaults(t *testing.T) {
	// A minimal target with no optional fields should get all defaults filled in.
	path := writeConfig(t, "targets:\n  - name: app\n    webhook_secret: s3cr3t\n")
	t.Setenv("DEPLOYER_CONFIG", path)

	cfg, err := loadAppConfig()
	require.NoError(t, err)

	assert.Equal(t, ":8080", cfg.ListenAddr)
	tgt := cfg.Targets[0]
	assert.Equal(t, "main", tgt.Branch)
	assert.Equal(t, "/deploy/compose.yaml", tgt.ComposeFile) // no repo_url → absolute default
	assert.Equal(t, filepath.Join("/deploy/work", "app"), tgt.WorkDir)
	assert.Equal(t, "/run/uncloud/uncloud.sock", tgt.SocketPath)
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
			t.Setenv("DEPLOYER_CONFIG", writeConfig(t, tt.yaml))
			_, err := loadAppConfig()
			require.Error(t, err)
			assert.Contains(t, err.Error(), tt.wantErr)
		})
	}
}

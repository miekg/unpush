package main

import (
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestOpenState(t *testing.T) {
	db, err := openState(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	defer db.Close()

	// Table must exist and be queryable.
	var count int
	err = db.QueryRow("SELECT COUNT(*) FROM deploys").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count)
}

func TestLastDeploy_Empty(t *testing.T) {
	db, err := openState(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	defer db.Close()

	_, _, ok, err := lastDeploy(db, "app")
	require.NoError(t, err)
	assert.False(t, ok)
}

func TestRecordAndLastDeploy(t *testing.T) {
	db, err := openState(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Truncate(time.Second)

	require.NoError(t, recordDeploy(db, "app", "abc123", now, false))

	commitID, succeeded, ok, err := lastDeploy(db, "app")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "abc123", commitID)
	assert.False(t, succeeded)

	// Record a successful deploy for the same commit.
	require.NoError(t, recordDeploy(db, "app", "abc123", now, true))

	commitID, succeeded, ok, err = lastDeploy(db, "app")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "abc123", commitID)
	assert.True(t, succeeded)
}

func TestLastDeploy_ReturnsLatestRow(t *testing.T) {
	db, err := openState(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Truncate(time.Second)
	require.NoError(t, recordDeploy(db, "app", "aaa111", now, true))
	require.NoError(t, recordDeploy(db, "app", "bbb222", now, false))

	commitID, succeeded, ok, err := lastDeploy(db, "app")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "bbb222", commitID)
	assert.False(t, succeeded)
}

func TestLastDeploy_IsolatedByTarget(t *testing.T) {
	db, err := openState(filepath.Join(t.TempDir(), "state.db"))
	require.NoError(t, err)
	defer db.Close()

	now := time.Now().Truncate(time.Second)
	require.NoError(t, recordDeploy(db, "app", "aaa111", now, true))
	require.NoError(t, recordDeploy(db, "infra", "bbb222", now, false))

	appCommit, appSucceeded, ok, err := lastDeploy(db, "app")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "aaa111", appCommit)
	assert.True(t, appSucceeded)

	infraCommit, infraSucceeded, ok, err := lastDeploy(db, "infra")
	require.NoError(t, err)
	require.True(t, ok)
	assert.Equal(t, "bbb222", infraCommit)
	assert.False(t, infraSucceeded)

	_, _, ok, err = lastDeploy(db, "unknown")
	require.NoError(t, err)
	assert.False(t, ok)
}

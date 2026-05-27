package main

import (
	"bytes"
	"context"
	"fmt"
	"log/slog"
	"net/url"
	"os/exec"
	"strings"
	"time"
)

// remoteHEAD returns the current HEAD commit SHA for branch on the remote repository at repoURL.
// If token is non-empty it is embedded in the URL as an x-access-token credential.
func remoteHEAD(ctx context.Context, repoURL, token, branch string) (string, error) {
	authURL := repoURL
	if token != "" {
		u, err := url.Parse(repoURL)
		if err != nil {
			return "", fmt.Errorf("parse repo URL: %w", err)
		}
		u.User = url.UserPassword("x-access-token", token)
		authURL = u.String()
	}

	cmd := exec.CommandContext(ctx, "git", "ls-remote", authURL, "refs/heads/"+branch)
	cmd.Stderr = logWriter(slog.LevelDebug)
	out, err := cmd.Output()
	if err != nil {
		return "", fmt.Errorf("git ls-remote: %w", err)
	}

	line := strings.TrimSpace(string(bytes.SplitN(out, []byte("\n"), 2)[0]))
	if line == "" {
		return "", fmt.Errorf("branch %q not found on remote", branch)
	}
	sha := strings.Fields(line)[0]
	return sha, nil
}

// localHEAD returns the HEAD commit SHA of the git repository in workDir, or "" if the directory
// does not exist or has no checked-out HEAD yet.
func localHEAD(workDir string) string {
	cmd := exec.Command("git", "-C", workDir, "rev-parse", "HEAD")
	out, err := cmd.Output()
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(out))
}

func (d *Deployer) startPoller(ctx context.Context) {
	interval, _ := time.ParseDuration(d.cfg.PollInterval) // already validated by loadFileConfig

	lastCommit := localHEAD(d.cfg.WorkDir)
	if lastCommit != "" {
		slog.Info("Poller seeded from existing checkout", "target", d.cfg.Name, "commit", lastCommit[:min(len(lastCommit), 8)])
	} else {
		slog.Info("Poller starting fresh, no existing checkout", "target", d.cfg.Name)
	}

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			d.poll(ctx, &lastCommit)
		}
	}
}

func (d *Deployer) poll(ctx context.Context, lastCommit *string) {
	sha, err := remoteHEAD(ctx, d.cfg.RepoURL, d.cfg.RepoToken, d.cfg.Branch)
	if err != nil {
		slog.Error("Failed to fetch remote HEAD", "target", d.cfg.Name, "error", err)
		return
	}

	if sha == *lastCommit {
		slog.Debug("No new commit", "target", d.cfg.Name, "commit", sha[:min(len(sha), 8)])
		return
	}

	slog.Info("New commit detected, triggering deploy", "target", d.cfg.Name,
		"old", (*lastCommit)[:min(len(*lastCommit), 8)],
		"new", sha[:min(len(sha), 8)],
	)
	*lastCommit = sha
	d.triggerDeploy(pushEvent{HeadCommit: struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	}{ID: sha}})
}

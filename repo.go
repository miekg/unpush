package main

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
)

// prepareRepo ensures the repository at repoURL is checked out at commit inside workDir.
// On first call it clones the repo; on subsequent calls it fetches and checks out the new commit.
// If token is non-empty it is embedded in the HTTPS URL as an x-access-token credential.
func prepareRepo(ctx context.Context, workDir, repoURL, token, commit string) error {
	cloneURL := repoURL
	if token != "" {
		u, err := url.Parse(repoURL)
		if err != nil {
			return fmt.Errorf("parse repo URL: %w", err)
		}
		u.User = url.UserPassword("x-access-token", token)
		cloneURL = u.String()
	}

	gitDir := filepath.Join(workDir, ".git")
	if _, err := os.Stat(gitDir); os.IsNotExist(err) {
		// Work dir may be stale from a failed previous clone — remove and start fresh.
		if err := os.RemoveAll(workDir); err != nil {
			return fmt.Errorf("clean work dir: %w", err)
		}
		slog.Info("Cloning repository", "url", repoURL, "dir", workDir)
		// --filter=blob:none defers downloading file contents until checkout, keeping the clone fast.
		if err := runGit(ctx, "", "clone", "--filter=blob:none", "--no-checkout", cloneURL, workDir); err != nil {
			return fmt.Errorf("clone: %w", err)
		}
	} else {
		slog.Info("Fetching repository", "dir", workDir)
		// Update the remote URL in case the token changed since the last clone.
		if err := runGit(ctx, workDir, "remote", "set-url", "origin", cloneURL); err != nil {
			return fmt.Errorf("update remote URL: %w", err)
		}
		if err := runGit(ctx, workDir, "fetch", "origin"); err != nil {
			return fmt.Errorf("fetch: %w", err)
		}
	}

	slog.Info("Checking out commit", "commit", commit[:min(len(commit), 8)])
	if err := runGit(ctx, workDir, "checkout", "--force", commit); err != nil {
		return fmt.Errorf("checkout %s: %w", commit, err)
	}
	return nil
}

func runGit(ctx context.Context, dir string, args ...string) error {
	cmd := exec.CommandContext(ctx, "git", args...)
	cmd.Dir = dir
	cmd.Stdout = logWriter(slog.LevelDebug)
	cmd.Stderr = logWriter(slog.LevelWarn)
	return cmd.Run()
}

// logWriter returns an io.Writer that logs each line written to it at the given level.
type lineWriter struct {
	level slog.Level
	buf   []byte
}

func logWriter(level slog.Level) io.Writer {
	return &lineWriter{level: level}
}

func (w *lineWriter) Write(p []byte) (int, error) {
	w.buf = append(w.buf, p...)
	for {
		idx := -1
		for i, b := range w.buf {
			if b == '\n' {
				idx = i
				break
			}
		}
		if idx < 0 {
			break
		}
		line := string(w.buf[:idx])
		w.buf = w.buf[idx+1:]
		if line != "" {
			slog.Log(context.Background(), w.level, line, "source", "git")
		}
	}
	return len(p), nil
}

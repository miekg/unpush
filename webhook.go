package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
)

type pushEvent struct {
	Ref        string `json:"ref"`
	Repository struct {
		FullName string `json:"full_name"`
	} `json:"repository"`
	HeadCommit struct {
		ID      string `json:"id"`
		Message string `json:"message"`
	} `json:"head_commit"`
}

func (d *Deployer) handleWebhook(w http.ResponseWriter, r *http.Request) {
	event := r.Header.Get("X-GitHub-Event")
	slog.Info("Received webhook request", "target", d.cfg.Name, "event", event, "remote_addr", r.RemoteAddr)

	body, err := io.ReadAll(io.LimitReader(r.Body, 1<<20))
	if err != nil {
		slog.Error("Failed to read webhook body", "target", d.cfg.Name, "error", err)
		http.Error(w, "failed to read body", http.StatusBadRequest)
		return
	}

	if d.cfg.WebhookSecret != "" {
		sig := r.Header.Get("X-Hub-Signature-256")
		if !verifySignature(body, sig, d.cfg.WebhookSecret) {
			slog.Warn("Webhook signature verification failed", "target", d.cfg.Name, "remote_addr", r.RemoteAddr)
			http.Error(w, "invalid signature", http.StatusUnauthorized)
			return
		}
	}

	if event != "push" {
		slog.Debug("Ignoring non-push event", "target", d.cfg.Name, "event", event)
		w.WriteHeader(http.StatusOK)
		return
	}

	var push pushEvent
	if err := json.Unmarshal(body, &push); err != nil {
		slog.Error("Failed to parse webhook body", "target", d.cfg.Name, "error", err)
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	targetRef := "refs/heads/" + d.cfg.Branch
	if push.Ref != targetRef {
		slog.Debug("Ignoring push to non-target branch", "target", d.cfg.Name, "ref", push.Ref, "want", targetRef)
		w.WriteHeader(http.StatusOK)
		return
	}

	commitID := push.HeadCommit.ID
	if len(commitID) > 8 {
		commitID = commitID[:8]
	}
	slog.Info("Accepted push event",
		"target", d.cfg.Name,
		"repo", push.Repository.FullName,
		"branch", d.cfg.Branch,
		"commit", commitID,
		"message", push.HeadCommit.Message,
	)

	w.WriteHeader(http.StatusAccepted)
	d.triggerDeploy(push)
}

func verifySignature(body []byte, sig, secret string) bool {
	if !strings.HasPrefix(sig, "sha256=") {
		return false
	}
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	expected := "sha256=" + hex.EncodeToString(mac.Sum(nil))
	return hmac.Equal([]byte(sig), []byte(expected))
}

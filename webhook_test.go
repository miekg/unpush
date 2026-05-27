package main

import (
	"bytes"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func sign(body []byte, secret string) string {
	mac := hmac.New(sha256.New, []byte(secret))
	mac.Write(body)
	return "sha256=" + hex.EncodeToString(mac.Sum(nil))
}

func newTestDeployer(cfg TargetConfig) *Deployer {
	return &Deployer{
		cfg:   cfg,
		queue: make(chan pushEvent, 1),
	}
}

func TestVerifySignature(t *testing.T) {
	body := []byte(`{"ref":"refs/heads/main"}`)
	secret := "mysecret"

	tests := []struct {
		name  string
		sig   string
		valid bool
	}{
		{"valid signature", sign(body, secret), true},
		{"wrong secret", sign(body, "wrongsecret"), false},
		{"missing prefix", hex.EncodeToString([]byte("anything")), false},
		{"empty signature", "", false},
		{"tampered body", sign([]byte(`{"ref":"refs/heads/other"}`), secret), false},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.valid, verifySignature(body, tt.sig, secret))
		})
	}
}

func TestHandleWebhook(t *testing.T) {
	validPush := pushEvent{Ref: "refs/heads/main"}
	validPush.Repository.FullName = "org/repo"
	validPush.HeadCommit.ID = "abc12345def67890"
	validPush.HeadCommit.Message = "fix: something"

	body, err := json.Marshal(validPush)
	require.NoError(t, err)

	tests := []struct {
		name           string
		secret         string
		sig            string
		event          string
		body           []byte
		wantStatus     int
		wantDeployment bool
	}{
		{
			name:           "valid push to target branch",
			secret:         "s3cr3t",
			sig:            sign(body, "s3cr3t"),
			event:          "push",
			body:           body,
			wantStatus:     http.StatusAccepted,
			wantDeployment: true,
		},
		{
			name:  "push to different branch",
			event: "push",
			body: func() []byte {
				e := validPush
				e.Ref = "refs/heads/feature-x"
				b, _ := json.Marshal(e)
				return b
			}(),
			secret: "s3cr3t",
			sig: func() string {
				e := validPush
				e.Ref = "refs/heads/feature-x"
				b, _ := json.Marshal(e)
				return sign(b, "s3cr3t")
			}(),
			wantStatus:     http.StatusOK,
			wantDeployment: false,
		},
		{
			name:           "non-push event",
			secret:         "s3cr3t",
			sig:            sign(body, "s3cr3t"),
			event:          "pull_request",
			body:           body,
			wantStatus:     http.StatusOK,
			wantDeployment: false,
		},
		{
			name:           "invalid signature",
			secret:         "s3cr3t",
			sig:            sign(body, "wrongsecret"),
			event:          "push",
			body:           body,
			wantStatus:     http.StatusUnauthorized,
			wantDeployment: false,
		},
		{
			name:           "missing signature",
			secret:         "s3cr3t",
			sig:            "",
			event:          "push",
			body:           body,
			wantStatus:     http.StatusUnauthorized,
			wantDeployment: false,
		},
		{
			name:           "no secret configured skips verification",
			secret:         "",
			sig:            "",
			event:          "push",
			body:           body,
			wantStatus:     http.StatusAccepted,
			wantDeployment: true,
		},
		{
			name:           "invalid JSON body",
			secret:         "s3cr3t",
			sig:            sign([]byte("notjson"), "s3cr3t"),
			event:          "push",
			body:           []byte("notjson"),
			wantStatus:     http.StatusBadRequest,
			wantDeployment: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			d := newTestDeployer(TargetConfig{
				WebhookSecret: tt.secret,
				Branch:        "main",
			})

			req := httptest.NewRequest(http.MethodPost, "/webhook", bytes.NewReader(tt.body))
			req.Header.Set("X-GitHub-Event", tt.event)
			if tt.sig != "" {
				req.Header.Set("X-Hub-Signature-256", tt.sig)
			}

			rr := httptest.NewRecorder()
			d.handleWebhook(rr, req)

			assert.Equal(t, tt.wantStatus, rr.Code)
			assert.Equal(t, tt.wantDeployment, len(d.queue) == 1)
		})
	}
}

package handler

import (
	"context"
	"crypto/hmac"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/Paca-AI/api/internal/apierr"
	"github.com/Paca-AI/api/internal/service/nexussync"
	"github.com/Paca-AI/api/internal/transport/http/httpx"
	"github.com/Paca-AI/api/internal/transport/http/presenter"
)

const (
	// nexusSignatureHeader / nexusTimestampHeader carry the ADR-040 §3.1
	// delivery signature: hex HMAC-SHA256 over "<ts>." + raw body.
	nexusSignatureHeader = "X-Nexus-Signature"
	nexusTimestampHeader = "X-Nexus-Timestamp"
	// nexusMaxClockDrift is the replay bound mandated by ADR-040 §3.3.1:
	// deliveries whose timestamp is more than five minutes from now are
	// rejected.
	nexusMaxClockDrift = 5 * time.Minute
	// nexusMaxBodyBytes bounds the webhook body read; identity events are a
	// few hundred bytes, so 1 MiB is generous.
	nexusMaxBodyBytes = 1 << 20
)

// UserChangeApplier applies a verified user.changed event to the local user
// mirror.  Satisfied by nexussync.Service.
type UserChangeApplier interface {
	ApplyUserChanged(ctx context.Context, evt nexussync.UserChanged) (nexussync.Outcome, error)
}

// NexusWebhookHandler implements the ADR-040 identity-sync receiver:
// POST /v1/nexus/webhook.  Authentication is the HMAC signature — the route
// is registered outside the session-auth middleware, like the OIDC callback.
type NexusWebhookHandler struct {
	secret []byte
	sync   UserChangeApplier
	log    *slog.Logger
}

// NewNexusWebhookHandler returns a NexusWebhookHandler.  secret is the shared
// webhook secret (VORTEX_WEBHOOK_SECRET, fallback NEXUS_WEBHOOK_SECRET); the
// caller must not construct the handler with an empty secret.
func NewNexusWebhookHandler(secret []byte, sync UserChangeApplier, log *slog.Logger) *NexusWebhookHandler {
	return &NexusWebhookHandler{secret: secret, sync: sync, log: log}
}

// Handle handles POST /v1/nexus/webhook (ADR-040 §3.3): it verifies the HMAC
// over the exact raw bytes, applies user.changed events, and acknowledges
// every other event type with 200 {"ignored": type} — Paca mirrors no groups,
// dataset grants, or app-admin flags, and a non-2xx would make identity log a
// delivery failure for a benign event.
func (h *NexusWebhookHandler) Handle(w http.ResponseWriter, r *http.Request) {
	body, err := io.ReadAll(io.LimitReader(r.Body, nexusMaxBodyBytes))
	if err != nil {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "could not read request body"))
		return
	}

	if !h.verifySignature(r.Header.Get(nexusTimestampHeader), r.Header.Get(nexusSignatureHeader), body) {
		h.log.Warn("nexus webhook: rejected unverified delivery", "remote_addr", r.RemoteAddr)
		presenter.Error(w, r, apierr.New(apierr.CodeUnauthenticated, "invalid webhook signature"))
		return
	}

	var event struct {
		Type string `json:"type"`
	}
	if err := json.Unmarshal(body, &event); err != nil || event.Type == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "malformed webhook payload"))
		return
	}

	switch event.Type {
	case "user.changed":
		h.handleUserChanged(w, r, body)
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"ignored": event.Type})
	}
}

// handleUserChanged applies the deprovision/restore signal (ADR-040 §3.2).
func (h *NexusWebhookHandler) handleUserChanged(w http.ResponseWriter, r *http.Request, body []byte) {
	var evt struct {
		UserID  string `json:"user_id"`
		Email   string `json:"email"`
		Status  string `json:"status"`
		Deleted bool   `json:"deleted"`
	}
	if err := json.Unmarshal(body, &evt); err != nil || evt.UserID == "" {
		presenter.Error(w, r, apierr.New(apierr.CodeBadRequest, "malformed user.changed payload"))
		return
	}

	outcome, err := h.sync.ApplyUserChanged(r.Context(), nexussync.UserChanged{
		UserID:  evt.UserID,
		Email:   evt.Email,
		Status:  evt.Status,
		Deleted: evt.Deleted,
	})
	if err != nil {
		// A genuine apply failure must surface as 5xx so identity logs the
		// missed delivery; the periodic reconcile is the durability backstop.
		h.log.Error("nexus webhook: user.changed apply failed", "error", err, "vortex_user_id", evt.UserID)
		presenter.Error(w, r, err)
		return
	}

	switch outcome {
	case nexussync.OutcomeNotProvisioned, nexussync.OutcomeNoChange:
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"skipped": string(outcome)})
	default:
		httpx.WriteJSON(w, http.StatusOK, map[string]string{"applied": string(outcome)})
	}
}

// verifySignature checks the ADR-040 delivery signature: hex HMAC-SHA256 with
// the shared secret over "<ts>." + raw body, where ts is the unix-seconds
// value of X-Nexus-Timestamp.  Timestamps more than nexusMaxClockDrift from
// now are rejected (replay bound) and digests are compared in constant time.
func (h *NexusWebhookHandler) verifySignature(ts, sig string, body []byte) bool {
	if len(h.secret) == 0 || ts == "" || sig == "" {
		return false
	}

	tsInt, err := strconv.ParseInt(ts, 10, 64)
	if err != nil {
		return false
	}
	drift := time.Since(time.Unix(tsInt, 0))
	if drift < 0 {
		drift = -drift
	}
	if drift > nexusMaxClockDrift {
		return false
	}

	mac := hmac.New(sha256.New, h.secret)
	mac.Write([]byte(ts))
	mac.Write([]byte("."))
	mac.Write(body)
	expected := mac.Sum(nil)

	// Decode the presented hex digest to bytes so the comparison is
	// case-insensitive and length mismatches fail closed.
	got, err := hex.DecodeString(sig)
	if err != nil {
		return false
	}
	return hmac.Equal(expected, got)
}

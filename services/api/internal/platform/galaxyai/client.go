// Package galaxyai calls the Vortex platform AI on behalf of a Paca user for
// the one-shot "write task description with AI" feature (ADR-038).
//
// It replaces the retired in-app agent (OpenHands) runtime: the agent surface
// is the platform ChatDock now, so this is the single in-context AI touchpoint
// left in Paca. Rather than spawn an agent sandbox, it does a stateless
// completion — mint a short-lived, NON-privileged act_as token at the identity
// service (authenticated with INTERNAL_SERVICE_SECRET via X-Service-Secret),
// then call the OpenAI-compatible /ai/v1 chat proxy with that token. The token
// names the requesting user so usage/billing attributes to them.
package galaxyai

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to the identity mint endpoint + the /ai/v1 chat proxy.
type Client struct {
	identityURL   string
	serviceSecret string
	proxyURL      string
	role          string
	http          *http.Client
}

// New builds a Client. proxyURL defaults to {identityURL}/ai/v1 and role to
// "paca-ai" when empty. A Client with missing identityURL/serviceSecret/proxyURL
// reports Enabled()==false and every call returns ErrDisabled.
func New(identityURL, serviceSecret, proxyURL, role string) *Client {
	if role == "" {
		role = "paca-ai"
	}
	if proxyURL == "" && identityURL != "" {
		proxyURL = strings.TrimRight(identityURL, "/") + "/ai/v1"
	}
	return &Client{
		identityURL:   strings.TrimRight(identityURL, "/"),
		serviceSecret: serviceSecret,
		proxyURL:      strings.TrimRight(proxyURL, "/"),
		role:          role,
		http:          &http.Client{Timeout: 60 * time.Second},
	}
}

// ErrDisabled is returned when the platform AI is not configured.
var ErrDisabled = fmt.Errorf("galaxy platform AI is not configured")

// Enabled reports whether the platform AI is configured.
func (c *Client) Enabled() bool {
	return c != nil && c.identityURL != "" && c.serviceSecret != "" && c.proxyURL != ""
}

// mintActAsToken mints a short-lived RS256 act_as token naming userSub. The
// identity endpoint refuses privileged roles, so a leaked secret can only mint
// a plain, capped token (see identity internal.mint_service_token).
func (c *Client) mintActAsToken(ctx context.Context, userSub string) (string, error) {
	if userSub == "" {
		userSub = "paca-write-with-ai"
	}
	body, _ := json.Marshal(map[string]any{
		"sub":         userSub,
		"aud":         "galaxy",
		"ttl_seconds": 300,
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.identityURL+"/internal/mint-service-token", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Service-Secret", c.serviceSecret)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("mint request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		return "", fmt.Errorf("mint failed: status %d: %s", resp.StatusCode, snippet(raw))
	}
	var out struct {
		AccessToken string `json:"access_token"`
	}
	if err := json.Unmarshal(raw, &out); err != nil || out.AccessToken == "" {
		return "", fmt.Errorf("mint response missing access_token")
	}
	return out.AccessToken, nil
}

// WriteDescription generates a Markdown task description from the task title
// and its current description (either may be empty), attributed to userSub.
// The returned string is Markdown the caller renders/stores as it sees fit.
func (c *Client) WriteDescription(ctx context.Context, userSub, title, currentDescription string) (string, error) {
	if !c.Enabled() {
		return "", ErrDisabled
	}
	token, err := c.mintActAsToken(ctx, userSub)
	if err != nil {
		return "", err
	}

	system := "You are a concise project-management assistant. Given a task's " +
		"title and any existing notes, write a clear, well-structured task " +
		"description in Markdown: a short summary sentence, then bullet points " +
		"for scope/acceptance criteria when useful. Write in the SAME language " +
		"as the title. Return ONLY the description body — no preamble, no title, " +
		"no surrounding code fence."

	var user strings.Builder
	user.WriteString("Task title: ")
	user.WriteString(strings.TrimSpace(title))
	if s := strings.TrimSpace(currentDescription); s != "" {
		user.WriteString("\n\nExisting notes:\n")
		user.WriteString(s)
	}

	body, _ := json.Marshal(map[string]any{
		"model": c.role,
		"messages": []map[string]string{
			{"role": "system", "content": system},
			{"role": "user", "content": user.String()},
		},
	})
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		c.proxyURL+"/chat/completions", bytes.NewReader(body))
	if err != nil {
		return "", err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := c.http.Do(req)
	if err != nil {
		return "", fmt.Errorf("ai request failed: %w", err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(io.LimitReader(resp.Body, 4<<20))
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("ai completion failed: status %d: %s", resp.StatusCode, snippet(raw))
	}
	var out struct {
		Choices []struct {
			Message struct {
				Content string `json:"content"`
			} `json:"message"`
		} `json:"choices"`
	}
	if err := json.Unmarshal(raw, &out); err != nil {
		return "", fmt.Errorf("ai response parse failed: %w", err)
	}
	if len(out.Choices) == 0 {
		return "", fmt.Errorf("ai response had no choices")
	}
	text := strings.TrimSpace(out.Choices[0].Message.Content)
	if text == "" {
		return "", fmt.Errorf("ai response was empty")
	}
	return text, nil
}

func snippet(b []byte) string {
	const max = 300
	if len(b) > max {
		return string(b[:max])
	}
	return string(b)
}

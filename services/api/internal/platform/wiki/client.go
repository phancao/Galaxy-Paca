// Package wiki calls the Galaxy AI Wiki (Outline fork, ADR-042) on behalf of
// a Paca user. The Wiki is the system of record for project documentation:
// Paca provisions one Wiki Folder (space) per project, creates/links Records
// (pages), and proxies search — while the actual editing happens in the Wiki
// editor embedded in Paca's Documentation tab.
//
// Authentication is the platform act-as pattern (the same one wiki_mcp_server
// uses): every request carries the Paca service account's API token as the
// bearer plus X-Galaxy-Act-As naming the end user's OIDC sub, so the Wiki
// enforces ITS OWN permissions as that user and content lands in the user's
// tenant team. An empty Actor runs as the plain service account.
package wiki

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

// Client talks to the Wiki RPC API (POST {apiURL}/api/<method>).
type Client struct {
	apiURL    string // server-to-server base, e.g. http://docx:3000
	apiToken  string // Wiki service-account API token (WIKI_API_TOKEN)
	publicURL string // browser-facing base, e.g. https://wiki.skyplatform.net
	http      *http.Client
}

// New builds a Client. publicURL defaults to apiURL when empty. A Client with
// missing apiURL/apiToken reports Enabled()==false and every call returns
// ErrDisabled.
func New(apiURL, apiToken, publicURL string) *Client {
	if publicURL == "" {
		publicURL = apiURL
	}
	return &Client{
		apiURL:    strings.TrimRight(apiURL, "/"),
		apiToken:  apiToken,
		publicURL: strings.TrimRight(publicURL, "/"),
		http:      &http.Client{Timeout: 30 * time.Second},
	}
}

// ErrDisabled is returned when the Wiki integration is not configured.
var ErrDisabled = fmt.Errorf("wiki integration is not configured")

// Enabled reports whether the Wiki integration is configured.
func (c *Client) Enabled() bool {
	return c != nil && c.apiURL != "" && c.apiToken != ""
}

// Actor names the end user a call runs as (act-as). Zero value = the plain
// service account. Sub is the Vortex OIDC subject (users.oidc_sub); the user
// must have signed in to the Wiki at least once to be resolvable there.
type Actor struct {
	Sub   string
	Email string
}

// PublicPageURL turns a Wiki-relative path (Folder.URL / Record.URL) into a
// browser-facing absolute URL.
func (c *Client) PublicPageURL(path string) string {
	if path == "" || strings.HasPrefix(path, "http") {
		return path
	}
	return c.publicURL + path
}

// Folder is a Wiki space (Outline "collection").
type Folder struct {
	ID   string `json:"id"`
	Name string `json:"name"`
	URL  string `json:"url"`
}

// Record is a Wiki page (Outline "document").
type Record struct {
	ID    string `json:"id"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// NavNode is one node of a folder's page tree (folders.records).
type NavNode struct {
	ID       string    `json:"id"`
	Title    string    `json:"title"`
	URL      string    `json:"url"`
	Children []NavNode `json:"children"`
}

// SearchResult is one records.search hit.
type SearchResult struct {
	Context string `json:"context"`
	Record  Record `json:"record"`
}

// envelope is the Wiki RPC response wrapper.
type envelope struct {
	OK      bool            `json:"ok"`
	Status  int             `json:"status"`
	Data    json.RawMessage `json:"data"`
	Error   string          `json:"error"`
	Message string          `json:"message"`
}

// rpc POSTs one RPC method with the bearer + act-as headers and decodes the
// data payload into out (which may be nil).
func (c *Client) rpc(ctx context.Context, actor Actor, method string, body any, out any) error {
	if !c.Enabled() {
		return ErrDisabled
	}
	payload, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("wiki %s: marshal: %w", method, err)
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.apiURL+"/api/"+method, bytes.NewReader(payload))
	if err != nil {
		return fmt.Errorf("wiki %s: %w", method, err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+c.apiToken)
	// The Wiki enforces https on POST /api/* by proto; internal http calls
	// must assert the forwarded proto or they are answered 405 (the same
	// gotcha as the RAGFlow internal callers).
	req.Header.Set("X-Forwarded-Proto", "https")
	if actor.Sub != "" {
		req.Header.Set("X-Galaxy-Act-As", actor.Sub)
	}
	if actor.Email != "" {
		req.Header.Set("X-Galaxy-Act-As-Email", actor.Email)
	}
	res, err := c.http.Do(req)
	if err != nil {
		return fmt.Errorf("wiki %s: %w", method, err)
	}
	defer res.Body.Close()
	raw, err := io.ReadAll(io.LimitReader(res.Body, 4<<20))
	if err != nil {
		return fmt.Errorf("wiki %s: read: %w", method, err)
	}
	var env envelope
	if err := json.Unmarshal(raw, &env); err != nil {
		if res.StatusCode >= 300 {
			return fmt.Errorf("wiki %s: status %d", method, res.StatusCode)
		}
		return fmt.Errorf("wiki %s: decode: %w", method, err)
	}
	if res.StatusCode >= 300 {
		msg := env.Message
		if msg == "" {
			msg = env.Error
		}
		return fmt.Errorf("wiki %s: status %d: %s", method, res.StatusCode, msg)
	}
	if out != nil && len(env.Data) > 0 {
		if err := json.Unmarshal(env.Data, out); err != nil {
			return fmt.Errorf("wiki %s: decode data: %w", method, err)
		}
	}
	return nil
}

// CreateFolder creates a Wiki space. private=false grants the whole tenant
// team read_write (open space, Confluence-style); private=true keeps the
// folder membership-only so access mirrors a private Paca project.
func (c *Client) CreateFolder(ctx context.Context, actor Actor, name, description string, private bool) (*Folder, error) {
	body := map[string]any{
		"name":        name,
		"description": description,
		"sharing":     false,
	}
	if !private {
		body["permission"] = "read_write"
	}
	var f Folder
	if err := c.rpc(ctx, actor, "folders.create", body, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// FolderInfo fetches one folder.
func (c *Client) FolderInfo(ctx context.Context, actor Actor, id string) (*Folder, error) {
	var f Folder
	if err := c.rpc(ctx, actor, "folders.info", map[string]any{"id": id}, &f); err != nil {
		return nil, err
	}
	return &f, nil
}

// FolderRecords returns the folder's page tree.
func (c *Client) FolderRecords(ctx context.Context, actor Actor, folderID string) ([]NavNode, error) {
	var nodes []NavNode
	if err := c.rpc(ctx, actor, "folders.records", map[string]any{"id": folderID}, &nodes); err != nil {
		return nil, err
	}
	return nodes, nil
}

// CreateRecord creates a published page in a folder.
func (c *Client) CreateRecord(ctx context.Context, actor Actor, folderID, title, text string) (*Record, error) {
	var rec Record
	body := map[string]any{
		"title":    title,
		"text":     text,
		"folderId": folderID,
		"publish":  true,
	}
	if err := c.rpc(ctx, actor, "records.create", body, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// RecordInfo fetches one page; it also serves as an access check — acting as
// a user who cannot read the record fails.
func (c *Client) RecordInfo(ctx context.Context, actor Actor, id string) (*Record, error) {
	var rec Record
	if err := c.rpc(ctx, actor, "records.info", map[string]any{"id": id}, &rec); err != nil {
		return nil, err
	}
	return &rec, nil
}

// searchItem tolerates both the fork's "record" key and upstream "document".
type searchItem struct {
	Context  string  `json:"context"`
	Record   *Record `json:"record"`
	Document *Record `json:"document"`
}

// SearchRecords full-text searches pages, optionally scoped to one folder.
func (c *Client) SearchRecords(ctx context.Context, actor Actor, query, folderID string) ([]SearchResult, error) {
	body := map[string]any{"query": query}
	if folderID != "" {
		body["folderId"] = folderID
	}
	var items []searchItem
	if err := c.rpc(ctx, actor, "records.search", body, &items); err != nil {
		return nil, err
	}
	results := make([]SearchResult, 0, len(items))
	for _, it := range items {
		rec := it.Record
		if rec == nil {
			rec = it.Document
		}
		if rec == nil {
			continue
		}
		results = append(results, SearchResult{Context: it.Context, Record: *rec})
	}
	return results, nil
}

// AddFolderUser grants a Wiki user access to a folder (permission read /
// read_write). Used to mirror Paca project membership onto private spaces.
func (c *Client) AddFolderUser(ctx context.Context, actor Actor, folderID, wikiUserID, permission string) error {
	return c.rpc(ctx, actor, "folders.add_user", map[string]any{
		"id":         folderID,
		"userId":     wikiUserID,
		"permission": permission,
	}, nil)
}

// RemoveFolderUser revokes a Wiki user's folder access.
func (c *Client) RemoveFolderUser(ctx context.Context, actor Actor, folderID, wikiUserID string) error {
	return c.rpc(ctx, actor, "folders.remove_user", map[string]any{
		"id":     folderID,
		"userId": wikiUserID,
	}, nil)
}

// wikiUser is the subset of the Wiki user shape membership sync needs.
type wikiUser struct {
	ID    string `json:"id"`
	Email string `json:"email"`
}

// ResolveUserByEmail finds the Wiki user id for an email (exact,
// case-insensitive match over a users.list query). Returns "" when the user
// has never signed in to the Wiki — membership sync treats that as skippable.
func (c *Client) ResolveUserByEmail(ctx context.Context, email string) (string, error) {
	var users []wikiUser
	if err := c.rpc(ctx, Actor{}, "users.list", map[string]any{"query": email, "limit": 25}, &users); err != nil {
		return "", err
	}
	for _, u := range users {
		if strings.EqualFold(u.Email, email) {
			return u.ID, nil
		}
	}
	return "", nil
}

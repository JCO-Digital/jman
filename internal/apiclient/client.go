// Package apiclient is a minimal HTTP client the `jman` CLI uses to reach
// jman-api's admin endpoints (currently: agent token management) when the
// underlying data lives in jman-api's own database (api.db) rather than the
// shared inventory database the CLI otherwise reads/writes directly.
//
// This is a different actor/auth scheme from internal/agent's client, which
// authenticates as a jman-agent server instance via a long-lived
// X-Agent-Token. apiclient authenticates as a human admin-level jman-api
// user (username/password, optionally TOTP) and holds a short-lived JWT.
package apiclient

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/JCO-Digital/jman/internal/models"
)

// Client talks to jman-api's admin HTTP endpoints as an authenticated human
// user. It is not safe for concurrent use.
type Client struct {
	BaseURL    string
	httpClient *http.Client

	token     string
	expiresAt time.Time
}

// New creates a Client for the given jman-api base URL (e.g.
// "https://jman-api.example.com"). It is not yet authenticated — call
// Login or SetToken before making requests.
func New(baseURL string) *Client {
	return &Client{
		BaseURL:    strings.TrimRight(baseURL, "/"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}
}

// SetToken pre-loads a previously obtained JWT (e.g. from a cached session
// file), skipping an interactive login as long as it hasn't expired.
func (c *Client) SetToken(token string, expiresAt time.Time) {
	c.token = token
	c.expiresAt = expiresAt
}

// Token returns the client's current JWT and its expiry, for callers that
// want to persist a session between CLI invocations.
func (c *Client) Token() (string, time.Time) {
	return c.token, c.expiresAt
}

// Authenticated reports whether the client currently holds an unexpired token.
func (c *Client) Authenticated() bool {
	return c.token != "" && time.Now().Before(c.expiresAt)
}

type loginRequest struct {
	Username string `json:"username"`
	Password string `json:"password"`
	TOTP     string `json:"totp"`
}

type loginResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// Login authenticates against POST /api/auth/login and stores the resulting JWT.
func (c *Client) Login(username, password, totp string) error {
	body, err := json.Marshal(loginRequest{Username: username, Password: password, TOTP: totp})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/auth/login", bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach jman-api at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("login failed: %s", readAPIError(resp))
	}

	var out loginResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("failed to decode login response: %w", err)
	}

	c.token = out.Token
	c.expiresAt = out.ExpiresAt
	return nil
}

type refreshResponse struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expiresAt"`
}

// refresh exchanges the current token for a fresh one via POST /api/auth/refresh.
func (c *Client) refresh() error {
	req, err := http.NewRequest(http.MethodPost, c.BaseURL+"/api/auth/refresh", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach jman-api at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("token refresh failed: %s", readAPIError(resp))
	}

	var out refreshResponse
	if err := json.NewDecoder(resp.Body).Decode(&out); err != nil {
		return fmt.Errorf("failed to decode refresh response: %w", err)
	}

	c.token = out.Token
	c.expiresAt = out.ExpiresAt
	return nil
}

// ensureFreshToken refreshes the held JWT if it's missing or expiring soon.
// Callers must have already authenticated via Login or SetToken — this does
// not perform an initial interactive login.
func (c *Client) ensureFreshToken() error {
	if c.token == "" {
		return fmt.Errorf("not authenticated: call Login first")
	}
	if time.Until(c.expiresAt) > 5*time.Minute {
		return nil
	}
	return c.refresh()
}

// ListAgentTokens calls GET /api/agent-tokens.
func (c *Client) ListAgentTokens() ([]models.AgentToken, error) {
	var tokens []models.AgentToken
	if err := c.doJSON(http.MethodGet, "/api/agent-tokens", nil, &tokens); err != nil {
		return nil, err
	}
	return tokens, nil
}

type createAgentTokenRequest struct {
	ServerID    int    `json:"server_id"`
	ServerName  string `json:"server_name"`
	Description string `json:"description"`
}

type createAgentTokenResponse struct {
	models.AgentToken
	Token string `json:"token"`
}

// CreateAgentToken calls POST /api/agent-tokens and returns the created
// token record along with its one-time plaintext value.
func (c *Client) CreateAgentToken(serverID int, serverName, description string) (models.AgentToken, string, error) {
	reqBody := createAgentTokenRequest{ServerID: serverID, ServerName: serverName, Description: description}
	var out createAgentTokenResponse
	if err := c.doJSON(http.MethodPost, "/api/agent-tokens", reqBody, &out); err != nil {
		return models.AgentToken{}, "", err
	}
	return out.AgentToken, out.Token, nil
}

// RevokeAgentToken calls DELETE /api/agent-tokens/{id}.
func (c *Client) RevokeAgentToken(id int) error {
	return c.doJSON(http.MethodDelete, fmt.Sprintf("/api/agent-tokens/%d", id), nil, nil)
}

// doJSON performs an authenticated JSON request, refreshing the held token
// first if it's near expiry, and decodes the response into out (if non-nil).
func (c *Client) doJSON(method, path string, reqBody, out any) error {
	if err := c.ensureFreshToken(); err != nil {
		return err
	}

	var bodyReader io.Reader
	if reqBody != nil {
		b, err := json.Marshal(reqBody)
		if err != nil {
			return err
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := http.NewRequest(method, c.BaseURL+path, bodyReader)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+c.token)
	if reqBody != nil {
		req.Header.Set("Content-Type", "application/json")
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("failed to reach jman-api at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return fmt.Errorf("request failed: %s", readAPIError(resp))
	}

	if out == nil {
		return nil
	}
	if err := json.NewDecoder(resp.Body).Decode(out); err != nil {
		return fmt.Errorf("failed to decode response: %w", err)
	}
	return nil
}

type apiErrorBody struct {
	Error string `json:"error"`
}

// readAPIError extracts jman-api's standard {"error": "..."} body, falling
// back to the raw status line if the body isn't in that shape.
func readAPIError(resp *http.Response) string {
	data, err := io.ReadAll(resp.Body)
	if err != nil || len(data) == 0 {
		return resp.Status
	}
	var body apiErrorBody
	if err := json.Unmarshal(data, &body); err == nil && body.Error != "" {
		return fmt.Sprintf("%s (%s)", body.Error, resp.Status)
	}
	return fmt.Sprintf("%s: %s", resp.Status, strings.TrimSpace(string(data)))
}

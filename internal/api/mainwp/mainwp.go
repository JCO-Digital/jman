package mainwp

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"

	"github.com/JCO-Digital/jman/internal/config"
)

type addSiteResponse struct {
	Success bool   `json:"success"`
	Error   string `json:"error"`
}

// AddSite makes a POST request to the configured MainWP dashboard API to add a new site
func AddSite(siteURL, admin, adminPassword string) error {
	if config.Cfg.URLMainWP == "" || config.Cfg.TokenMainWP == "" {
		return fmt.Errorf("MainWP URL or Token is not configured")
	}

	endpoint, err := url.Parse(config.Cfg.URLMainWP)
	if err != nil {
		return fmt.Errorf("invalid MainWP URL: %w", err)
	}
	endpoint = endpoint.JoinPath("sites/add")

	q := endpoint.Query()
	q.Add("url", siteURL)
	q.Add("admin", admin)
	q.Add("adminpassword", adminPassword)
	endpoint.RawQuery = q.Encode()

	req, err := http.NewRequest(http.MethodPost, endpoint.String(), nil)
	if err != nil {
		return fmt.Errorf("failed to create MainWP request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer "+config.Cfg.TokenMainWP)

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("MainWP request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		bodyBytes, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("MainWP API error (status %d): %s", resp.StatusCode, string(bodyBytes))
	}

	var addResp addSiteResponse
	if err := json.NewDecoder(resp.Body).Decode(&addResp); err != nil {
		// If the response isn't JSON, we can just read the body string and return an error
		var buf bytes.Buffer
		if _, readErr := buf.ReadFrom(resp.Body); readErr == nil {
			return fmt.Errorf("MainWP returned invalid JSON: %s", buf.String())
		}
		return fmt.Errorf("failed to decode MainWP response: %w", err)
	}

	if !addResp.Success {
		return fmt.Errorf("failed to add site to MainWP: %s", addResp.Error)
	}

	return nil
}

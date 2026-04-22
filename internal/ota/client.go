package ota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

var httpClient = &http.Client{
	Timeout: 60 * time.Second,
}

type CheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	Asset           string `json:"asset"`
}

type TriggerResult struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type StatusResult struct {
	State       string  `json:"state"`
	InProgress  bool    `json:"in_progress"`
	ProgressPct float64 `json:"progress_pct"`
	LastError   string `json:"last_error"`
}

type Client struct {
	baseURL    string
	httpClient *http.Client
}

// NewClient constructs a new OTA client for the device at the given IP and port.
func NewClient(ip string, port int) *Client {
	return &Client{
		baseURL:    fmt.Sprintf("http://%s:%d", ip, port),
		httpClient: httpClient,
	}
}

// Check queries the device for available OTA updates.
// On 202 Accepted, it polls every 2s until receiving 200 OK or context deadline.
func (c *Client) Check(ctx context.Context) (*CheckResult, error) {
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/ota/check", nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}

		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}

		body, err := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if err != nil {
			return nil, fmt.Errorf("read response: %w", err)
		}

		if resp.StatusCode == http.StatusOK {
			var result CheckResult
			if err := json.Unmarshal(body, &result); err != nil {
				return nil, fmt.Errorf("decode response: %w", err)
			}
			return &result, nil
		}

		if resp.StatusCode == http.StatusAccepted {
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(2 * time.Second):
				// retry
			}
			continue
		}

		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}
}

// Trigger initiates an OTA update on the device. Returns the result, HTTP status code, and error.
func (c *Client) Trigger(ctx context.Context) (*TriggerResult, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/ota/update", nil)
	if err != nil {
		return nil, 0, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, 0, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("read response: %w", err)
	}

	var result TriggerResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, resp.StatusCode, fmt.Errorf("decode response: %w", err)
	}

	return &result, resp.StatusCode, nil
}

// FetchVersion queries the device's /api/version plain-text endpoint and
// returns the trimmed version string. Uses a short per-request timeout
// independent of the client's default so the caller can drive its own polling.
func (c *Client) FetchVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/version", nil)
	if err != nil {
		return "", fmt.Errorf("create request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("unexpected status %d", resp.StatusCode)
	}
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}
	v := ""
	for _, r := range string(b) {
		if r == '\n' || r == '\r' {
			continue
		}
		v += string(r)
	}
	return v, nil
}

// WaitForBoot polls /api/version until the device responds with a non-empty
// version string or the context deadline fires. Intended for use after an OTA
// triggers a reboot so callers can display the actually-booted version.
func (c *Client) WaitForBoot(ctx context.Context, interval time.Duration) (string, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// Attempt immediately instead of waiting a full tick.
	for {
		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		v, err := c.FetchVersion(reqCtx)
		cancel()
		if err == nil && v != "" {
			return v, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

// PollStatus queries the device for the current OTA update status.
func (c *Client) PollStatus(ctx context.Context) (*StatusResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/ota/status", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var result StatusResult
	if err := json.Unmarshal(body, &result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	return &result, nil
}

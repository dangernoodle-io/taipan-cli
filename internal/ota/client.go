package ota

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
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

// Check kicks the device's update-check worker and polls /api/update/status
// until last_check_ts advances. Returns a CheckResult on success.
func (c *Client) Check(ctx context.Context) (*CheckResult, error) {
	// Snapshot current ts before kick so we can detect advancement.
	type statusResp struct {
		Current     string `json:"current"`
		Latest      string `json:"latest"`
		Available   bool   `json:"available"`
		LastCheckOK bool   `json:"last_check_ok"`
		LastCheckTs int64  `json:"last_check_ts"`
		DownloadURL string `json:"download_url"`
	}
	fetchStatus := func() (*statusResp, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/update/status", nil)
		if err != nil {
			return nil, fmt.Errorf("create request: %w", err)
		}
		resp, err := c.httpClient.Do(req)
		if err != nil {
			return nil, fmt.Errorf("request failed: %w", err)
		}
		body, _ := io.ReadAll(resp.Body)
		_ = resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
		}
		var s statusResp
		if err := json.Unmarshal(body, &s); err != nil {
			return nil, fmt.Errorf("decode response: %w", err)
		}
		return &s, nil
	}

	before, err := fetchStatus()
	if err != nil {
		return nil, fmt.Errorf("pre-kick status: %w", err)
	}
	beforeTs := before.LastCheckTs

	// Kick the check worker.
	kickReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/update/check", nil)
	if err != nil {
		return nil, fmt.Errorf("create kick request: %w", err)
	}
	kickResp, err := c.httpClient.Do(kickReq)
	if err != nil {
		return nil, fmt.Errorf("kick request failed: %w", err)
	}
	_, _ = io.ReadAll(kickResp.Body)
	_ = kickResp.Body.Close()
	if kickResp.StatusCode != http.StatusOK && kickResp.StatusCode != http.StatusAccepted {
		return nil, fmt.Errorf("kick failed: %d", kickResp.StatusCode)
	}

	// Poll /api/update/status until last_check_ts advances.
	for {
		s, err := fetchStatus()
		if err != nil {
			return nil, err
		}
		if s.LastCheckOK && s.LastCheckTs > beforeTs {
			return &CheckResult{
				CurrentVersion:  s.Current,
				LatestVersion:   s.Latest,
				UpdateAvailable: s.Available,
				Asset:           "",
			}, nil
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(2 * time.Second):
			// retry
		}
	}
}

// Trigger initiates an OTA update on the device. Returns the result, HTTP status code, and error.
func (c *Client) Trigger(ctx context.Context) (*TriggerResult, int, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/update/apply", nil)
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

// FetchVersion queries /api/info and returns the version field.
func (c *Client) FetchVersion(ctx context.Context) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/info", nil)
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
	var info struct {
		Version string `json:"version"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return "", fmt.Errorf("decode response: %w", err)
	}
	return strings.TrimSpace(info.Version), nil
}

// WaitForBoot polls /api/health until the device responds 200 or the context
// deadline fires. Returns the booted version from /api/info once alive.
func (c *Client) WaitForBoot(ctx context.Context, interval time.Duration) (string, error) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	// Attempt immediately instead of waiting a full tick.
	for {
		reqCtx, cancel := context.WithTimeout(ctx, 4*time.Second)
		alive, _ := c.pingHealth(reqCtx)
		cancel()
		if alive {
			v, err := c.FetchVersion(ctx)
			if err != nil {
				return "", err
			}
			return v, nil
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-ticker.C:
		}
	}
}

// pingHealth returns true if /api/health responds 200.
func (c *Client) pingHealth(ctx context.Context) (bool, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/health", nil)
	if err != nil {
		return false, err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return false, err
	}
	_, _ = io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK, nil
}

// PollStatus queries the device for the current OTA update progress.
func (c *Client) PollStatus(ctx context.Context) (*StatusResult, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/update/progress", nil)
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

package ota

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

var httpClient = &http.Client{
	Timeout: 60 * time.Second,
}

// ErrCheckUnavailable is returned by Check when the device does not expose the
// update-check routes (e.g. boot-mode / bb_ota_boot devices). Callers may
// treat this as a swallowable condition and proceed to Trigger.
var ErrCheckUnavailable = errors.New("update check routes unavailable on device")

type CheckResult struct {
	CurrentVersion  string `json:"current_version"`
	LatestVersion   string `json:"latest_version"`
	UpdateAvailable bool   `json:"update_available"`
	Outcome         string `json:"outcome"`
	DownloadURL     string `json:"download_url"`
}

type TriggerResult struct {
	Status string `json:"status"`
	Error  string `json:"error"`
}

type StatusResult struct {
	State       string  `json:"state"`
	InProgress  bool    `json:"in_progress"`
	ProgressPct float64 `json:"progress_pct"`
	LastError   string  `json:"last_error"`
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

// OutcomeCheckOnApply is returned by Check when the device signals that the
// update check will happen during apply (e.g. heap-tight boot-mode boards).
// The caller should skip status polling and call Trigger directly.
const OutcomeCheckOnApply = "check_on_apply"

// Check performs a best-effort update status query. It optionally POSTs
// /api/update/check first (tolerating 404/unsupported), then GETs
// /api/update/status once and returns whatever the device reports.
//
// If the device does not expose these routes (404 / 405 / connection error),
// Check returns (nil, ErrCheckUnavailable) so callers can swallow the error
// and proceed to Trigger directly.
//
// If either the POST /api/update/check body or the GET /api/update/status
// outcome field signals "check_on_apply", Check returns a CheckResult with
// Outcome == OutcomeCheckOnApply so the caller can go straight to Trigger.
func (c *Client) Check(ctx context.Context) (*CheckResult, error) {
	type kickResp struct {
		Status string `json:"status"`
	}
	type statusResp struct {
		Current     string `json:"current"`
		Latest      string `json:"latest"`
		Available   bool   `json:"available"`
		LastCheckTs int64  `json:"last_check_ts"`
		DownloadURL string `json:"download_url"`
		Outcome     string `json:"outcome"`
	}

	// Best-effort POST /api/update/check — tolerate 404 / 405 / network errors.
	kickReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/api/update/check", nil)
	if err != nil {
		return nil, ErrCheckUnavailable
	}
	kickRespHTTP, kickErr := c.httpClient.Do(kickReq)
	if kickErr == nil {
		kickBody, _ := io.ReadAll(kickRespHTTP.Body)
		_ = kickRespHTTP.Body.Close()
		if kickRespHTTP.StatusCode == http.StatusNotFound || kickRespHTTP.StatusCode == http.StatusMethodNotAllowed {
			return nil, ErrCheckUnavailable
		}
		// HTTP 200 with check_on_apply directive: skip status poll, go straight to apply.
		if kickRespHTTP.StatusCode == http.StatusOK {
			var k kickResp
			if err := json.Unmarshal(kickBody, &k); err == nil && k.Status == OutcomeCheckOnApply {
				return &CheckResult{Outcome: OutcomeCheckOnApply}, nil
			}
		}
	}
	// Network error on kick — device may not support the route at all.
	if kickErr != nil {
		return nil, ErrCheckUnavailable
	}

	// GET /api/update/status — single shot, no polling.
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/update/status", nil)
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, ErrCheckUnavailable
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusNotFound, http.StatusMethodNotAllowed:
		return nil, ErrCheckUnavailable
	case http.StatusServiceUnavailable:
		return nil, fmt.Errorf("device update check not initialized")
	case http.StatusOK:
		// fall through
	default:
		return nil, fmt.Errorf("unexpected status %d: %s", resp.StatusCode, body)
	}

	var s statusResp
	if err := json.Unmarshal(body, &s); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	// Status outcome may also signal check_on_apply (available=false, outcome="check_on_apply").
	if s.Outcome == OutcomeCheckOnApply {
		return &CheckResult{Outcome: OutcomeCheckOnApply}, nil
	}

	return &CheckResult{
		CurrentVersion:  s.Current,
		LatestVersion:   s.Latest,
		UpdateAvailable: s.Available,
		Outcome:         s.Outcome,
		DownloadURL:     s.DownloadURL,
	}, nil
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

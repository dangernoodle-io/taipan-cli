package discover

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

// Enrichment timeout needs to cover miners under mining load — /api/info
// composes state under a mutex and can take a second or more. A too-tight
// timeout was causing enrichment to fail silently and leaving info.Version
// pinned to the (potentially stale) mDNS TXT value.
var httpClient = &http.Client{Timeout: 8 * time.Second}

type apiInfoResponse struct {
	Board      string `json:"board"`
	Version    string `json:"version"`
	MAC        string `json:"mac"`
	WorkerName string `json:"worker_name"`
}

func deviceFromEntry(e *zeroconf.ServiceEntry) DeviceInfo {
	info := DeviceInfo{
		Hostname: e.HostName,
		Port:     e.Port,
	}

	if len(e.AddrIPv4) > 0 {
		info.IP = e.AddrIPv4[0].String()
	}

	// Parse TXT records for quick fields
	txt := parseTXT(e.Text)
	info.Board = txt["board"]
	info.Version = txt["version"]
	info.MAC = txt["mac"]

	// Enrich from /api/info (adds worker_name not in TXT). Fall back to the
	// lightweight plain-text /api/version so we always refresh info.Version
	// even when /api/info times out.
	if info.IP != "" {
		infoOK := false
		infoURL := fmt.Sprintf("http://%s:%d/api/info", info.IP, info.Port)
		if resp, err := httpClient.Get(infoURL); err == nil {
			func() {
				defer func() { _ = resp.Body.Close() }()
				var apiResp apiInfoResponse
				if json.NewDecoder(resp.Body).Decode(&apiResp) == nil {
					if apiResp.Board != "" {
						info.Board = apiResp.Board
					}
					if apiResp.Version != "" {
						info.Version = apiResp.Version
					}
					if apiResp.MAC != "" {
						info.MAC = apiResp.MAC
					}
					if apiResp.WorkerName != "" {
						info.Worker = apiResp.WorkerName
					}
					infoOK = true
				}
			}()
		}
		if !infoOK {
			verURL := fmt.Sprintf("http://%s:%d/api/version", info.IP, info.Port)
			if resp, err := httpClient.Get(verURL); err == nil {
				func() {
					defer func() { _ = resp.Body.Close() }()
					if b, err := io.ReadAll(resp.Body); err == nil {
						v := strings.TrimSpace(string(b))
						if v != "" {
							info.Version = v
						}
					}
				}()
			}
		}
	}

	return info
}

func parseTXT(records []string) map[string]string {
	m := make(map[string]string, len(records))
	for _, r := range records {
		if i := strings.Index(r, "="); i > 0 {
			m[r[:i]] = r[i+1:]
		}
	}
	return m
}

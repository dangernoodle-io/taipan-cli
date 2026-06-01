package discover

import (
	"encoding/json"
	"fmt"
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
	Board   string `json:"board"`
	Version string `json:"version"`
}

func deviceFromEntry(e *zeroconf.ServiceEntry) DeviceInfo {
	info := DeviceInfo{
		Hostname: strings.TrimSuffix(e.HostName, "."),
		Port:     e.Port,
	}

	if len(e.AddrIPv4) > 0 {
		info.IP = e.AddrIPv4[0].String()
	}

	// Parse TXT records for quick fields
	txt := parseTXT(e.Text)
	info.Board = txt["board"]
	info.Version = txt["version"]

	// Enrich from /api/info (board/version override stale TXT values).
	if info.IP != "" {
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
				}
			}()
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

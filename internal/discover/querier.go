package discover

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/grandcat/zeroconf"
)

var httpClient = &http.Client{Timeout: 3 * time.Second}

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

	// Enrich from /api/info (adds worker_name not in TXT)
	if info.IP != "" {
		url := fmt.Sprintf("http://%s:%d/api/info", info.IP, info.Port)
		if resp, err := httpClient.Get(url); err == nil {
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

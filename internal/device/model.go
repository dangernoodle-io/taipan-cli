package device

// StatsResponse represents the JSON response from GET /api/stats.
type StatsResponse struct {
	Hashrate        float64  `json:"hashrate"`
	HashrateAvg     float64  `json:"hashrate_avg"`
	TempC           float64  `json:"temp_c"`
	Shares          uint32   `json:"shares"`
	SessionShares   uint32   `json:"session_shares"`
	SessionRejected uint32   `json:"session_rejected"`
	LastShareAgoS   int64    `json:"last_share_ago_s"`
	BestDiff        float64  `json:"best_diff"`
	UptimeS         float64  `json:"uptime_s"`
	AsicHashrate    *float64 `json:"asic_hashrate,omitempty"`
	AsicHashrateAvg *float64 `json:"asic_hashrate_avg,omitempty"`
	AsicShares      *uint32  `json:"asic_shares,omitempty"`
	AsicTempC       *float64 `json:"asic_temp_c,omitempty"`
}

// InfoResponse represents the JSON response from GET /api/info.
type InfoResponse struct {
	Board       string  `json:"board"`
	ProjectName string  `json:"project_name"`
	Version     string  `json:"version"`
	IDFVersion  string  `json:"idf_version"`
	BuildDate   string  `json:"build_date"`
	BuildTime   string  `json:"build_time"`
	Cores       int     `json:"cores"`
	MAC         string  `json:"mac"`
	WorkerName  string  `json:"worker_name"`
	SSID        string  `json:"ssid"`
	TotalHeap   uint64  `json:"total_heap"`
	FreeHeap    uint64  `json:"free_heap"`
	FlashSize   uint32  `json:"flash_size"`
	ResetReason string  `json:"reset_reason"`
	WDTResets   int     `json:"wdt_resets"`
	BootTime    *int64  `json:"boot_time,omitempty"`
	AppSize     *uint32 `json:"app_size,omitempty"`
}

// SettingsResponse represents the JSON response from GET /api/settings.
type SettingsResponse struct {
	PoolHost     string `json:"pool_host"`
	PoolPort     int    `json:"pool_port"`
	Wallet       string `json:"wallet"`
	Worker       string `json:"worker"`
	PoolPass     string `json:"pool_pass"`
	DisplayEn    bool   `json:"display_en"`
	OTASkipCheck bool   `json:"ota_skip_check"`
}

// SetSettingResponse represents the JSON response from PATCH /api/settings.
type SetSettingResponse struct {
	Status         string `json:"status"`
	RebootRequired bool   `json:"reboot_required"`
}

// RebootResponse represents the JSON response from POST /api/reboot.
type RebootResponse struct {
	Status string `json:"status"`
}

// PoolStat represents a single pool entry in the stats array of PoolResponse.
type PoolStat struct {
	Host       string  `json:"host"`
	Port       int     `json:"port"`
	Shares     int64   `json:"shares"`
	BestDiff   float64 `json:"best_diff"`
	BlocksFound int    `json:"blocks_found"`
	LastSeenS  int     `json:"last_seen_s"`
}

// PoolResponse represents the JSON response from GET /api/pool.
type PoolResponse struct {
	Host                  string     `json:"host"`
	Port                  int        `json:"port"`
	Worker                string     `json:"worker"`
	Wallet                string     `json:"wallet"`
	Connected             bool       `json:"connected"`
	SessionStartAgoS      *int64     `json:"session_start_ago_s"`
	CurrentDifficulty     float64    `json:"current_difficulty"`
	PoolEffectiveHashrate float64    `json:"pool_effective_hashrate"`
	LatencyMs             *int       `json:"latency_ms"`
	VersionMask           *string    `json:"version_mask"`
	ActivePoolIdx         int        `json:"active_pool_idx"`
	LifetimeBlocksTotal   int        `json:"lifetime_blocks_total"`
	Stats                 []PoolStat `json:"stats"`
}

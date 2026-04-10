package device

// StatsResponse represents the JSON response from GET /api/stats.
type StatsResponse struct {
	Hashrate        float64  `json:"hashrate"`
	HashrateAvg     float64  `json:"hashrate_avg"`
	TempC           float64  `json:"temp_c"`
	Shares          uint32   `json:"shares"`
	PoolDifficulty  float64  `json:"pool_difficulty"`
	SessionShares   uint32   `json:"session_shares"`
	SessionRejected uint32   `json:"session_rejected"`
	LastShareAgoS   int64    `json:"last_share_ago_s"`
	LifetimeShares  float64  `json:"lifetime_shares"`
	BestDiff        float64  `json:"best_diff"`
	PoolHost        string   `json:"pool_host"`
	PoolPort        int      `json:"pool_port"`
	Worker          string   `json:"worker"`
	Wallet          string   `json:"wallet"`
	UptimeS         float64  `json:"uptime_s"`
	Version         string   `json:"version"`
	BuildDate       string   `json:"build_date"`
	BuildTime       string   `json:"build_time"`
	Board           string   `json:"board"`
	DisplayEn       bool     `json:"display_en"`
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

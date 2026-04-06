package discover

// DeviceInfo represents a discovered TaipanMiner device.
type DeviceInfo struct {
	Hostname string `json:"hostname"`
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Board    string `json:"board"`
	Version  string `json:"version"`
	MAC      string `json:"mac"`
	Worker   string `json:"worker"`
}

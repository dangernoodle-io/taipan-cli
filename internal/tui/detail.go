package tui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var (
	detailSectionStyle = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("63"))
	detailLabelStyle   = lipgloss.NewStyle().Faint(true)
	detailValueStyle   = lipgloss.NewStyle()
	detailDimStyle     = lipgloss.NewStyle().Faint(true)
	detailOnlineStyle  = lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	detailOfflineStyle = lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
)

// renderDetail renders the detail view for the currently selected device.
func renderDetail(m Model) string {
	if len(m.targets) == 0 || m.selected >= len(m.targets) {
		return ""
	}

	d := m.targets[m.selected]
	st, hasState := m.state[d.Hostname]
	isOffline := !hasState || st.err != nil || st.stats == nil

	var sb strings.Builder

	// ── header ─────────────────────────────────────────────────────────────
	var statusStr string
	if isOffline {
		statusStr = detailOfflineStyle.Render("offline")
	} else {
		statusStr = detailOnlineStyle.Render("online")
	}

	board := d.Board
	version := d.Version
	if hasState && st.info != nil {
		if st.info.Board != "" {
			board = st.info.Board
		}
		if st.info.Version != "" {
			version = st.info.Version
		}
	}

	sb.WriteString(detailSectionStyle.Render("▐▎ " + d.Hostname))
	sb.WriteString("  ")
	sb.WriteString(statusStr)
	sb.WriteString("\n")
	sb.WriteString(detailDimStyle.Render(fmt.Sprintf("   %s  %s", board, version)))
	sb.WriteString("\n\n")

	if isOffline {
		msg := "no data available"
		if hasState && st.err != nil {
			msg = st.err.Error()
		}
		sb.WriteString(detailDimStyle.Render("   " + msg))
		sb.WriteString("\n")
		return sb.String()
	}

	s := st.stats

	// ── mining ─────────────────────────────────────────────────────────────
	sb.WriteString(detailSectionStyle.Render("  Mining"))
	sb.WriteString("\n")

	hashrate := s.Hashrate
	hashrateAvg := s.HashrateAvg
	if s.AsicHashrate != nil {
		hashrate = *s.AsicHashrate
	}
	if s.AsicHashrateAvg != nil {
		hashrateAvg = *s.AsicHashrateAvg
	}

	tempC := s.TempC
	if s.AsicTempC != nil {
		tempC = *s.AsicTempC
	}

	acc := s.SessionShares
	if s.AsicShares != nil {
		acc = *s.AsicShares
	}

	writeField(&sb, "Hashrate", fmt.Sprintf("%s  avg %s", fmtHashrate(hashrate), fmtHashrate(hashrateAvg)))
	writeField(&sb, "Temp", fmt.Sprintf("%.1f°C", tempC))
	writeField(&sb, "Shares", fmt.Sprintf("%d accepted  %d rejected", acc, s.SessionRejected))
	writeField(&sb, "Best Diff", fmtDiff(s.BestDiff))
	writeField(&sb, "Uptime", fmtDuration(s.UptimeS))

	if s.AsicHashrate != nil {
		sb.WriteString("\n")
		sb.WriteString(detailDimStyle.Render("   ASIC"))
		sb.WriteString("\n")
		writeField(&sb, "ASIC Hashrate", fmt.Sprintf("%s  avg %s", fmtHashrate(*s.AsicHashrate), fmtHashrate(hashrateAvg)))
		if s.AsicTempC != nil {
			writeField(&sb, "ASIC Temp", fmt.Sprintf("%.1f°C", *s.AsicTempC))
		}
		if s.AsicShares != nil {
			writeField(&sb, "ASIC Shares", fmt.Sprintf("%d", *s.AsicShares))
		}
	}

	// ── pool ───────────────────────────────────────────────────────────────
	sb.WriteString("\n")
	sb.WriteString(detailSectionStyle.Render("  Pool"))
	sb.WriteString("\n")

	if st.pool != nil {
		p := st.pool
		connStr := detailDimStyle.Render("disconnected")
		if p.Connected {
			connStr = detailOnlineStyle.Render("connected")
		}
		writeField(&sb, "Server", fmt.Sprintf("%s:%d", p.Host, p.Port))
		writeField(&sb, "Worker", p.Worker)
		writeFieldRaw(&sb, "Connected", connStr)
		writeField(&sb, "Difficulty", fmtDiff(p.CurrentDifficulty))
		writeField(&sb, "Pool Hashrate", fmtHashrate(p.PoolEffectiveHashrate))
		writeField(&sb, "Lifetime Blocks", fmt.Sprintf("%d", p.LifetimeBlocksTotal))
		if p.LatencyMs != nil {
			writeField(&sb, "Latency", fmt.Sprintf("%d ms", *p.LatencyMs))
		}
		if p.SessionStartAgoS != nil {
			writeField(&sb, "Session Age", fmtDuration(float64(*p.SessionStartAgoS)))
		}
	} else {
		sb.WriteString(detailDimStyle.Render("   no pool data"))
		sb.WriteString("\n")
	}

	// ── device info ────────────────────────────────────────────────────────
	sb.WriteString("\n")
	sb.WriteString(detailSectionStyle.Render("  Device"))
	sb.WriteString("\n")

	if st.info != nil {
		info := st.info
		writeField(&sb, "Board", info.Board)
		writeField(&sb, "Version", info.Version)
		writeField(&sb, "IDF Version", info.IDFVersion)
		writeField(&sb, "SSID", info.Network.SSID)
		writeField(&sb, "Free Heap", fmt.Sprintf("%s / %s", fmtBytes(info.FreeHeap), fmtBytes(info.TotalHeap)))
		writeField(&sb, "Reset Reason", info.ResetReason)
	} else {
		sb.WriteString(detailDimStyle.Render("   no device info"))
		sb.WriteString("\n")
	}

	return sb.String()
}

// renderDetailFooter renders the footer hint line for detail mode.
func renderDetailFooter() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		" esc back · ↑/↓ scroll · r refresh · q quit",
	)
}

func writeField(sb *strings.Builder, label, value string) {
	fmt.Fprintf(sb, "   %s  %s\n",
		detailLabelStyle.Render(padRight(label, 14)),
		detailValueStyle.Render(value),
	)
}

func writeFieldRaw(sb *strings.Builder, label, rendered string) {
	fmt.Fprintf(sb, "   %s  %s\n",
		detailLabelStyle.Render(padRight(label, 14)),
		rendered,
	)
}

// fmtDuration formats seconds as a human-readable duration string.
func fmtDuration(secs float64) string {
	d := time.Duration(secs) * time.Second
	h := int(d.Hours())
	m := int(d.Minutes()) % 60
	s := int(d.Seconds()) % 60
	if h > 0 {
		return fmt.Sprintf("%dh %dm %ds", h, m, s)
	}
	if m > 0 {
		return fmt.Sprintf("%dm %ds", m, s)
	}
	return fmt.Sprintf("%ds", s)
}

// fmtDiff formats a difficulty value with SI suffix.
func fmtDiff(d float64) string {
	switch {
	case d >= 1e12:
		return fmt.Sprintf("%.2f T", d/1e12)
	case d >= 1e9:
		return fmt.Sprintf("%.2f G", d/1e9)
	case d >= 1e6:
		return fmt.Sprintf("%.2f M", d/1e6)
	case d >= 1e3:
		return fmt.Sprintf("%.2f K", d/1e3)
	default:
		return fmt.Sprintf("%.2f", d)
	}
}

// fmtBytes formats a byte count with appropriate unit.
func fmtBytes(b uint64) string {
	switch {
	case b >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(b)/(1<<20))
	case b >= 1<<10:
		return fmt.Sprintf("%.1f KB", float64(b)/(1<<10))
	default:
		return fmt.Sprintf("%d B", b)
	}
}

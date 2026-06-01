package tui

import (
	"fmt"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/bubbles/spinner"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/dangernoodle-io/taipan-cli/internal/device"
	"github.com/dangernoodle-io/taipan-cli/internal/discover"
)

// viewMode controls whether we show the fleet list or device detail.
type viewMode int

const (
	modeList   viewMode = iota
	modeDetail viewMode = iota
)

// tickMsg is the periodic refresh signal.
type tickMsg time.Time

func tickCmd() tea.Cmd {
	return tea.Tick(5*time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

// discoveredMsg carries the result of the async discovery step.
type discoveredMsg struct {
	targets []discover.DeviceInfo
	err     error
}

func discoverCmd(fn func() ([]discover.DeviceInfo, error)) tea.Cmd {
	return func() tea.Msg {
		ts, err := fn()
		return discoveredMsg{targets: ts, err: err}
	}
}

// deviceState holds the most-recent poll result for one device.
type deviceState struct {
	stats   *device.StatsResponse
	pool    *device.PoolResponse
	info    *device.InfoResponse
	err     error
	updated time.Time
}

// poolAggregate holds summary data for one pool.
type poolAggregate struct {
	host     string
	hashrate float64
	count    int
}

// Model is the root Bubble Tea model for the fleet monitor.
type Model struct {
	discoverFn  func() ([]discover.DeviceInfo, error)
	targets     []discover.DeviceInfo
	state       map[string]deviceState
	spin        spinner.Model
	vp          viewport.Model
	mode        viewMode
	discovering bool
	discoverErr error
	ready       bool
	selected    int
	width       int
	height      int
}

// NewModel constructs the initial model; discovery runs as the first TUI command.
func NewModel(discoverFn func() ([]discover.DeviceInfo, error)) Model {
	spin := spinner.New(spinner.WithSpinner(spinner.Dot))
	return Model{
		discoverFn:  discoverFn,
		state:       make(map[string]deviceState),
		spin:        spin,
		discovering: true,
	}
}

// Init starts discovery and the spinner immediately.
func (m Model) Init() tea.Cmd {
	return tea.Batch(discoverCmd(m.discoverFn), m.spin.Tick)
}

// Update handles all incoming messages.
func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		m.vp.Width = msg.Width
		m.vp.Height = msg.Height - 4 // reserve header + footer lines
		if m.mode == modeDetail {
			m.vp.SetContent(renderDetail(m))
		}
		return m, nil

	case discoveredMsg:
		m.discovering = false
		if msg.err != nil {
			m.discoverErr = msg.err
			return m, nil
		}
		m.targets = msg.targets
		if len(m.targets) == 0 {
			return m, nil
		}
		return m, tea.Batch(refreshAll(m.targets), tickCmd())

	case tickMsg:
		if len(m.targets) > 0 {
			return m, tea.Batch(refreshAll(m.targets), tickCmd())
		}
		return m, tickCmd()

	case PolledMsg:
		m.state[msg.Host] = deviceState{
			stats:   msg.Stats,
			pool:    msg.Pool,
			info:    msg.Info,
			err:     msg.Err,
			updated: time.Now(),
		}
		if !m.ready && len(m.targets) > 0 && len(m.state) >= len(m.targets) {
			m.ready = true
		}
		if m.mode == modeDetail {
			m.vp.SetContent(renderDetail(m))
		}
		return m, nil

	case spinner.TickMsg:
		if m.discovering || !m.ready {
			var cmd tea.Cmd
			m.spin, cmd = m.spin.Update(msg)
			return m, cmd
		}
		return m, nil

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		case "r":
			if len(m.targets) > 0 {
				return m, refreshAll(m.targets)
			}
			return m, nil
		case "up", "k":
			if m.mode == modeDetail {
				m.vp.ScrollUp(1)
				return m, nil
			}
			if m.selected > 0 {
				m.selected--
			}
			return m, nil
		case "down", "j":
			if m.mode == modeDetail {
				m.vp.ScrollDown(1)
				return m, nil
			}
			if m.selected < len(m.targets)-1 {
				m.selected++
			}
			return m, nil
		case "enter":
			if m.mode == modeList && m.ready && len(m.targets) > 0 {
				m.mode = modeDetail
				m.vp.Width = m.width
				m.vp.Height = m.height - 4
				m.vp.GotoTop()
				m.vp.SetContent(renderDetail(m))
			}
			return m, nil
		case "esc":
			if m.mode == modeDetail {
				m.mode = modeList
			}
			return m, nil
		}
	}

	return m, nil
}

// View renders the TUI.
func (m Model) View() string {
	if m.discovering {
		return fmt.Sprintf("\n\n  %s  Discovering devices…\n", m.spin.View())
	}
	if m.discoverErr != nil {
		errStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("9"))
		return errStyle.Render(fmt.Sprintf("\n  Error: %v\n", m.discoverErr)) + "\n  press q to quit\n"
	}
	if len(m.targets) == 0 {
		return "\n  No devices found · press q to quit\n"
	}

	if !m.ready {
		plural := "miner"
		if len(m.targets) != 1 {
			plural = "miners"
		}
		querying := fmt.Sprintf("  %s  Querying %d %s…", m.spin.View(), len(m.targets), plural)
		return "\n" + querying + "\n"
	}

	if m.mode == modeDetail {
		content := renderDetail(m)
		if m.height > 4 && m.vp.Height > 0 {
			m.vp.SetContent(content)
			return m.vp.View() + "\n" + renderDetailFooter()
		}
		return content + "\n" + renderDetailFooter()
	}

	return renderBanner(m) + "\n" + renderHeader(m) + "\n" + renderRows(m) + "\n" + renderFooter()
}

// renderBanner renders the fleet summary banner.
func renderBanner(m Model) string {
	n := len(m.targets)
	online, pools := fleetSummary(m)

	titleStyle := lipgloss.NewStyle().Bold(true)
	dimStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))

	line1 := titleStyle.Render(fmt.Sprintf(" FLEET   %d miner%s · %d online", n, plural(n), online))

	var sb strings.Builder
	sb.WriteString(line1)
	for _, p := range pools {
		sb.WriteString("\n")
		sb.WriteString(dimStyle.Render(fmt.Sprintf("   %-24s %s · %d miner%s", p.host, fmtHashrate(p.hashrate), p.count, plural(p.count))))
	}
	return sb.String()
}

// renderHeader renders the column header line.
func renderHeader(m Model) string {
	headerStyle := lipgloss.NewStyle().Bold(true)
	dimStyle := lipgloss.NewStyle().Faint(true)

	// Compute column widths from targets
	hostWidth := len("Host")
	boardWidth := len("Board")
	versionWidth := len("Version")
	hashWidth := len("Hashrate")
	tempWidth := utf8.RuneCountInString("Temp")
	sharesWidth := len("Shares")

	for _, d := range m.targets {
		hostWidth = max(hostWidth, len(d.Hostname))
		boardWidth = max(boardWidth, len(d.Board))
		versionWidth = max(versionWidth, len(d.Version))

		st, hasState := m.state[d.Hostname]
		if hasState && st.err == nil && st.stats != nil {
			s := st.stats
			hashrate := s.Hashrate
			if s.AsicHashrate != nil {
				hashrate = *s.AsicHashrate
			}
			tempC := s.TempC
			if s.AsicTempC != nil {
				tempC = *s.AsicTempC
			}
			acc := s.SessionShares
			if s.AsicShares != nil {
				acc = *s.AsicShares
			}

			hashStr := fmtHashrate(hashrate)
			tempStr := fmt.Sprintf("%.1f°C", tempC)
			sharesStr := fmt.Sprintf("%d/%d", acc, s.SessionRejected)

			hashWidth = max(hashWidth, len(hashStr))
			tempWidth = max(tempWidth, utf8.RuneCountInString(tempStr))
			sharesWidth = max(sharesWidth, len(sharesStr))
		}
	}

	// Compute prefix width (accent bar + dot + space: "▎ " = 2 chars + space = 3)
	const prefixWidth = 3
	const gutter = 2

	// Build header with padding
	hostPadded := padRight("Host", hostWidth)
	boardPadded := padRight("Board", boardWidth)
	versionPadded := padRight("Version", versionWidth)
	hashPadded := padRight("Hashrate", hashWidth)
	tempPadded := padRightRunes("Temp", tempWidth)
	sharesPadded := padRight("Shares", sharesWidth)

	headerLine := fmt.Sprintf("%s %s %s%s %s%s %s%s %s%s %s",
		strings.Repeat(" ", prefixWidth),
		hostPadded,
		strings.Repeat(" ", gutter),
		boardPadded,
		strings.Repeat(" ", gutter),
		versionPadded,
		strings.Repeat(" ", gutter),
		hashPadded,
		strings.Repeat(" ", gutter),
		tempPadded,
		strings.Repeat(" ", gutter)+sharesPadded,
	)

	return dimStyle.Render(headerStyle.Render(headerLine))
}

// renderRows renders the accent-card rows for all targets.
func renderRows(m Model) string {
	accentStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("63"))
	accentSelStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("99")).Bold(true)
	dimStyle := lipgloss.NewStyle().Faint(true)
	onlineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("2"))
	offlineStyle := lipgloss.NewStyle().Foreground(lipgloss.Color("240"))
	boldStyle := lipgloss.NewStyle().Bold(true)

	// First pass: build display strings and compute column widths
	type rowData struct {
		accentStr   string
		dotStr      string
		hostStr     string
		boardStr    string
		versionStr  string
		hashStr     string
		tempStr     string
		sharesStr   string
	}

	var rowsData []rowData
	hostWidth := 0
	boardWidth := 0
	versionWidth := 0
	hashWidth := 0
	tempWidth := 0
	sharesWidth := 0

	for i, d := range m.targets {
		sel := i == m.selected
		st, hasState := m.state[d.Hostname]
		isOffline := !hasState || st.err != nil || st.stats == nil

		var accent string
		if sel {
			accent = accentSelStyle.Render("▐▎")
		} else {
			accent = accentStyle.Render(" ▎")
		}

		// Compute display strings
		if isOffline {
			dot := offlineStyle.Render("○")
			hostStr := dimStyle.Render(d.Hostname)
			boardStr := dimStyle.Render(d.Board)
			versionStr := dimStyle.Render(d.Version)
			hashStr := dimStyle.Render("")
			tempStr := dimStyle.Render("")
			sharesStr := dimStyle.Render("")

			rd := rowData{
				accentStr:  accent,
				dotStr:     dot,
				hostStr:    hostStr,
				boardStr:   boardStr,
				versionStr: versionStr,
				hashStr:    hashStr,
				tempStr:    tempStr,
				sharesStr:  sharesStr,
			}
			rowsData = append(rowsData, rd)

			// Track widths (use uncolored length for alignment)
			hostWidth = max(hostWidth, len(d.Hostname))
			boardWidth = max(boardWidth, len(d.Board))
			versionWidth = max(versionWidth, len(d.Version))
		} else {
			s := st.stats
			hashrate := s.Hashrate
			if s.AsicHashrate != nil {
				hashrate = *s.AsicHashrate
			}
			tempC := s.TempC
			if s.AsicTempC != nil {
				tempC = *s.AsicTempC
			}
			acc := s.SessionShares
			if s.AsicShares != nil {
				acc = *s.AsicShares
			}

			dot := onlineStyle.Render("●")

			var hostStr string
			if sel {
				hostStr = boldStyle.Render(d.Hostname)
			} else {
				hostStr = d.Hostname
			}

			hashStr := fmtHashrate(hashrate)
			tempStr := fmt.Sprintf("%.1f°C", tempC)
			sharesStr := fmt.Sprintf("%d/%d", acc, s.SessionRejected)

			rd := rowData{
				accentStr:  accent,
				dotStr:     dot,
				hostStr:    hostStr,
				boardStr:   d.Board,
				versionStr: d.Version,
				hashStr:    hashStr,
				tempStr:    tempStr,
				sharesStr:  sharesStr,
			}
			rowsData = append(rowsData, rd)

			// Track widths (use uncolored length for alignment)
			hostWidth = max(hostWidth, len(d.Hostname))
			boardWidth = max(boardWidth, len(d.Board))
			versionWidth = max(versionWidth, len(d.Version))
			hashWidth = max(hashWidth, len(hashStr))
			tempWidth = max(tempWidth, utf8.RuneCountInString(tempStr))
			sharesWidth = max(sharesWidth, len(sharesStr))
		}
	}

	// Second pass: format rows with aligned columns
	const gutter = 2
	var rows []string
	for i, rd := range rowsData {
		st, hasState := m.state[m.targets[i].Hostname]
		isOffline := !hasState || st.err != nil || st.stats == nil

		if isOffline {
			// Offline: accent dot hostname<padded> board<padded> version<padded> offline
			hostPadded := padRight(m.targets[i].Hostname, hostWidth)
			boardPadded := padRight(m.targets[i].Board, boardWidth)
			versionPadded := padRight(m.targets[i].Version, versionWidth)
			offline := dimStyle.Render("offline")
			rows = append(rows, fmt.Sprintf("%s %s %s %s%s %s%s %s",
				rd.accentStr,
				rd.dotStr,
				hostPadded,
				strings.Repeat(" ", gutter),
				boardPadded,
				strings.Repeat(" ", gutter),
				versionPadded,
				strings.Repeat(" ", gutter)+offline,
			))
		} else {
			hostPadded := padRight(m.targets[i].Hostname, hostWidth)
			boardPadded := padRight(m.targets[i].Board, boardWidth)
			versionPadded := padRight(rd.versionStr, versionWidth)
			hashPadded := padRight(rd.hashStr, hashWidth)
			tempPadded := padRightRunes(rd.tempStr, tempWidth)
			sharesPadded := padRight(rd.sharesStr, sharesWidth)

			rows = append(rows, fmt.Sprintf("%s %s %s %s%s %s%s %s%s %s%s %s",
				rd.accentStr,
				rd.dotStr,
				hostPadded,
				strings.Repeat(" ", gutter),
				boardPadded,
				strings.Repeat(" ", gutter),
				versionPadded,
				strings.Repeat(" ", gutter),
				hashPadded,
				strings.Repeat(" ", gutter),
				tempPadded,
				strings.Repeat(" ", gutter)+sharesPadded,
			))
		}
	}

	return strings.Join(rows, "\n")
}

// padRight pads a string to the given width on the right with spaces.
func padRight(s string, width int) string {
	if len(s) >= width {
		return s
	}
	return s + strings.Repeat(" ", width-len(s))
}

// padRightRunes pads a string to the given rune-count width on the right.
// Use this for strings with multi-byte characters like the degree sign.
func padRightRunes(s string, width int) string {
	runeCount := utf8.RuneCountInString(s)
	if runeCount >= width {
		return s
	}
	return s + strings.Repeat(" ", width-runeCount)
}

// max returns the greater of two integers.
func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

// renderFooter renders the keyboard hint line.
func renderFooter() string {
	return lipgloss.NewStyle().Foreground(lipgloss.Color("240")).Render(
		" ↑/↓ select · enter detail · r refresh · q quit",
	)
}

// fleetSummary returns online count and sorted pool aggregates.
func fleetSummary(m Model) (int, []poolAggregate) {
	online := 0
	poolMap := map[string]*poolAggregate{}

	for _, d := range m.targets {
		st, ok := m.state[d.Hostname]
		if !ok || st.err != nil || st.stats == nil {
			continue
		}
		online++
		if st.pool == nil {
			continue
		}
		host := st.pool.Host
		hashrate := st.stats.Hashrate
		if st.stats.AsicHashrate != nil {
			hashrate = *st.stats.AsicHashrate
		}
		if agg, exists := poolMap[host]; exists {
			agg.hashrate += hashrate
			agg.count++
		} else {
			poolMap[host] = &poolAggregate{host: host, hashrate: hashrate, count: 1}
		}
	}

	pools := make([]poolAggregate, 0, len(poolMap))
	for _, v := range poolMap {
		pools = append(pools, *v)
	}
	sort.Slice(pools, func(i, j int) bool {
		return pools[i].hashrate > pools[j].hashrate
	})
	return online, pools
}

// plural returns "s" when n != 1.
func plural(n int) string {
	if n == 1 {
		return ""
	}
	return "s"
}

// fmtHashrate formats a hashrate value for display.
func fmtHashrate(h float64) string {
	switch {
	case h >= 1e12:
		return fmt.Sprintf("%.2f TH/s", h/1e12)
	case h >= 1e9:
		return fmt.Sprintf("%.2f GH/s", h/1e9)
	case h >= 1e6:
		return fmt.Sprintf("%.2f MH/s", h/1e6)
	case h >= 1e3:
		return fmt.Sprintf("%.2f kH/s", h/1e3)
	default:
		return fmt.Sprintf("%.2f H/s", h)
	}
}

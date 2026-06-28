package cli

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	"github.com/dangernoodle-io/taipan-cli/internal/ui"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	logHosts   []string
	logAll     bool
	logTimeout int
)

// logEvent is the structured JSON payload emitted by breadboard's /api/events?topic=log endpoint.
type logEvent struct {
	Ts    int64  `json:"ts"`
	Level string `json:"level"`
	Tag   string `json:"tag"`
	Msg   string `json:"msg"`
}

var logCmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream logs from TaipanMiner devices",
	RunE:  runLog,
}

func init() {
	logCmd.Flags().StringArrayVar(&logHosts, "host", nil, "Target device hostname (repeatable)")
	logCmd.Flags().BoolVar(&logAll, "all", false, "Stream logs from all discovered devices")
	logCmd.Flags().IntVarP(&logTimeout, "timeout", "t", 5, "Browse timeout in seconds")
	rootCmd.AddCommand(logCmd)
}

var logPalette = []color.Attribute{
	color.FgCyan, color.FgMagenta, color.FgYellow,
	color.FgGreen, color.FgBlue, color.FgRed,
}

func runLog(cmd *cobra.Command, args []string) error {
	if !logAll && len(logHosts) == 0 {
		return fmt.Errorf("must specify --all or at least one --host")
	}

	targetDevices, err := resolveTargets(logHosts, logAll, logTimeout)
	if err != nil {
		return err
	}
	if len(targetDevices) == 0 {
		return fmt.Errorf("no devices found")
	}

	ctx, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	multi := len(targetDevices) > 1

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, device := range targetDevices {
		wg.Add(1)

		var prefixFn func(string) string
		if multi {
			c := color.New(logPalette[i%len(logPalette)])
			prefixFn = func(hostname string) string {
				return c.Sprintf("[%s] ", hostname)
			}
		}

		go func(d discover.DeviceInfo, pf func(string) string) {
			defer wg.Done()
			stopConn := ui.Single("connecting to " + d.Hostname)
			if err := streamDevice(ctx, d, pf, &mu, stopConn); err != nil && ctx.Err() == nil {
				output.Error("[%s] %v", d.Hostname, err)
			}
		}(device, prefixFn)
	}

	wg.Wait()
	return nil
}

func streamDevice(ctx context.Context, device discover.DeviceInfo, prefixFn func(string) string, mu *sync.Mutex, stopConn func()) error {
	url := fmt.Sprintf("http://%s:%d/api/events?topic=log", device.IP, device.Port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		stopConn()
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		stopConn()
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		stopConn()
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	connStopped := false
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines, SSE comments, event: and id: fields
		if line == "" || strings.HasPrefix(line, ":") ||
			strings.HasPrefix(line, "event:") || strings.HasPrefix(line, "id:") {
			continue
		}

		// Only process data: lines
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		// Stop the connection spinner before printing the first line
		if !connStopped {
			stopConn()
			connStopped = true
		}

		formatted := formatLogEvent(payload)

		if prefixFn != nil {
			mu.Lock()
			fmt.Println(prefixFn(device.Hostname) + formatted)
			mu.Unlock()
		} else {
			fmt.Println(formatted)
		}
	}

	if !connStopped {
		stopConn()
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return err
	}

	return nil
}

// formatLogEvent parses a JSON log event payload and returns a formatted terminal line.
// On parse failure it returns the raw payload so nothing is silently dropped.
func formatLogEvent(payload string) string {
	var evt logEvent
	if err := json.Unmarshal([]byte(payload), &evt); err != nil {
		return payload
	}
	return fmt.Sprintf("%s %s: %s", evt.Level, evt.Tag, evt.Msg)
}

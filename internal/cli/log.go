package cli

import (
	"bufio"
	"context"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/output"
	"github.com/fatih/color"
	"github.com/spf13/cobra"
)

var (
	logHosts   []string
	logAll     bool
	logTimeout int
)

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

	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(logTimeout)*time.Second)
	defer cancel()

	devices, err := discover.Browse(ctx)
	if err != nil {
		return err
	}

	if len(devices) == 0 {
		return fmt.Errorf("no devices found")
	}

	sort.Slice(devices, func(i, j int) bool {
		return devices[i].Hostname < devices[j].Hostname
	})

	var targetDevices []discover.DeviceInfo
	if logAll {
		targetDevices = devices
	} else {
		targetDevices = filterDevices(devices, logHosts)
		if len(targetDevices) == 0 {
			return fmt.Errorf("no matching devices found")
		}
	}

	ctx, cancel = signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()

	multi := len(targetDevices) > 1

	var wg sync.WaitGroup
	var mu sync.Mutex

	for i, device := range targetDevices {
		wg.Add(1)

		var prefixFn func(string) string
		if multi {
			c := color.New(logPalette[i%len(logPalette)])
			prefixFn = func(worker string) string {
				return c.Sprintf("[%s] ", worker)
			}
		}

		go func(d discover.DeviceInfo, pf func(string) string) {
			defer wg.Done()
			if err := streamDevice(ctx, d, pf, &mu); err != nil && ctx.Err() == nil {
				output.Error("[%s] %v", d.Worker, err)
			}
		}(device, prefixFn)
	}

	wg.Wait()
	return nil
}

func streamDevice(ctx context.Context, device discover.DeviceInfo, prefixFn func(string) string, mu *sync.Mutex) error {
	url := fmt.Sprintf("http://%s:%d/api/logs", device.IP, device.Port)

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return err
	}

	client := &http.Client{}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status: %d", resp.StatusCode)
	}

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()

		// Skip empty lines and SSE comments
		if line == "" || strings.HasPrefix(line, ":") {
			continue
		}

		// Strip "data: " prefix
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")

		if prefixFn != nil {
			mu.Lock()
			fmt.Println(prefixFn(device.Worker) + payload)
			mu.Unlock()
		} else {
			fmt.Println(payload)
		}
	}

	if err := scanner.Err(); err != nil && ctx.Err() == nil {
		return err
	}

	return nil
}

package cli

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/dangernoodle-io/taipan-cli/internal/discover"
	"github.com/dangernoodle-io/taipan-cli/internal/ui"
)

// directDevice builds a DeviceInfo for an explicit --host value, bypassing mDNS.
// Accepts "host" or "host:port"; defaults to port 80.
func directDevice(host string) discover.DeviceInfo {
	name := strings.TrimSuffix(strings.TrimSpace(host), ".")
	h, port := name, 80
	if hh, ps, err := net.SplitHostPort(name); err == nil {
		if p, perr := strconv.Atoi(ps); perr == nil {
			h, port = hh, p
		}
	}
	return discover.DeviceInfo{Hostname: h, IP: h, Port: port}
}

// resolveTargets returns the devices to act on. With explicit hosts (and not
// --all) it skips mDNS discovery and targets them directly; otherwise it browses
// for timeout seconds (then filters to hosts, unless all).
func resolveTargets(hosts []string, all bool, timeout int) ([]discover.DeviceInfo, error) {
	if !all && len(hosts) > 0 {
		out := make([]discover.DeviceInfo, 0, len(hosts))
		for _, h := range hosts {
			out = append(out, directDevice(h))
		}
		return out, nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Duration(timeout)*time.Second)
	defer cancel()
	stop := ui.Single("Discovering devices…")
	devices, err := discover.Browse(ctx)
	stop()
	if err != nil {
		return nil, err
	}
	if all {
		return devices, nil
	}
	return filterDevices(devices, hosts), nil
}

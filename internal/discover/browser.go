package discover

import (
	"context"
	"fmt"
	"sync"

	"github.com/grandcat/zeroconf"
)

// Browse discovers TaipanMiner devices via mDNS. The context deadline controls
// how long to listen for responses.
func Browse(ctx context.Context) ([]DeviceInfo, error) {
	resolver, err := zeroconf.NewResolver(nil)
	if err != nil {
		return nil, fmt.Errorf("creating mDNS resolver: %w", err)
	}

	entries := make(chan *zeroconf.ServiceEntry)
	if err = resolver.Browse(ctx, "_taipanminer._tcp", "local", entries); err != nil {
		return nil, fmt.Errorf("browsing mDNS: %w", err)
	}

	var (
		mu      sync.Mutex
		devices []DeviceInfo
		wg      sync.WaitGroup
	)

	for entry := range entries {
		wg.Add(1)
		go func(e *zeroconf.ServiceEntry) {
			defer wg.Done()
			info := deviceFromEntry(e)
			mu.Lock()
			devices = append(devices, info)
			mu.Unlock()
		}(entry)
	}

	wg.Wait()
	return devices, nil
}

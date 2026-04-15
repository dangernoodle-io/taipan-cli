package flash

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
	"os"
	"strings"
)

const (
	// appDescMagic is the magic word for esp_app_desc_t (esp_image_format.h)
	appDescMagic = 0xABCD5432
	// appDescOffset is the offset of esp_app_desc_t in the firmware binary
	appDescOffset = 0x20
	// appDescSize is the total size of the esp_app_desc_t struct
	appDescSize = 256
)

// FirmwareInfo contains parsed information from the firmware binary's app descriptor
type FirmwareInfo struct {
	ProjectName string
	Version     string
	IdfVersion  string
	Target      string // inferred from project name
}

// ParseFirmwareInfo reads and parses the app descriptor from a firmware binary
func ParseFirmwareInfo(binPath string) (*FirmwareInfo, error) {
	file, err := os.Open(binPath)
	if err != nil {
		return nil, fmt.Errorf("cannot open firmware: %w", err)
	}
	defer func() {
		_ = file.Close()
	}()

	// Read the app descriptor at offset 0x20
	buffer := make([]byte, appDescSize)
	n, err := file.ReadAt(buffer, appDescOffset)
	if err != nil && err != io.EOF {
		return nil, fmt.Errorf("cannot read app descriptor: %w", err)
	}
	if n < appDescSize {
		return nil, fmt.Errorf("firmware too short: expected %d bytes from offset 0x%x, got %d",
			appDescSize, appDescOffset, n)
	}

	// Verify magic word (little-endian)
	magic := binary.LittleEndian.Uint32(buffer[0:4])
	if magic != appDescMagic {
		return nil, fmt.Errorf("invalid app descriptor magic: got 0x%08x, expected 0x%08x",
			magic, appDescMagic)
	}

	// Parse fields (layout: magic(4) + secure_version(4) + reserved(8) + version[32] + project_name[32] + ...)
	// Version at offset 16, project_name at offset 48
	// Null-terminate strings by finding the first null byte
	versionBytes := buffer[16:48]
	if idx := bytes.IndexByte(versionBytes, 0); idx >= 0 {
		versionBytes = versionBytes[:idx]
	}

	projectNameBytes := buffer[48:80]
	if idx := bytes.IndexByte(projectNameBytes, 0); idx >= 0 {
		projectNameBytes = projectNameBytes[:idx]
	}

	idfVersionBytes := buffer[128:160]
	if idx := bytes.IndexByte(idfVersionBytes, 0); idx >= 0 {
		idfVersionBytes = idfVersionBytes[:idx]
	}

	projectName := string(projectNameBytes)
	version := string(versionBytes)
	idfVer := string(idfVersionBytes)

	// Extract target from project name (e.g., "taipanminer-bitdsk-n8t" -> "bitdsk-n8t")
	target := extractTarget(projectName)

	return &FirmwareInfo{
		ProjectName: projectName,
		Version:     version,
		IdfVersion:  idfVer,
		Target:      target,
	}, nil
}

// extractTarget extracts the board target from the project name.
// Expected format: <project>-<board> (e.g., "taipanminer-bitdsk-n8t" -> "bitdsk-n8t")
func extractTarget(projectName string) string {
	parts := strings.Split(projectName, "-")
	if len(parts) > 1 {
		return strings.Join(parts[1:], "-")
	}
	return projectName
}

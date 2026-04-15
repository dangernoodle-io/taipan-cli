package flash

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestBinary creates a minimal firmware binary with a valid app descriptor.
// The app descriptor is at offset 0x20, so we need at least 0x20 + 256 bytes.
func buildTestBinary(t *testing.T, projectName, version, idfVer string) []byte {
	buffer := make([]byte, 0x20+appDescSize)

	// Write magic at offset 0x20 (relative to file start)
	binary.LittleEndian.PutUint32(buffer[0x20:0x24], appDescMagic)

	// Write version at offset 0x20 + 16 (after magic(4) + secure_version(4) + reserved(8))
	copy(buffer[0x20+16:0x20+48], []byte(version))

	// Write project_name at offset 0x20 + 48
	copy(buffer[0x20+48:0x20+80], []byte(projectName))

	// Write idf_ver at offset 0x20 + 128
	copy(buffer[0x20+128:0x20+160], []byte(idfVer))

	return buffer
}

func TestParseFirmwareInfo_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test.bin")

	// Create a test binary with valid app descriptor
	binData := buildTestBinary(t, "taipanminer-bitaxe-601", "v1.0.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	info, err := ParseFirmwareInfo(binPath)
	require.NoError(t, err)

	assert.Equal(t, "taipanminer-bitaxe-601", info.ProjectName)
	assert.Equal(t, "v1.0.0", info.Version)
	assert.Equal(t, "v4.4.0", info.IdfVersion)
	assert.Equal(t, "bitaxe-601", info.Target)
}

func TestParseFirmwareInfo_TruncatedFile(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "truncated.bin")

	// Create a file that has some data but not enough at offset 0x20
	truncated := make([]byte, 0x20+100)
	require.NoError(t, os.WriteFile(binPath, truncated, 0o644))

	_, err := ParseFirmwareInfo(binPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "firmware too short")
}

func TestParseFirmwareInfo_BadMagic(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "badmagic.bin")

	binData := buildTestBinary(t, "test", "v1.0.0", "v4.4.0")
	// Corrupt the magic at the correct offset
	binary.LittleEndian.PutUint32(binData[0x20:0x24], 0xDEADBEEF)
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	_, err := ParseFirmwareInfo(binPath)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid app descriptor magic")
}

func TestExtractTarget(t *testing.T) {
	tests := []struct {
		projectName string
		expected    string
	}{
		{"taipanminer-bitaxe-601", "bitaxe-601"},
		{"taipanminer-bitdsk-n8t", "bitdsk-n8t"},
		{"taipanminer-tdongle-s3", "tdongle-s3"},
		{"custom-project-name", "project-name"},
		{"single", "single"},
	}

	for _, tc := range tests {
		t.Run(tc.projectName, func(t *testing.T) {
			result := extractTarget(tc.projectName)
			assert.Equal(t, tc.expected, result)
		})
	}
}

func TestParseFirmwareInfo_NullTerminatedStrings(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "nullterm.bin")

	buffer := make([]byte, 0x20+appDescSize)
	binary.LittleEndian.PutUint32(buffer[0x20:0x24], appDescMagic)

	// Write null-terminated strings at proper offsets
	copy(buffer[0x20+16:], []byte("v2.0.0\x00extra_data"))
	copy(buffer[0x20+48:], []byte("taipanminer-bitaxe-403\x00more_data"))
	copy(buffer[0x20+128:], []byte("v5.0.0\x00"))

	require.NoError(t, os.WriteFile(binPath, buffer, 0o644))

	info, err := ParseFirmwareInfo(binPath)
	require.NoError(t, err)

	// Strings should be trimmed at null terminators
	assert.Equal(t, "v2.0.0", info.Version)
	assert.Equal(t, "taipanminer-bitaxe-403", info.ProjectName)
	assert.Equal(t, "v5.0.0", info.IdfVersion)
}

func TestParseFirmwareInfo_NonExistentFile(t *testing.T) {
	_, err := ParseFirmwareInfo("/nonexistent/path.bin")
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot open firmware")
}

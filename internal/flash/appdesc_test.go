package flash

import (
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// buildTestBinary creates a minimal OTA firmware binary with a valid app descriptor at 0x20.
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

// buildTestFactoryBinary creates a minimal factory firmware binary with a valid app descriptor at 0x20020.
func buildTestFactoryBinary(t *testing.T, projectName, version, idfVer string) []byte {
	buffer := make([]byte, 0x20020+appDescSize)
	// Fill with zeros (simulating bootloader, partition table, etc.)

	// Write magic at offset 0x20020 (relative to file start)
	binary.LittleEndian.PutUint32(buffer[0x20020:0x20024], appDescMagic)

	// Write version at offset 0x20020 + 16
	copy(buffer[0x20020+16:0x20020+48], []byte(version))

	// Write project_name at offset 0x20020 + 48
	copy(buffer[0x20020+48:0x20020+80], []byte(projectName))

	// Write idf_ver at offset 0x20020 + 128
	copy(buffer[0x20020+128:0x20020+160], []byte(idfVer))

	return buffer
}

func TestParseFirmwareInfo_Valid(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "test.bin")

	// Create a test binary with valid app descriptor
	binData := buildTestBinary(t, "taipanminer-bitaxe-601", "v1.0.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	info, err := ParseFirmwareInfo(binPath, false)
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

	_, err := ParseFirmwareInfo(binPath, false)
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

	_, err := ParseFirmwareInfo(binPath, false)
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

	info, err := ParseFirmwareInfo(binPath, false)
	require.NoError(t, err)

	// Strings should be trimmed at null terminators
	assert.Equal(t, "v2.0.0", info.Version)
	assert.Equal(t, "taipanminer-bitaxe-403", info.ProjectName)
	assert.Equal(t, "v5.0.0", info.IdfVersion)
}

func TestParseFirmwareInfo_NonExistentFile(t *testing.T) {
	_, err := ParseFirmwareInfo("/nonexistent/path.bin", false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "cannot open firmware")
}

func TestParseFirmwareInfo_Factory(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "factory.bin")

	// Create a factory binary with app descriptor at 0x20020
	binData := buildTestFactoryBinary(t, "taipanminer-tdongle-s3", "v1.5.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	info, err := ParseFirmwareInfo(binPath, true)
	require.NoError(t, err)

	assert.Equal(t, "taipanminer-tdongle-s3", info.ProjectName)
	assert.Equal(t, "v1.5.0", info.Version)
	assert.Equal(t, "v4.4.0", info.IdfVersion)
	assert.Equal(t, "tdongle-s3", info.Target)
}

func TestParseFirmwareInfo_MismatchedType_OTAAsFactory(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "ota.bin")

	// Create an OTA binary (magic at 0x20) but try to parse as factory (expecting magic at 0x20020)
	binData := buildTestBinary(t, "taipanminer-tdongle-s3", "v1.0.0", "v4.4.0")
	// Pad to 0x20020 + 256 so the factory parse attempt has enough data to check the alternate offset
	padded := make([]byte, 0x20020+appDescSize)
	copy(padded, binData)
	require.NoError(t, os.WriteFile(binPath, padded, 0o644))

	_, err := ParseFirmwareInfo(binPath, true)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "OTA image")
}

func TestParseFirmwareInfo_MismatchedType_FactoryAsOTA(t *testing.T) {
	tmpDir := t.TempDir()
	binPath := filepath.Join(tmpDir, "factory.bin")

	// Create a factory binary (magic at 0x20020) but try to parse as OTA (expecting magic at 0x20)
	binData := buildTestFactoryBinary(t, "taipanminer-bitaxe-601", "v1.5.0", "v4.4.0")
	require.NoError(t, os.WriteFile(binPath, binData, 0o644))

	_, err := ParseFirmwareInfo(binPath, false)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "factory image")
}

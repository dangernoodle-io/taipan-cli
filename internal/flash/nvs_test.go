package flash

import (
	"encoding/binary"
	"hash/crc32"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestGenerateNVS_ValidSize(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "taipanminer",
			Key:       "provisioned",
			Type:      "u8",
			Value:     uint8(1),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)
	assert.Equal(t, totalPartitionSize, len(data))
}

func TestGenerateNVS_InvalidSize(t *testing.T) {
	entries := []NVSEntry{}
	_, err := GenerateNVS(entries, 0x5000)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid partition size")
}

func TestGenerateNVS_U8Entry(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "taipanminer",
			Key:       "provisioned",
			Type:      "u8",
			Value:     uint8(1),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Verify partition is filled with correct data
	assert.NotNil(t, data)

	// Check first page header
	pageHeader := data[0:32]
	assert.Equal(t, uint8(0xFE), pageHeader[0], "first byte should be page state active")
	assert.Equal(t, uint8(0xFE), pageHeader[8], "byte 8 should be version")

	// Verify CRC32 in header (ESP-IDF uses raw register, no final XOR)
	headerCrc := binary.LittleEndian.Uint32(pageHeader[28:32])
	computedCrc := crc32.Update(0xFFFFFFFF, crc32.IEEETable,pageHeader[4:28])
	assert.Equal(t, computedCrc, headerCrc, "header CRC32 should match")
}

func TestGenerateNVS_U16Entry(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "taipanminer",
			Key:       "pool_port",
			Type:      "u16",
			Value:     uint16(3337),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Verify u16 is stored correctly in entry data
	assert.NotNil(t, data)
	assert.Equal(t, totalPartitionSize, len(data))

	// Find the pool_port entry (should be second in first page, after namespace entry)
	entryOffset := firstEntryOffset + entrySize // Skip namespace entry
	entryData := data[entryOffset : entryOffset+entrySize]

	// Bytes 24-25 should contain the u16 value
	val := binary.LittleEndian.Uint16(entryData[24:26])
	assert.Equal(t, uint16(3337), val, "u16 value should be correctly encoded")
}

func TestGenerateNVS_StringEntry(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "taipanminer",
			Key:       "wifi_ssid",
			Type:      "string",
			Value:     "test-network",
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Verify string entry exists and is properly encoded
	assert.NotNil(t, data)
	assert.Equal(t, totalPartitionSize, len(data))

	// Find string entry (should be second in first page after namespace)
	entryOffset := firstEntryOffset + entrySize
	entryData := data[entryOffset : entryOffset+entrySize]

	// Byte 1 should be string type (SZ = 0x21)
	assert.Equal(t, uint8(0x21), entryData[1], "entry type should be string")

	// Bytes 24-25 should contain string length including null terminator
	strLen := binary.LittleEndian.Uint16(entryData[24:26])
	assert.Equal(t, uint16(13), strLen, "string length should be 13 (12 chars + null)")
}

func TestGenerateNVS_TaipanminerConfig(t *testing.T) {
	// Test with all taipanminer configuration entries
	entries := []NVSEntry{
		{
			Namespace: "taipanminer",
			Key:       "wifi_ssid",
			Type:      "string",
			Value:     "test-network",
		},
		{
			Namespace: "taipanminer",
			Key:       "wifi_pass",
			Type:      "string",
			Value:     "test-pass-123",
		},
		{
			Namespace: "taipanminer",
			Key:       "pool_host",
			Type:      "string",
			Value:     "pool.example.com",
		},
		{
			Namespace: "taipanminer",
			Key:       "pool_port",
			Type:      "u16",
			Value:     uint16(3337),
		},
		{
			Namespace: "taipanminer",
			Key:       "wallet_addr",
			Type:      "string",
			Value:     "1TestWalletAddr123456789012345",
		},
		{
			Namespace: "taipanminer",
			Key:       "worker",
			Type:      "string",
			Value:     "test-worker-1",
		},
		{
			Namespace: "taipanminer",
			Key:       "pool_pass",
			Type:      "string",
			Value:     "x",
		},
		{
			Namespace: "taipanminer",
			Key:       "provisioned",
			Type:      "u8",
			Value:     uint8(1),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Verify all entries are present by checking for key strings
	expectedKeys := []string{
		"wifi_ssid",
		"wifi_pass",
		"pool_host",
		"pool_port",
		"wallet_addr",
		"worker",
		"pool_pass",
		"provisioned",
	}

	for _, key := range expectedKeys {
		found := false
		for i := 0; i < len(data)-16; i++ {
			// Look for the key string in the binary
			if string(data[i:i+len(key)]) == key {
				found = true
				break
			}
		}
		assert.True(t, found, "key %q should be present in binary", key)
	}
}

func TestGenerateNVS_PageHeader(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "key1",
			Type:      "u8",
			Value:     uint8(42),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Check first page header
	header := data[0:32]

	// Byte 0: state
	assert.Equal(t, uint8(0xFE), header[0])

	// Bytes 1-3: reserved
	assert.Equal(t, byte(0xFF), header[1])
	assert.Equal(t, byte(0xFF), header[2])
	assert.Equal(t, byte(0xFF), header[3])

	// Bytes 4-7: sequence number
	seqNum := binary.LittleEndian.Uint32(header[4:8])
	assert.Equal(t, uint32(0), seqNum, "first page should have sequence 0")

	// Byte 8: version
	assert.Equal(t, uint8(0xFE), header[8])

	// Bytes 9-27: should be 0xFF
	for i := 9; i < 28; i++ {
		assert.Equal(t, byte(0xFF), header[i], "byte %d should be 0xFF", i)
	}

	// Bytes 28-31: CRC32 (ESP-IDF compatible — no final XOR)
	crc := binary.LittleEndian.Uint32(header[28:32])
	computedCrc := crc32.Update(0xFFFFFFFF, crc32.IEEETable,header[4:28])
	assert.Equal(t, computedCrc, crc)
}

func TestGenerateNVS_EntryBitmap(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "key1",
			Type:      "u8",
			Value:     uint8(1),
		},
		{
			Namespace: "test",
			Key:       "key2",
			Type:      "u8",
			Value:     uint8(2),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Check bitmap (bytes 32-63)
	// Should have entries marked as written (0x02 per 2 bits)
	bitmap := data[32:64]

	// First entry (namespace): bits 0-1 of byte 32
	byte0 := bitmap[0]
	firstEntryState := byte0 & 0x03
	assert.Equal(t, uint8(entryStateWritten), firstEntryState, "first entry should be marked written")

	// Second entry: bits 2-3 of byte 32
	secondEntryState := (byte0 >> 2) & 0x03
	assert.Equal(t, uint8(entryStateWritten), secondEntryState, "second entry should be marked written")
}

func TestGenerateNVS_StringSpan(t *testing.T) {
	// Test string that requires multiple entry spans (> 24 bytes of data)
	longString := "this-is-a-very-long-test-string-that-spans-multiple-entries-and-should-be-encoded-correctly"

	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "longkey",
			Type:      "string",
			Value:     longString,
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	assert.NotNil(t, data)
	assert.True(t, len(data) == totalPartitionSize)

	// Verify string header entry has correct span
	entryOffset := firstEntryOffset + entrySize // Skip namespace
	entryData := data[entryOffset : entryOffset+entrySize]
	span := entryData[2]

	// span = 1 (header) + ceil(strLen/entrySize) data entries
	dataEntries := (len(longString) + 1 + entrySize - 1) / entrySize
	if dataEntries == 0 {
		dataEntries = 1
	}
	expectedSpan := uint8(1 + dataEntries)
	assert.Equal(t, expectedSpan, span, "span should be calculated correctly for long string")
}

func TestGenerateNVS_EntryCRC32(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "key1",
			Type:      "u8",
			Value:     uint8(42),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Get the data entry (skip namespace entry)
	entryOffset := firstEntryOffset + entrySize
	entryData := data[entryOffset : entryOffset+entrySize]

	// Bytes 4-7: stored CRC32
	storedCrc := binary.LittleEndian.Uint32(entryData[4:8])

	// Recompute CRC32: bytes 0-3 (nsIndex, type, span, chunkIndex) + bytes 8-31 (key + data)
	// ESP-IDF uses raw register value (no final XOR)
	crcInput := make([]byte, 28)
	copy(crcInput[0:4], entryData[0:4])
	copy(crcInput[4:28], entryData[8:32])
	computedCrc := crc32.Update(0xFFFFFFFF, crc32.IEEETable,crcInput)

	assert.Equal(t, computedCrc, storedCrc, "entry CRC32 should match computed value")
}

func TestGenerateNVS_KeyNullTerminated(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "mykey",
			Type:      "u8",
			Value:     uint8(99),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Get data entry (skip namespace)
	entryOffset := firstEntryOffset + entrySize
	entryData := data[entryOffset : entryOffset+entrySize]

	// Bytes 8-23: key (16 bytes)
	key := entryData[8:24]
	assert.Equal(t, byte('m'), key[0])
	assert.Equal(t, byte('y'), key[1])
	assert.Equal(t, byte('k'), key[2])
	assert.Equal(t, byte('e'), key[3])
	assert.Equal(t, byte('y'), key[4])
	assert.Equal(t, byte(0), key[5], "key should be null-terminated")
}

func TestGenerateNVS_MultipleNamespaces(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "ns1",
			Key:       "key1",
			Type:      "u8",
			Value:     uint8(1),
		},
		{
			Namespace: "ns2",
			Key:       "key2",
			Type:      "u16",
			Value:     uint16(2),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Should have entries for both namespaces
	assert.True(t, len(data) == totalPartitionSize)

	// Verify both namespace names are present
	binaryStr := string(data)
	assert.Contains(t, binaryStr, "ns1")
	assert.Contains(t, binaryStr, "ns2")
}

func TestGenerateNVS_InvalidU8Value(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "key",
			Type:      "u8",
			Value:     "invalid",
		},
	}

	_, err := GenerateNVS(entries, totalPartitionSize)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid u8 value")
}

func TestGenerateNVS_InvalidU16Value(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "key",
			Type:      "u16",
			Value:     "invalid",
		},
	}

	_, err := GenerateNVS(entries, totalPartitionSize)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid u16 value")
}

func TestGenerateNVS_InvalidStringValue(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "key",
			Type:      "string",
			Value:     42,
		},
	}

	_, err := GenerateNVS(entries, totalPartitionSize)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid string value")
}

func TestGenerateNVS_UnknownType(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "key",
			Type:      "unknown",
			Value:     "value",
		},
	}

	_, err := GenerateNVS(entries, totalPartitionSize)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "unknown entry type")
}

func TestGenerateNVS_IntegerConversion(t *testing.T) {
	// Test that int values can be converted to u8 and u16
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "u8key",
			Type:      "u8",
			Value:     42,
		},
		{
			Namespace: "test",
			Key:       "u16key",
			Type:      "u16",
			Value:     3337,
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)
	assert.NotNil(t, data)
}

func TestGenerateNVS_PageFill(t *testing.T) {
	// Verify that unwritten areas are filled with 0xFF
	entries := []NVSEntry{
		{
			Namespace: "test",
			Key:       "key",
			Type:      "u8",
			Value:     uint8(1),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// Check that bytes after entries in first page are 0xFF
	firstPageEnd := pageSize
	// After header (32) + bitmap (32) + namespace entry (32) + data entry (32) = 128 bytes
	entryDataStart := firstEntryOffset + 2*entrySize

	// Random samples in unwritten area should be 0xFF
	assert.Equal(t, byte(0xFF), data[entryDataStart+100])
	assert.Equal(t, byte(0xFF), data[entryDataStart+200])
	assert.Equal(t, byte(0xFF), data[firstPageEnd-1])
}

func TestGenerateNVS_NamespaceEntry(t *testing.T) {
	entries := []NVSEntry{
		{
			Namespace: "taipanminer",
			Key:       "key1",
			Type:      "u8",
			Value:     uint8(1),
		},
	}

	data, err := GenerateNVS(entries, totalPartitionSize)
	require.NoError(t, err)

	// First entry should be namespace entry
	nsEntryOffset := firstEntryOffset
	nsEntry := data[nsEntryOffset : nsEntryOffset+entrySize]

	// Byte 0: namespace index should be 0 for namespace entry
	assert.Equal(t, uint8(0), nsEntry[0], "namespace entry should have nsidx=0")

	// Byte 1: type should be U8 (0x01) — namespace entries store their index as a U8 value
	assert.Equal(t, uint8(0x01), nsEntry[1], "namespace entry should have type=U8")

	// Byte 24: data should contain namespace index (1 for first namespace)
	assert.Equal(t, uint8(1), nsEntry[24], "namespace entry data should contain namespace index")

	// Bytes 8-23: should contain namespace name
	assert.Equal(t, byte('t'), nsEntry[8])
	assert.Equal(t, byte('a'), nsEntry[9])
	assert.Equal(t, byte('i'), nsEntry[10])
}

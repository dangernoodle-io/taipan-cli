package flash

import (
	"encoding/binary"
	"fmt"
	"hash/crc32"
	"math"
)

const (
	// NVS partition constants
	pageSize              = 4096
	headerSize            = 32
	bitmapSize            = 32
	entrySize             = 32
	entriesPerPage        = 126
	totalPages            = 6
	totalPartitionSize    = pageSize * totalPages
	firstEntryOffset      = 64 // headerSize + bitmapSize
	pageStateActive       = 0xFE
	pageStateEmpty        = 0xFF
	pageVersion           = 0xFE // v2
	maxKeyLen             = 15
	stringDataSize        = 24 // Bytes 8-31 in entry (excluding key)
	namespaceEntryType    = 0x01 // namespace entries use U8 type with value = namespace index
	u8Type                = 0x01
	u16Type               = 0x02
	stringType            = 0x21 // SZ (null-terminated string)
	singleChunkIndex      = 0xFF
	spanOne               = 1
	entryStateEmpty   = 0x03 // 0b11
	entryStateWritten = 0x02 // 0b10
	entryStateErased  = 0x00 // 0b00
)

// NVSEntry represents a key-value pair to write to NVS
type NVSEntry struct {
	Namespace string
	Key       string
	Type      string // "u8", "u16", "string"
	Value     interface{}
}

// entry represents an internal NVS entry with computed fields
type entry struct {
	namespaceIdx uint8
	entryType    uint8
	span         uint8
	chunkIndex   uint8
	key          [16]byte
	data         [8]byte
	crc32Val     uint32
	rawData      []byte // For multi-span strings
}

// newEntry creates an entry with data field pre-filled with 0xFF,
// matching ESP-IDF's convention for unused bytes.
func newEntry() *entry {
	e := &entry{}
	for i := range e.data {
		e.data[i] = 0xFF
	}
	return e
}

// GenerateNVS creates an NVS partition binary from entries
func GenerateNVS(entries []NVSEntry, partitionSize int) ([]byte, error) {
	if partitionSize != totalPartitionSize {
		return nil, fmt.Errorf("invalid partition size: expected 0x%x, got 0x%x", totalPartitionSize, partitionSize)
	}

	// Create partition buffer filled with 0xFF
	partition := make([]byte, partitionSize)
	for i := range partition {
		partition[i] = 0xFF
	}

	// Group entries by namespace
	namespaceMap := make(map[string][]*NVSEntry)
	for i, e := range entries {
		namespaceMap[e.Namespace] = append(namespaceMap[e.Namespace], &entries[i])
	}

	// Process each namespace
	pageIdx := 0
	nsCounter := uint8(0)
	for ns, nsEntries := range namespaceMap {
		nsCounter++
		// Write namespace entry first — type is U8 with data = namespace index
		nsEntry := newEntry()
		nsEntry.namespaceIdx = 0
		nsEntry.entryType = namespaceEntryType
		nsEntry.span = spanOne
		nsEntry.chunkIndex = singleChunkIndex
		copyKeyToEntry(ns, nsEntry)
		nsEntry.data[0] = nsCounter // namespace index stored as U8 value
		nsEntry.crc32Val = calculateEntryCRC32(nsEntry)

		// Collect all entries for this namespace
		var entriesToWrite []*entry
		entriesToWrite = append(entriesToWrite, nsEntry)

		for _, nvse := range nsEntries {
			e, err := parseNVSEntry(nvse, nsCounter) // use the namespace index
			if err != nil {
				return nil, err
			}
			entriesToWrite = append(entriesToWrite, e...)
		}

		// Write entries to pages
		pageIdx += writePage(&partition, pageIdx, uint32(pageIdx), entriesToWrite)
	}

	return partition, nil
}

// parseNVSEntry converts an NVSEntry to one or more internal entries (for string spanning)
func parseNVSEntry(nvse *NVSEntry, namespaceIdx uint8) ([]*entry, error) {
	var result []*entry

	switch nvse.Type {
	case "u8":
		val, ok := nvse.Value.(uint8)
		if !ok {
			// Try to convert from int or other numeric types
			if iv, ok := nvse.Value.(int); ok {
				if iv < 0 || iv > 255 {
					return nil, fmt.Errorf("u8 value out of range: %v", iv)
				}
				val = uint8(iv)
			} else {
				return nil, fmt.Errorf("invalid u8 value: %v", nvse.Value)
			}
		}
		e := newEntry()
		e.namespaceIdx = namespaceIdx
		e.entryType = u8Type
		e.span = spanOne
		e.chunkIndex = singleChunkIndex
		copyKeyToEntry(nvse.Key, e)
		e.data[0] = val
		e.crc32Val = calculateEntryCRC32(e)
		result = append(result, e)

	case "u16":
		val, ok := nvse.Value.(uint16)
		if !ok {
			// Try to convert from int or other numeric types
			if iv, ok := nvse.Value.(int); ok {
				if iv < 0 || iv > 65535 {
					return nil, fmt.Errorf("u16 value out of range: %v", iv)
				}
				val = uint16(iv)
			} else {
				return nil, fmt.Errorf("invalid u16 value: %v", nvse.Value)
			}
		}
		e := newEntry()
		e.namespaceIdx = namespaceIdx
		e.entryType = u16Type
		e.span = spanOne
		e.chunkIndex = singleChunkIndex
		copyKeyToEntry(nvse.Key, e)
		binary.LittleEndian.PutUint16(e.data[0:2], val)
		e.crc32Val = calculateEntryCRC32(e)
		result = append(result, e)

	case "string":
		str, ok := nvse.Value.(string)
		if !ok {
			return nil, fmt.Errorf("invalid string value: %v", nvse.Value)
		}
		strBytes := append([]byte(str), 0) // Add null terminator
		strLen := len(strBytes)

		// Calculate span: 1 header entry + ceil(strLen / entrySize) data entries
		dataEntries := int(math.Ceil(float64(strLen) / float64(entrySize)))
		if dataEntries == 0 {
			dataEntries = 1
		}
		span := uint8(1 + dataEntries) // header + data entries

		// Create header entry
		e := newEntry()
		e.namespaceIdx = namespaceIdx
		e.entryType = stringType
		e.span = span
		e.chunkIndex = singleChunkIndex
		copyKeyToEntry(nvse.Key, e)
		binary.LittleEndian.PutUint16(e.data[0:2], uint16(strLen))
		// data[2:3] already 0xFF from newEntry() (reserved field)
		// rawDataCrc32 will be calculated after all spans are created
		e.rawData = strBytes
		// Calculate string data CRC and store in header entry
		stringCrc := calculateStringCRC32(strBytes)
		binary.LittleEndian.PutUint32(e.data[4:8], stringCrc)
		e.crc32Val = calculateEntryCRC32(e)
		result = append(result, e)

	default:
		return nil, fmt.Errorf("unknown entry type: %s", nvse.Type)
	}

	return result, nil
}

// writePage writes entries to a single page and returns number of pages written
func writePage(partition *[]byte, pageNum int, seqNum uint32, entries []*entry) int {
	if pageNum >= totalPages {
		return 0
	}

	pageOffset := pageNum * pageSize
	page := (*partition)[pageOffset : pageOffset+pageSize]

	// Initialize page with 0xFF
	for i := range page {
		page[i] = 0xFF
	}

	// Write header
	writePageHeader(page, seqNum)

	// Write entry bitmap and entries
	bitmapOffset := headerSize
	entriesWritten := 0

	slotIdx := 0
	for _, e := range entries {
		if slotIdx >= entriesPerPage {
			// Need another page — but this is a simplification; in practice
			// our entries fit in one page
			break
		}

		// Mark slot as written in bitmap
		markBitmapWritten(page, bitmapOffset, slotIdx)

		// Write the entry header
		entryOffset := firstEntryOffset + slotIdx*entrySize
		writeEntry(page[entryOffset:entryOffset+entrySize], e)
		slotIdx++
		entriesWritten++

		// For string entries, write raw data into subsequent slots
		if e.rawData != nil {
			dataSlots := int(e.span) - 1 // header already written
			for ds := 0; ds < dataSlots; ds++ {
				if slotIdx >= entriesPerPage {
					break
				}
				markBitmapWritten(page, bitmapOffset, slotIdx)
				dataOffset := firstEntryOffset + slotIdx*entrySize
				// Copy chunk of raw data, rest stays 0xFF
				start := ds * entrySize
				end := start + entrySize
				if end > len(e.rawData) {
					end = len(e.rawData)
				}
				if start < len(e.rawData) {
					copy(page[dataOffset:dataOffset+entrySize], e.rawData[start:end])
				}
				slotIdx++
			}
		}
	}

	return 1
}

// markBitmapWritten sets the 2-bit entry state to "written" (0b10) in the bitmap.
func markBitmapWritten(page []byte, bitmapOffset int, slotIdx int) {
	bitIndex := uint(slotIdx) * 2
	byteIdx := bitmapOffset + int(bitIndex/8)
	bitOffset := bitIndex % 8
	mask := uint8(0x3) << bitOffset
	page[byteIdx] = (page[byteIdx] &^ mask) | ((entryStateWritten & 0x3) << bitOffset)
}

// writePageHeader writes the NVS page header
func writePageHeader(page []byte, seqNum uint32) {
	// Byte 0: state
	page[0] = pageStateActive

	// Bytes 1-3: reserved (0xFF)
	page[1] = 0xFF
	page[2] = 0xFF
	page[3] = 0xFF

	// Bytes 4-7: sequence number (uint32 LE)
	binary.LittleEndian.PutUint32(page[4:8], seqNum)

	// Byte 8: version
	page[8] = pageVersion

	// Bytes 9-27: reserved (0xFF)
	for i := 9; i < 28; i++ {
		page[i] = 0xFF
	}

	// Bytes 28-31: CRC32 of bytes 4-27
	binary.LittleEndian.PutUint32(page[28:32], espCRC32(page[4:28]))
}

// writeEntry writes an entry to the page
func writeEntry(entrySpace []byte, e *entry) {
	entrySpace[0] = e.namespaceIdx
	entrySpace[1] = e.entryType
	entrySpace[2] = e.span
	entrySpace[3] = e.chunkIndex

	// Bytes 4-7: CRC32
	binary.LittleEndian.PutUint32(entrySpace[4:8], e.crc32Val)

	// Bytes 8-23: key (16 bytes)
	copy(entrySpace[8:24], e.key[:])

	// Bytes 24-31: data (8 bytes)
	copy(entrySpace[24:32], e.data[:])
}

// copyKeyToEntry copies a key string to an entry, null-terminated
func copyKeyToEntry(key string, e *entry) {
	if len(key) > maxKeyLen {
		key = key[:maxKeyLen]
	}
	copy(e.key[:], key)
	e.key[len(key)] = 0 // Null terminate
}

// espCRC32 computes CRC32 matching ESP-IDF's esp_rom_crc32_le(0xFFFFFFFF, data, len)
// and Python's zlib.crc32(data, 0xFFFFFFFF). Go's crc32.Update with initial value
// 0xFFFFFFFF produces the same result because both use seed XOR then process.
func espCRC32(data []byte) uint32 {
	return crc32.Update(0xFFFFFFFF, crc32.IEEETable, data)
}

// calculateEntryCRC32 calculates CRC32 for an entry.
// Covers: nsIndex(1) + type(1) + span(1) + chunkIndex(1) + key(16) + data(8) = 28 bytes.
func calculateEntryCRC32(e *entry) uint32 {
	buf := make([]byte, 28)
	buf[0] = e.namespaceIdx
	buf[1] = e.entryType
	buf[2] = e.span
	buf[3] = e.chunkIndex
	copy(buf[4:20], e.key[:])
	copy(buf[20:28], e.data[:])
	return espCRC32(buf)
}

// calculateStringCRC32 calculates CRC32 for the raw string data (including null terminator).
func calculateStringCRC32(strBytes []byte) uint32 {
	return espCRC32(strBytes)
}

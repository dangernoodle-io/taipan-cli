package flash

import (
	"fmt"

	espflasher "tinygo.org/x/espflasher/pkg/espflasher"
)

// boardChipMap maps board names to chip types
var boardChipMap = map[string]espflasher.ChipType{
	"bitaxe-601": espflasher.ChipESP32S3,
	"bitaxe-403": espflasher.ChipESP32S3,
	"tdongle-s3": espflasher.ChipESP32S3,
	"bitdsk-n8t": espflasher.ChipESP32C3,
}

// ChipForBoard returns the espflasher.ChipType for the given board name.
// Returns an error if the board is not recognized.
func ChipForBoard(board string) (espflasher.ChipType, error) {
	chipType, ok := boardChipMap[board]
	if !ok {
		return 0, fmt.Errorf("unsupported board: %s", board)
	}
	return chipType, nil
}

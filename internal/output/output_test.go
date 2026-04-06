package output

import (
	"testing"

	"github.com/fatih/color"
	"github.com/stretchr/testify/assert"
)

func TestDisable(t *testing.T) {
	// Save original value
	originalNoColor := color.NoColor
	defer func() { color.NoColor = originalNoColor }()

	// Ensure it starts false
	color.NoColor = false

	// Call Disable
	Disable()

	// Assert NoColor is now true
	assert.Equal(t, true, color.NoColor)
}

package output

import (
	"fmt"
	"os"

	"github.com/fatih/color"
)

var (
	errorColor   = color.New(color.FgRed)
	warnColor    = color.New(color.FgYellow)
	successColor = color.New(color.FgGreen)
)

func Error(format string, a ...interface{}) {
	_, _ = fmt.Fprintln(os.Stderr, errorColor.Sprintf(format, a...))
}

func Warn(format string, a ...interface{}) {
	_, _ = fmt.Fprintln(os.Stderr, warnColor.Sprintf(format, a...))
}

func Success(format string, a ...interface{}) {
	_, _ = fmt.Fprintln(os.Stdout, successColor.Sprintf(format, a...))
}

func Info(format string, a ...interface{}) {
	_, _ = fmt.Fprintln(os.Stdout, fmt.Sprintf(format, a...))
}

// Disable turns off all color output.
func Disable() {
	color.NoColor = true
}

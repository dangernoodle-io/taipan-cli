package ui

import (
	"os"
	"time"

	"github.com/briandowns/spinner"
	"github.com/fatih/color"
	"github.com/mattn/go-isatty"
)

var enabled = true

// SetEnabled toggles spinners globally (root sets false on --no-color/NO_COLOR;
// commands set false on --json).
func SetEnabled(v bool) { enabled = v }

// isTTY is a var so tests can override.
var isTTY = func() bool { return isatty.IsTerminal(os.Stderr.Fd()) }

func active() bool { return enabled && isTTY() }

// Single starts a transient spinner on stderr and returns a stop func that
// clears the line. No-op (returns a no-op stop) when stderr isn't a TTY or
// spinners are disabled. Spinners only start when active() (stderr is a TTY)
// and the user hasn't disabled color, so forcing color on for the spinner's
// lifetime is safe; we restore the prior color.NoColor on stop so result
// output is unaffected.
func Single(msg string) func() {
	if !active() {
		return func() {}
	}
	prev := color.NoColor
	color.NoColor = false
	s := spinner.New(spinner.CharSets[14], 80*time.Millisecond, spinner.WithWriter(os.Stderr))
	_ = s.Color("cyan")
	s.Suffix = " " + msg
	s.Start()
	return func() {
		s.Stop()
		color.NoColor = prev
	}
}

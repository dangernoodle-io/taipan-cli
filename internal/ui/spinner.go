package ui

import (
	"os"

	"github.com/chelnak/ysmrr"
	"github.com/mattn/go-isatty"
)

var enabled = true

// SetEnabled toggles spinners globally (root sets false on --no-color/NO_COLOR;
// commands set false on --json).
func SetEnabled(v bool) { enabled = v }

// isTTY is a var so tests can override.
var isTTY = func() bool { return isatty.IsTerminal(os.Stderr.Fd()) }

func active() bool { return enabled && isTTY() }

// Group manages a set of related spinners.
type Group struct {
	sm ysmrr.SpinnerManager
	on bool
}

// NewGroup creates a new spinner group. When inactive (not a TTY or disabled),
// all methods are safe no-ops that produce no output.
func NewGroup() *Group {
	if !active() {
		return &Group{on: false}
	}
	sm := ysmrr.NewSpinnerManager(ysmrr.WithWriter(os.Stderr))
	return &Group{sm: sm, on: true}
}

// Line wraps a single spinner line.
type Line struct{ s *ysmrr.Spinner }

// Add adds a new spinner line with the given message.
func (g *Group) Add(msg string) *Line {
	if !g.on {
		return &Line{}
	}
	return &Line{s: g.sm.AddSpinner(msg)}
}

// Start starts all spinners in the group.
func (g *Group) Start() {
	if g.on {
		g.sm.Start()
	}
}

// Stop stops all spinners in the group.
func (g *Group) Stop() {
	if g.on {
		g.sm.Stop()
	}
}

// Update updates the spinner message.
func (l *Line) Update(msg string) {
	if l.s != nil {
		l.s.UpdateMessage(msg)
	}
}

// Complete marks the spinner as complete with a message.
func (l *Line) Complete(msg string) {
	if l.s != nil {
		l.s.CompleteWithMessage(msg)
	}
}

// Error marks the spinner as error with a message.
func (l *Line) Error(msg string) {
	if l.s != nil {
		l.s.ErrorWithMessage(msg)
	}
}

// Single is a one-line spinner convenience; returns a stop func (no-op when inactive).
func Single(msg string) func() {
	g := NewGroup()
	if !g.on {
		return func() {}
	}
	g.Add(msg)
	g.Start()
	return g.Stop
}

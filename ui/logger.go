package ui

// A program's own log, as opposed to what it says to a person: the lines a
// program started by its socket writes to the journal, or to a terminal when
// someone runs it by hand. charm.land/log does the work; this keeps ui's
// part, which is the session's theme at a terminal and logfmt, which a
// pipe, a file and the journal all read, everywhere else.

import (
	"bytes"
	"image/color"
	"io"
	"log/slog"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"charm.land/log/v2"
	"github.com/charmbracelet/colorprofile"
)

// NewLogger is a logger for the program called name, writing to w, or to the
// session's error stream when w is nil. The result is a *slog.Logger, so a program
// names no third-party type for its log. At a terminal (the error stream, one
// that renders, not --plain or --json) a line is styled in the session's theme;
// anywhere else it is logfmt with a timestamp, one line each, nothing drawn.
//
// Each entry is one line, whatever its message and values hold.
func (s *Session) NewLogger(w io.Writer, name string) *slog.Logger {
	styled := loggerIsStyled(w, s.errStream(), s.plain.Load() || s.noColor.Load(), s.json.Load())
	profile := s.loggerProfile(w)
	if w == nil {
		w = s.errW()
	}
	return slog.New(newLogger(w, name, styled, s.clockFunc(), profile, s.errStream()))
}

func (s *Session) loggerProfile(w io.Writer) colorprofile.Profile {
	if w == nil && s.term.ErrColour {
		return colorprofile.TrueColor
	}
	return colorprofile.NoTTY
}

// A writer a caller gives is a file or a pipe, whatever it is.
func loggerIsStyled(w io.Writer, errStream stream, plain, json bool) bool {
	return w == nil && errStream.paints() && errStream.interactive && !plain && !json
}

func newLogger(w io.Writer, name string, styled bool, now func() time.Time, profile colorprofile.Profile, st stream) *log.Logger {
	opts := log.Options{
		Prefix:          Name(name),
		ReportTimestamp: !styled,
		TimeFormat:      time.RFC3339,
		TimeFunction:    func(time.Time) time.Time { return now() },
	}
	if !styled {
		l := log.NewWithOptions(w, opts)
		l.SetColorProfile(profile)
		l.SetFormatter(log.LogfmtFormatter)
		return l
	}
	l := log.NewWithOptions(oneLine{w: w, keep: logColours(opts, profile, st)}, opts)
	styleLogger(l, profile, st)
	return l
}

func styleLogger(l *log.Logger, profile colorprofile.Profile, st stream) {
	l.SetColorProfile(profile)
	l.SetFormatter(log.TextFormatter)
	l.SetStyles(logStyles(st))
}

// oneLine is the styled face's writer. charm.land/log writes an entry in one
// Write and prints a message as it is given, and a value that holds a newline
// over several lines, so a line from outside could pass for an entry of the
// program's own. Each Write is quoted as Name quotes a name, but for its last
// newline and the colours in keep.
type oneLine struct {
	w    io.Writer
	keep map[string]bool
}

func (o oneLine) Write(p []byte) (int, error) {
	line := escape(strings.TrimSuffix(string(p), "\n"), o.keep) + "\n"
	if _, err := io.WriteString(o.w, line); err != nil {
		return 0, err
	}
	return len(p), nil
}

// logColours is every colour escape the styled face paints with, read from an
// entry of each level that it writes with nothing from outside in it.
func logColours(opts log.Options, profile colorprofile.Profile, st stream) map[string]bool {
	var b bytes.Buffer
	probe := log.NewWithOptions(&b, opts)
	styleLogger(probe, profile, st)
	probe.SetLevel(log.DebugLevel)
	for _, level := range []log.Level{log.DebugLevel, log.InfoLevel, log.WarnLevel, log.ErrorLevel, log.FatalLevel} {
		probe.Log(level, "message", "key", "value", "quoted", "a value")
	}
	keep := map[string]bool{}
	for _, m := range sgr.FindAllString(b.String(), -1) {
		keep[m] = true
	}
	return keep
}

func logStyles(on stream) *log.Styles {
	st := log.DefaultStyles()
	for level, c := range map[log.Level]color.Color{
		log.DebugLevel: orNone(on.colour(nameMuted)),
		log.InfoLevel:  orNone(on.colour(nameAction)),
		log.WarnLevel:  orNone(on.colour(nameAttn)),
		log.ErrorLevel: orNone(on.colour(nameBad)),
		log.FatalLevel: orNone(on.colour(nameBad)),
	} {
		st.Levels[level] = lipgloss.NewStyle().
			SetString(strings.ToUpper(level.String())).
			Bold(true).
			Width(5).
			Foreground(c)
	}
	return st
}

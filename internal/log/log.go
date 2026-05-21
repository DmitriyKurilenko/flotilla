// Package log constructs the structured logger every flotilla command
// uses. We build on stdlib log/slog; no external logging library.
//
// Three output modes:
//
//   - default: human-readable text on stderr (slog.NewTextHandler).
//   - --json:  JSON-per-line on stdout for CI/scripts.
//   - --quiet: only ERROR records, suppressing INFO progress.
package log

import (
	"io"
	"log/slog"
)

// Options carries the global flags that affect logging.
type Options struct {
	Quiet bool
	JSON  bool
}

// New returns a *slog.Logger writing to w configured per opts.
func New(w io.Writer, opts Options) *slog.Logger {
	level := slog.LevelInfo
	if opts.Quiet {
		level = slog.LevelError
	}
	handlerOpts := &slog.HandlerOptions{Level: level}

	var h slog.Handler
	if opts.JSON {
		h = slog.NewJSONHandler(w, handlerOpts)
	} else {
		h = slog.NewTextHandler(w, handlerOpts)
	}
	return slog.New(h)
}

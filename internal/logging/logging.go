// Copyright 2026 Red Hat
// SPDX-License-Identifier: Apache-2.0

package logging

import (
	"io"
	"log/slog"
)

// NewLogger creates a new structured logger with JSON output.
// If verbose is true, the logger uses Debug level; otherwise Info level.
func NewLogger(w io.Writer, verbose bool) *slog.Logger {
	level := slog.LevelInfo
	if verbose {
		level = slog.LevelDebug
	}

	opts := &slog.HandlerOptions{
		Level: level,
	}

	handler := slog.NewJSONHandler(w, opts)
	return slog.New(handler)
}

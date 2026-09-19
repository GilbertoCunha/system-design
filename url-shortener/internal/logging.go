package internal

import (
	"io"
	"log/slog"
)

// Creates a structured logger that writes JSON lines to w,
// discarding records below the given level
func NewLogger(w io.Writer, level slog.Leveler) *slog.Logger {
	return slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{
		Level: level,
	}))
}

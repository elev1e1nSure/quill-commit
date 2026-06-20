package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"strings"
	"time"
)

// EventLogger emits watcher events to the UI channel and optionally to a structured log file.
type EventLogger struct {
	ctx    context.Context
	events chan<- Event
	logger *slog.Logger
}

// newEventLogger creates an EventLogger. A nil logger disables file logging.
func newEventLogger(ctx context.Context, events chan<- Event, logger *slog.Logger) *EventLogger {
	return &EventLogger{ctx: ctx, events: events, logger: logger}
}

// Emit sends an event to the UI channel and writes it to the log file.
func (el *EventLogger) Emit(kind EventKind, msg string) {
	el.EmitDetail(kind, msg, "")
}

// EmitDetail is like Emit but also carries an optional detail string (raw error text or AI fix).
func (el *EventLogger) EmitDetail(kind EventKind, msg, detail string) {
	if el.logger != nil {
		el.log(kind, msg, detail)
	}

	e := Event{Kind: kind, Message: msg, Detail: detail, Time: time.Now()}
	select {
	case el.events <- e:
	default:
		fmt.Fprintf(os.Stderr, "warn: event channel full, dropped %s: %s\n", kindName(kind), msg)
	}
}

// log writes the event to the structured log file at the appropriate level.
func (el *EventLogger) log(kind EventKind, msg, detail string) {
	attrs := []any{slog.String("event", kindName(kind))}
	if detail != "" {
		// Collapse multi-line detail to a single log attribute (first meaningful line + count).
		lines := strings.Split(strings.TrimSpace(detail), "\n")
		if len(lines) == 1 {
			attrs = append(attrs, slog.String("detail", lines[0]))
		} else {
			attrs = append(attrs, slog.String("detail", lines[0]), slog.Int("detail_lines", len(lines)))
		}
	}
	el.logger.Log(el.ctx, kindLevel(kind), msg, attrs...)
}

// kindLevel maps event kinds to slog severity levels.
func kindLevel(kind EventKind) slog.Level {
	switch kind {
	case EventError, EventCommitError:
		return slog.LevelError
	case EventForced:
		return slog.LevelWarn
	case EventCheck, EventSkip, EventDelay, EventDecision:
		return slog.LevelDebug
	default:
		return slog.LevelInfo
	}
}

// kindName returns a short lowercase name for use in structured log attributes.
func kindName(kind EventKind) string {
	name, ok := EventKindNames[kind]
	if !ok {
		return fmt.Sprintf("unknown(%d)", kind)
	}
	// Strip "Event" prefix: "EventCommit" → "commit"
	name = strings.TrimPrefix(name, "Event")
	// CamelCase → snake_case: "CommitError" → "commit_error"
	var b strings.Builder
	for i, r := range name {
		if i > 0 && r >= 'A' && r <= 'Z' {
			b.WriteByte('_')
		}
		b.WriteRune(r | 0x20) // toLower
	}
	return b.String()
}

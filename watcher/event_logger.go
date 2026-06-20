package watcher

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"time"
)

// EventLogger emits watcher events to the UI channel and optionally to a log file.
type EventLogger struct {
	ctx     context.Context
	events  chan<- Event
	logger  *slog.Logger
}

// newEventLogger creates an EventLogger. A nil logger disables file logging.
func newEventLogger(ctx context.Context, events chan<- Event, logger *slog.Logger) *EventLogger {
	return &EventLogger{
		ctx:     ctx,
		events:  events,
		logger:  logger,
	}
}

// Emit sends an event to the UI channel and logs it to the configured log file.
func (el *EventLogger) Emit(kind EventKind, msg string) {
	el.EmitDetail(kind, msg, "")
}

// EmitDetail is like Emit but also carries an optional detail string (e.g. raw error text or AI fix).
func (el *EventLogger) EmitDetail(kind EventKind, msg, detail string) {
	name, ok := EventKindNames[kind]
	if !ok {
		name = fmt.Sprintf("UnknownEvent(%d)", kind)
	}

	if el.logger != nil {
		var level slog.Level
		switch kind {
		case EventError, EventCommitError:
			level = slog.LevelError
		case EventForced:
			level = slog.LevelWarn
		case EventCheck, EventSkip:
			level = slog.LevelDebug
		default:
			level = slog.LevelInfo
		}
		el.logger.Log(el.ctx, level, msg, slog.String("event", name))
	}

	e := Event{Kind: kind, Message: msg, Detail: detail, Time: time.Now()}
	select {
	case el.events <- e:
	default:
		fmt.Fprintf(os.Stderr, "warn: event channel full, dropped %s: %s\n", name, msg)
	}
}

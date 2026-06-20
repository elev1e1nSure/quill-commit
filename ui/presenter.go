package ui

import (
	"fmt"
	"strings"
	"time"

	"quill-commit/config"
	"quill-commit/watcher"
)

// Presenter translates watcher events into view-model state.
type Presenter struct {
	cfg config.Config
}

func newPresenter(cfg config.Config) *Presenter {
	return &Presenter{cfg: cfg}
}

// ApplyEvent updates the model state for the given event and returns the log
// entry that should be appended (if any).
func (p *Presenter) ApplyEvent(m *Model, e watcher.Event) (logEntry string) {
	ts := formatTimestamp(e.Time)

	switch e.Kind {
	case watcher.EventCheck:
		m.nextCheck = e.Time.Add(time.Duration(p.cfg.Interval * float64(time.Minute)))
		m.sending = false
		m.stabilizing = false

	case watcher.EventSending:
		m.sending = true
		m.stabilizing = false
		m.statusLockedUntil = e.Time.Add(2 * time.Second)

	case watcher.EventDecision:
		m.sending = false
		if !strings.Contains(e.Message, "commit:") {
			var delaySec, delayCount, maxDelays int
			n, err := fmt.Sscanf(e.Message, "model says wait %ds (delay %d/%d)", &delaySec, &delayCount, &maxDelays)
			if n >= 2 && err == nil {
				m.delayCounter = delayCount
				m.nextCheck = e.Time.Add(time.Duration(delaySec) * time.Second)
			} else {
				m.delayCounter++
				var d int
				if n, err := fmt.Sscanf(e.Message, "model says wait %ds", &d); n == 1 && err == nil {
					m.nextCheck = e.Time.Add(time.Duration(d) * time.Second)
				}
			}
		}
		// No log entry — decision outcome is visible in the status bar and timer.

	case watcher.EventSkip:
		m.delayCounter = 0
		switch {
		case strings.Contains(e.Message, "diff changed"):
			m.stabilizing = true
			m.nextCheck = e.Time.Add(time.Duration(p.cfg.Stabilize * float64(time.Minute)))
			m.statusLockedUntil = e.Time.Add(2 * time.Second)
		default:
			m.stabilizing = false
		}
		// No log entry — skip states are shown in the status bar ("watching...", "waiting...").

	case watcher.EventDelay:
		// No log entry — retry countdown is shown in the status bar timer.

	case watcher.EventForced:
		m.delayCounter = 0
		m.nextCheck = e.Time.Add(time.Duration(p.cfg.Interval * float64(time.Minute)))
		return ts + "  " + stWarn.Render("forced") + "  " + stDim.Render("max delays reached, committing")

	case watcher.EventError:
		m.sending = false
		m.stabilizing = false
		m.amending = false
		m.delayCounter = 0
		return ts + "  " + stWarn.Render("error") + "  " + stDim.Render(cleanError(e.Message))

	case watcher.EventInfo:
		// Only quarantine warnings are worth a log entry; other info is ambient/transient.
		if strings.HasPrefix(e.Message, "quarantine:") {
			body := strings.TrimPrefix(e.Message, "quarantine: ")
			return ts + "  " + stWarn.Render("quarantine") + "  " + stDim.Render(body)
		}

	case watcher.EventCommitError:
		m.sending = false
		m.stabilizing = false
		m.errorRaw = e.Detail
		m.errorFix = ""
		m.showDetail = false
		m.statusLockedUntil = e.Time.Add(2 * time.Second)
		summary := firstMeaningfulLine(e.Detail)
		return ts + "  " + stWarn.Render("blocked") + "  " + stDim.Render(summary) + "  " + stDim.Render("ctrl+o")

	case watcher.EventErrorExplain:
		m.sending = false
		m.errorFix = e.Detail
		return ts + "  " + stDim.Render("→") + "  " + stText.Render(e.Message)
	}

	return ""
}

// cleanError strips repetitive "op: git args: exit status N:" prefix noise
// from git error strings, leaving a single readable line.
func cleanError(msg string) string {
	// Strip outer operation prefix added by formatGitError ("op: ...").
	if idx := strings.Index(msg, ": "); idx > 0 {
		msg = msg[idx+2:]
	}
	// Strip inner "git args: exit status N: " prefix from runGit.
	if idx := strings.Index(msg, ": exit status "); idx >= 0 {
		rest := msg[idx+2:]
		if i2 := strings.Index(rest, ": "); i2 >= 0 {
			msg = rest[i2+2:]
		}
	}
	for _, line := range strings.Split(msg, "\n") {
		if line = strings.TrimSpace(line); line != "" {
			return truncate(line, 80)
		}
	}
	return truncate(strings.TrimSpace(msg), 80)
}

// firstMeaningfulLine finds the first non-warning, non-hint line in a raw
// hook/git error block — what actually went wrong, not git's preamble.
func firstMeaningfulLine(raw string) string {
	for _, line := range strings.Split(raw, "\n") {
		line = strings.TrimSpace(line)
		if line == "" {
			continue
		}
		low := strings.ToLower(line)
		if strings.HasPrefix(low, "warning:") || strings.HasPrefix(low, "hint:") {
			continue
		}
		if idx := strings.Index(line, ": exit status "); idx >= 0 {
			rest := line[idx+2:]
			if i2 := strings.Index(rest, ": "); i2 >= 0 {
				line = rest[i2+2:]
			}
		}
		return truncate(line, 80)
	}
	// Fallback: first line as-is.
	first := strings.SplitN(strings.TrimSpace(raw), "\n", 2)[0]
	return truncate(first, 80)
}

func truncate(s string, max int) string {
	if len(s) <= max {
		return s
	}
	return s[:max-1] + "…"
}

func formatTimestamp(t time.Time) string {
	return stDim.Render(t.Format("15:04"))
}

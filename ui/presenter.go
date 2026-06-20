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

// newPresenter creates a Presenter.
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

	case watcher.EventDecision:
		m.sending = false
		if !strings.Contains(e.Message, "commit:") {
			delayMin, delayCount, maxDelays := 0, 0, 0
			n, err := fmt.Sscanf(e.Message, "model says wait %dm (delay %d/%d)", &delayMin, &delayCount, &maxDelays)
			if n >= 2 && err == nil {
				m.delayCounter = delayCount
				m.nextCheck = e.Time.Add(time.Duration(delayMin) * time.Minute)
			} else {
				m.delayCounter++
				if n, err := fmt.Sscanf(e.Message, "model says wait %dm", &delayMin); n == 1 && err == nil {
					m.nextCheck = e.Time.Add(time.Duration(delayMin) * time.Minute)
				}
			}
		}

	case watcher.EventForced:
		m.delayCounter = 0
		m.nextCheck = e.Time.Add(time.Duration(p.cfg.Interval * float64(time.Minute)))
		return ts + "  " + stWarn.Render(e.Message)

	case watcher.EventError:
		m.sending = false
		m.stabilizing = false
		m.amending = false
		m.delayCounter = 0
		return ts + "  " + stText.Render(e.Message)

	case watcher.EventSkip:
		m.delayCounter = 0
		switch {
		case strings.Contains(e.Message, "diff changed"):
			m.stabilizing = true
			m.nextCheck = e.Time.Add(time.Duration(p.cfg.Stabilize * float64(time.Minute)))
		case strings.Contains(e.Message, "diff empty"):
			m.stabilizing = false
		default:
			m.stabilizing = false
			return ts + "  " + stText.Render(e.Message)
		}

	case watcher.EventDelay:
		return ts + "  " + stDim.Render(e.Message)

	case watcher.EventInfo:
		return ts + "  " + stDim.Render(e.Message)

	case watcher.EventCommitError:
		m.sending = false
		m.stabilizing = false
		m.errorRaw = e.Detail
		m.errorFix = ""
		m.showDetail = false
		return ts + "  " + stWarn.Render("commit blocked") + "  " + stDim.Render("ctrl+o for details")

	case watcher.EventErrorExplain:
		m.sending = false
		m.errorFix = e.Detail
		return ts + "  " + stDim.Render("explain: ") + stText.Render(e.Message)
	}

	return ""
}

func formatTimestamp(t time.Time) string {
	return stDim.Render(t.Format("15:04"))
}

package ui

import (
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"

	"quill-commit/git"
	"quill-commit/watcher"
)

// UpdateHandlers processes Bubble Tea messages and updates the model.
type UpdateHandlers struct {
	m *Model
}

func newUpdateHandlers(m *Model) *UpdateHandlers {
	return &UpdateHandlers{m: m}
}

// HandleKey processes keyboard input.
func (h *UpdateHandlers) HandleKey(msg tea.KeyMsg) tea.Cmd {
	m := h.m
	switch msg.String() {
	case "q", "ctrl+c":
		if m.quitPending {
			return tea.Quit
		}
		m.quitPending = true
		return tea.Tick(3*time.Second, func(_ time.Time) tea.Msg { return quitResetMsg{} })
	case "p":
		if m.sending || m.amending {
			return nil
		}
		if m.paused {
			m.paused = false
			m.sendCmd(watcher.Cmd{Kind: watcher.CmdResume})
		} else {
			m.paused = true
			m.sendCmd(watcher.Cmd{Kind: watcher.CmdPause})
		}
	case "a":
		if m.sending || m.amending {
			return nil
		}
		if !m.amending {
			m.amending = true
			m.sendCmd(watcher.Cmd{Kind: watcher.CmdAmend})
		}
	case "ctrl+o":
		if m.errorRaw != "" {
			m.showDetail = !m.showDetail
		}
	}
	return nil
}

// HandleWindowSize initializes or resizes the viewport.
func (h *UpdateHandlers) HandleWindowSize(msg tea.WindowSizeMsg) {
	m := h.m
	m.width = msg.Width
	m.height = msg.Height
	vpH := m.height - statusBlockHeight - footerHeight - 3 // log block: top border + title + bottom border = 3 overhead
	if vpH < 3 {
		vpH = 3
	}
	vpW := m.width - boxOverhead
	if vpW < 10 {
		vpW = 10
	}
	if !m.ready {
		m.vp = viewport.New(vpW, vpH)
		m.ready = true
	} else {
		m.vp.Width = vpW
		m.vp.Height = vpH
	}
	m.syncViewport()
}

// HandleTick toggles the pulse state and schedules the next tick.
func (h *UpdateHandlers) HandleTick() tea.Cmd {
	h.m.pulseTick = !h.m.pulseTick
	return secondTick()
}

// HandleSpinner advances the spinner and schedules the next frame.
func (h *UpdateHandlers) HandleSpinner() tea.Cmd {
	h.m.spinnerFrame = (h.m.spinnerFrame + 1) % len(spinnerFrames)
	return spinnerTick()
}

// HandleEvent processes a watcher event and re-enqueues the listener.
func (h *UpdateHandlers) HandleEvent(msg eventMsg) tea.Cmd {
	m := h.m
	e := watcher.Event(msg)

	// Commit and amend clear all active states immediately — never defer.
	if e.Kind == watcher.EventCommit || e.Kind == watcher.EventAmend {
		m.sending = false
		m.stabilizing = false
		m.amending = false
		m.errorRaw = ""
		m.errorFix = ""
		m.showDetail = false
		m.statusLockedUntil = time.Time{}
		if e.Kind == watcher.EventCommit {
			m.delayCounter = 0
			m.nextCheck = e.Time.Add(time.Duration(m.cfg.Interval * float64(time.Minute)))
		}
		return tea.Batch(
			func() tea.Msg {
				return headHashMsg{
					hash:    git.HeadHash(),
					message: e.Message,
					isAmend: e.Kind == watcher.EventAmend,
					time:    e.Time,
				}
			},
			listenEvent(m.events),
		)
	}

	// Events that clear an active status are deferred if the current status
	// hasn't been visible for the minimum 2 seconds yet.
	if delay := time.Until(m.statusLockedUntil); delay > 0 {
		switch e.Kind {
		case watcher.EventDecision, watcher.EventCheck, watcher.EventError,
			watcher.EventErrorExplain, watcher.EventInfo, watcher.EventDelay,
			watcher.EventForced:
			return tea.Batch(
				listenEvent(m.events),
				tea.Tick(delay, func(_ time.Time) tea.Msg { return deferredEventMsg(e) }),
			)
		}
	}

	if logEntry := m.presenter.ApplyEvent(m, e); logEntry != "" {
		m.log = append(m.log, logEntry)
	}
	m.syncViewport()
	return listenEvent(m.events)
}

// HandleDeferredEvent applies an event that was held back for the minimum
// status display duration. Does not re-enqueue the listener (already running).
func (h *UpdateHandlers) HandleDeferredEvent(msg deferredEventMsg) tea.Cmd {
	m := h.m
	e := watcher.Event(msg)
	if logEntry := m.presenter.ApplyEvent(m, e); logEntry != "" {
		m.log = append(m.log, logEntry)
	}
	m.syncViewport()
	return nil
}

// HandleHeadHash appends a commit/amend entry to the log.
func (h *UpdateHandlers) HandleHeadHash(msg headHashMsg) {
	m := h.m
	ts := stDim.Render(msg.time.Format("15:04"))
	if msg.hash != "" {
		m.lastCommit = stAccent2.Render(msg.hash) + " " + stText.Render(msg.message)
		if msg.isAmend {
			m.log = append(m.log, ts+"  "+stDim.Render("amended")+"  "+stAccent2.Render(msg.hash)+" "+stText.Render(msg.message))
		} else {
			m.log = append(m.log, ts+"  "+stAccent2.Render(msg.hash)+" "+stText.Render(msg.message))
		}
	} else {
		m.lastCommit = stText.Render(msg.message)
		if msg.isAmend {
			m.log = append(m.log, ts+"  "+stDim.Render("amended")+"  "+stText.Render(msg.message))
		} else {
			m.log = append(m.log, ts+"  "+stText.Render(msg.message))
		}
	}
	m.syncViewport()
}

package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"quill-commit/config"
	"quill-commit/git"
	"quill-commit/watcher"
)

var (
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	stText    = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4D4D4"))
	stTitle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C9BD2")).Bold(true)
	stAccent2 = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4842A"))
	stSuccess = lipgloss.NewStyle().Foreground(lipgloss.Color("#5FA862"))
	stWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A82A"))
	stErr     = lipgloss.NewStyle().Foreground(lipgloss.Color("#D44A4A"))

	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#6C9BD2")).
			Padding(0, 1)
)

// statusBlockHeight is the fixed line count of the rendered status block
// (top border + title + 4 rows + bottom border = 7).
const statusBlockHeight = 7

// boxOverhead is the total horizontal chars added by boxStyle (2 borders + 2 padding each side = 4).
const boxOverhead = 4

const (
	statusNone        int = iota
	statusChecking
	statusIdle
	statusStabilizing
	statusDelaying
)

type tickMsg time.Time
type eventMsg watcher.Event

type Model struct {
	cfg          config.Config
	events       <-chan watcher.Event
	nextCheck    time.Time
	delayCounter int
	lastCommit   string
	log          []string
	vp           viewport.Model
	width        int
	height       int
	ready        bool
	statusKind   int
	statusLine   string
}

func New(cfg config.Config, events <-chan watcher.Event) Model {
	return Model{
		cfg:        cfg,
		events:     events,
		nextCheck:  time.Now().Add(time.Duration(cfg.Interval * float64(time.Minute))),
		statusKind: statusNone,
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(secondTick(), listenEvent(m.events))
}

func secondTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func listenEvent(ch <-chan watcher.Event) tea.Cmd {
	return func() tea.Msg {
		e, ok := <-ch
		if !ok {
			return nil
		}
		return eventMsg(e)
	}
}

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			return m, tea.Quit
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpH := m.height - statusBlockHeight - 3 - 1 // log block: top border + title + bottom border + status line = 4 overhead
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

	case tickMsg:
		cmds = append(cmds, secondTick())
		switch m.statusKind {
		case statusIdle:
			remaining := time.Until(m.nextCheck)
			if remaining <= 0 {
				m.statusLine = "Idle... Checking diff..."
			} else {
				m.statusLine = fmt.Sprintf("Idle... Checking diff in %ds", int(remaining.Seconds()))
			}
		case statusStabilizing:
			remaining := time.Until(m.nextCheck)
			if remaining <= 0 {
				m.statusLine = "Diff changed, re-checking..."
			} else {
				m.statusLine = fmt.Sprintf("Diff changed, re-checking in %ds", int(remaining.Seconds()))
			}
		}

	case eventMsg:
		m.applyEvent(watcher.Event(msg))
		m.syncViewport()
		cmds = append(cmds, listenEvent(m.events))
	}

	if m.ready {
		var vpCmd tea.Cmd
		m.vp, vpCmd = m.vp.Update(msg)
		if vpCmd != nil {
			cmds = append(cmds, vpCmd)
		}
	}

	return m, tea.Batch(cmds...)
}

func (m *Model) applyEvent(e watcher.Event) {
	ts := stDim.Render(e.Time.Format("15:04:05"))

	switch e.Kind {
	case watcher.EventCheck:
		m.nextCheck = e.Time.Add(time.Duration(m.cfg.Interval * float64(time.Minute)))
		m.statusKind = statusChecking
		m.statusLine = "Checking diff..."

	case watcher.EventDecision:
		if strings.Contains(e.Message, "commit:") {
			m.log = append(m.log, ts+"  "+stSuccess.Render(e.Message))
		} else {
			m.delayCounter++
			m.log = append(m.log, ts+"  "+stWarn.Render(e.Message))
			m.statusKind = statusDelaying
			m.statusLine = e.Message
		}

	case watcher.EventCommit:
		hash := git.HeadHash()
		if hash != "" {
			m.lastCommit = stAccent2.Render(hash) + " " + stText.Render(e.Message)
		} else {
			m.lastCommit = stText.Render(e.Message)
		}
		m.delayCounter = 0
		m.log = append(m.log, ts+"  "+stSuccess.Render("committed: "+e.Message))
		m.statusKind = statusNone
		m.statusLine = ""

	case watcher.EventForced:
		m.delayCounter = 0
		m.log = append(m.log, ts+"  "+stWarn.Render(e.Message))
		m.statusKind = statusNone
		m.statusLine = ""

	case watcher.EventError:
		m.log = append(m.log, ts+"  "+stErr.Render(e.Message))
		m.statusKind = statusNone
		m.statusLine = ""

	case watcher.EventSkip:
		if strings.Contains(e.Message, "diff empty") {
			m.statusKind = statusIdle
			remaining := time.Until(m.nextCheck)
			if remaining <= 0 {
				m.statusLine = "Idle... Checking diff..."
			} else {
				m.statusLine = fmt.Sprintf("Idle... Checking diff in %ds", int(remaining.Seconds()))
			}
		} else if strings.Contains(e.Message, "diff changed") {
			m.nextCheck = e.Time.Add(time.Duration(m.cfg.Stabilize * float64(time.Minute)))
			m.statusKind = statusStabilizing
			remaining := time.Until(m.nextCheck)
			if remaining <= 0 {
				m.statusLine = "Diff changed, re-checking..."
			} else {
				m.statusLine = fmt.Sprintf("Diff changed, re-checking in %ds", int(remaining.Seconds()))
			}
		} else {
			m.log = append(m.log, ts+"  "+stDim.Render(e.Message))
		}

	case watcher.EventDelay:
		m.log = append(m.log, ts+"  "+stDim.Render(e.Message))
	}
}

func (m *Model) syncViewport() {
	if !m.ready {
		return
	}
	m.vp.SetContent(strings.Join(m.log, "\n"))
	m.vp.GotoBottom()
}

func (m Model) View() string {
	if !m.ready {
		return "\n  " + stDim.Render("initializing...")
	}
	return lipgloss.JoinVertical(lipgloss.Left,
		m.renderStatus(),
		m.renderLogBlock(),
	)
}

func formatInterval(minutes float64) string {
	d := time.Duration(minutes * float64(time.Minute))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm %ds", m, s)
}

func (m Model) renderStatus() string {
	remaining := time.Until(m.nextCheck)
	var nextStr string
	switch {
	case remaining <= 0:
		nextStr = stAccent2.Render("checking...")
	case int(remaining.Minutes()) > 0:
		nextStr = stText.Render(fmt.Sprintf("in %dm %ds", int(remaining.Minutes()), int(remaining.Seconds())%60))
	default:
		nextStr = stText.Render(fmt.Sprintf("in %ds", int(remaining.Seconds())))
	}

	lastCommit := m.lastCommit
	if lastCommit == "" {
		lastCommit = stDim.Render("none")
	}

	lbl := func(s string) string { return stDim.Render(fmt.Sprintf("%-12s", s)) }
	rows := strings.Join([]string{
		stTitle.Render("status"),
		lbl("interval") + "  " + stText.Render(formatInterval(m.cfg.Interval)) + "  " + stDim.Render("stabilize "+formatInterval(m.cfg.Stabilize)),
		lbl("next check") + "  " + nextStr,
		lbl("delays") + "  " + stText.Render(fmt.Sprintf("%d / %d", m.delayCounter, m.cfg.MaxDelays)),
		lbl("last commit") + "  " + lastCommit,
	}, "\n")

	return boxStyle.Width(m.width - boxOverhead).Render(rows)
}

func (m Model) renderLogBlock() string {
	content := stTitle.Render("log") + "\n" + m.vp.View()
	if m.statusLine != "" {
		content += "\n" + stDim.Render(m.statusLine)
	}
	return boxStyle.Width(m.width - boxOverhead).Render(content)
}

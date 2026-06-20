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
	stWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A82A"))
	boxStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#6C9BD2")).
			Padding(0, 1)
)

// statusBlockHeight is the fixed line count of the rendered status block
// (top border + title + 4 content rows + hints row + bottom border = 8).
const statusBlockHeight = 8

// boxOverhead is the total horizontal chars added by boxStyle (2 borders + 2 padding each side = 4).
const boxOverhead = 4

type tickMsg time.Time
type spinnerMsg time.Time
type eventMsg watcher.Event

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

type quitResetMsg struct{}

type Model struct {
	cfg          config.Config
	events       <-chan watcher.Event
	cmds         chan<- watcher.Cmd
	nextCheck    time.Time
	delayCounter int
	lastCommit   string
	log          []string
	vp           viewport.Model
	width        int
	height       int
	ready        bool
	spinnerFrame int
	sending      bool
	stabilizing  bool
	pulseTick    bool
	paused       bool
	amending     bool
	quitPending  bool
}

func New(cfg config.Config, events <-chan watcher.Event, cmds chan<- watcher.Cmd) Model {
	return Model{
		cfg:       cfg,
		events:    events,
		cmds:      cmds,
		nextCheck: time.Now().Add(time.Duration(cfg.Interval * float64(time.Minute))),
	}
}

func (m Model) Init() tea.Cmd {
	return tea.Batch(secondTick(), spinnerTick(), listenEvent(m.events))
}

func secondTick() tea.Cmd {
	return tea.Tick(time.Second, func(t time.Time) tea.Msg { return tickMsg(t) })
}

func spinnerTick() tea.Cmd {
	return tea.Tick(100*time.Millisecond, func(t time.Time) tea.Msg { return spinnerMsg(t) })
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
	case quitResetMsg:
		m.quitPending = false

	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c":
			if m.quitPending {
				return m, tea.Quit
			}
			m.quitPending = true
			return m, tea.Tick(3*time.Second, func(t time.Time) tea.Msg { return quitResetMsg{} })
		case "p":
			if m.paused {
				m.paused = false
				m.sendCmd(watcher.Cmd{Kind: watcher.CmdResume})
			} else {
				m.paused = true
				m.sendCmd(watcher.Cmd{Kind: watcher.CmdPause})
			}
		case "a":
			if !m.amending {
				m.amending = true
				m.sendCmd(watcher.Cmd{Kind: watcher.CmdAmend})
			}
		}

	case tea.WindowSizeMsg:
		m.width = msg.Width
		m.height = msg.Height
		vpH := m.height - statusBlockHeight - 3 // log block: top border + title + bottom border = 3 overhead
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
		m.pulseTick = !m.pulseTick
		cmds = append(cmds, secondTick())

	case spinnerMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		cmds = append(cmds, spinnerTick())

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

func (m *Model) sendCmd(cmd watcher.Cmd) {
	select {
	case m.cmds <- cmd:
	default:
	}
}

func (m *Model) applyEvent(e watcher.Event) {
	ts := stDim.Render(e.Time.Format("15:04"))

	switch e.Kind {
	case watcher.EventCheck:
		m.nextCheck = e.Time.Add(time.Duration(m.cfg.Interval * float64(time.Minute)))
		m.sending = false
		m.stabilizing = false

	case watcher.EventSending:
		m.sending = true
		m.stabilizing = false

	case watcher.EventDecision:
		m.sending = false
		if strings.Contains(e.Message, "commit:") {
			// EventCommit handles the log entry
		} else {
			var delayMin, delayCount, maxDelays int
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

	case watcher.EventAmend:
		m.amending = false
		m.sending = false
		hash := git.HeadHash()
		if hash != "" {
			m.lastCommit = stAccent2.Render(hash) + " " + stText.Render(e.Message)
			m.log = append(m.log, ts+"  "+stDim.Render("amended")+"  "+stAccent2.Render(hash)+" "+stText.Render(e.Message))
		} else {
			m.lastCommit = stText.Render(e.Message)
			m.log = append(m.log, ts+"  "+stDim.Render("amended")+"  "+stText.Render(e.Message))
		}

	case watcher.EventCommit:
		m.sending = false
		m.stabilizing = false
		hash := git.HeadHash()
		if hash != "" {
			m.lastCommit = stAccent2.Render(hash) + " " + stText.Render(e.Message)
			m.log = append(m.log, ts+"  "+stAccent2.Render(hash)+" "+stText.Render(e.Message))
		} else {
			m.lastCommit = stText.Render(e.Message)
			m.log = append(m.log, ts+"  "+stText.Render(e.Message))
		}
		m.delayCounter = 0
		m.nextCheck = e.Time.Add(time.Duration(m.cfg.Interval * float64(time.Minute)))

	case watcher.EventForced:
		m.delayCounter = 0
		m.log = append(m.log, ts+"  "+stWarn.Render(e.Message))
		m.nextCheck = e.Time.Add(time.Duration(m.cfg.Interval * float64(time.Minute)))

	case watcher.EventError:
		m.sending = false
		m.stabilizing = false
		m.amending = false
		m.delayCounter = 0
		m.log = append(m.log, ts+"  "+stText.Render(e.Message))

	case watcher.EventSkip:
		m.delayCounter = 0
		if strings.Contains(e.Message, "diff changed") {
			m.stabilizing = true
			m.nextCheck = e.Time.Add(time.Duration(m.cfg.Stabilize * float64(time.Minute)))
		} else if strings.Contains(e.Message, "diff empty") {
			m.stabilizing = false
		} else {
			m.stabilizing = false
			m.log = append(m.log, ts+"  "+stText.Render(e.Message))
		}

	case watcher.EventDelay:
		m.log = append(m.log, ts+"  "+stDim.Render(e.Message))

	case watcher.EventInfo:
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

func (m Model) renderStatus() string {
	remaining := time.Until(m.nextCheck)
	var nextStr string
	spinner := stAccent2.Render(spinnerFrames[m.spinnerFrame])
	switch {
	case m.paused:
		nextStr = stWarn.Render("PAUSED")
	case m.amending:
		nextStr = spinner + " " + stAccent2.Render("amending...")
	case m.sending:
		nextStr = spinner + " " + stAccent2.Render("asking model...")
	case remaining <= 0:
		nextStr = spinner + " " + stAccent2.Render("checking...")
	case m.stabilizing:
		nextStr = spinner + " " + stAccent2.Render("stabilizing...")
	case int(remaining.Minutes()) > 0:
		nextStr = stText.Render(fmt.Sprintf("next check in %dm %ds", int(remaining.Minutes()), int(remaining.Seconds())%60))
	default:
		nextStr = stText.Render(fmt.Sprintf("next check in %ds", int(remaining.Seconds())))
	}

	lastCommit := m.lastCommit
	if lastCommit == "" {
		lastCommit = stDim.Render("none")
	} else if m.stabilizing {
		dot := "●"
		if m.pulseTick {
			dot = "○"
		}
		lastCommit = stDim.Render(dot) + " " + lastCommit
	}

	pauseKey := "p: pause"
	if m.paused {
		pauseKey = "p: resume"
	}
	var hintsStr string
	if m.quitPending {
		hintsStr = stWarn.Render("press q / ctrl+c again to quit")
	} else {
		hintsStr = stDim.Render(fmt.Sprintf("%s  a: amend  q: quit", pauseKey))
	}

	lbl := func(s string) string { return stDim.Render(fmt.Sprintf("%-12s", s)) }
	rows := strings.Join([]string{
		stTitle.Render("info"),
		lbl("status") + "  " + nextStr,
		lbl("delays") + "  " + stText.Render(fmt.Sprintf("%d / %d", m.delayCounter, m.cfg.MaxDelays)),
		lbl("last commit") + "  " + lastCommit,
		"",
		hintsStr,
	}, "\n")

	return boxStyle.Width(m.width - boxOverhead).Render(rows)
}

func (m Model) renderLogBlock() string {
	content := stTitle.Render("log") + "\n" + m.vp.View()
	return boxStyle.Width(m.width - boxOverhead).Render(content)
}

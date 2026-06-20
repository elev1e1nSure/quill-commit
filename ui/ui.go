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
// (top border + title + 3 content rows + bottom border = 6).
const statusBlockHeight = 6

// footerHeight is the single hints line rendered below the log block.
const footerHeight = 1

// boxOverhead is the total horizontal chars added by boxStyle (2 borders + 2 padding each side = 4).
const boxOverhead = 4

type tickMsg time.Time
type spinnerMsg time.Time
type eventMsg watcher.Event
type headHashMsg struct {
	hash    string
	message string
	isAmend bool
	time    time.Time
}

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

	case tickMsg:
		m.pulseTick = !m.pulseTick
		cmds = append(cmds, secondTick())

	case spinnerMsg:
		m.spinnerFrame = (m.spinnerFrame + 1) % len(spinnerFrames)
		cmds = append(cmds, spinnerTick())

	case eventMsg:
		e := watcher.Event(msg)
		if e.Kind == watcher.EventCommit || e.Kind == watcher.EventAmend {
			m.sending = false
			m.stabilizing = false
			m.amending = false
			if e.Kind == watcher.EventCommit {
				m.delayCounter = 0
				m.nextCheck = e.Time.Add(time.Duration(m.cfg.Interval * float64(time.Minute)))
			}
			cmds = append(cmds, func() tea.Msg {
				return headHashMsg{
					hash:    git.HeadHash(),
					message: e.Message,
					isAmend: e.Kind == watcher.EventAmend,
					time:    e.Time,
				}
			})
		} else {
			m.applyEvent(e)
			m.syncViewport()
		}
		cmds = append(cmds, listenEvent(m.events))

	case headHashMsg:
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
		m.renderHints(),
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
	default:
		durStr := fmt.Sprintf("%ds", int(remaining.Seconds()))
		if int(remaining.Minutes()) > 0 {
			durStr = fmt.Sprintf("%dm %ds", int(remaining.Minutes()), int(remaining.Seconds())%60)
		}
		nextStr = spinner + " " + stAccent2.Render("waiting ("+durStr+")")
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

	lbl := func(s string) string { return stDim.Render(fmt.Sprintf("%-12s", s)) }
	rows := strings.Join([]string{
		stTitle.Render("info"),
		lbl("status") + "  " + nextStr,
		lbl("delays") + "  " + stText.Render(fmt.Sprintf("%d / %d", m.delayCounter, m.cfg.MaxDelays)),
		lbl("last commit") + "  " + lastCommit,
	}, "\n")

	return boxStyle.Width(m.width - boxOverhead).Render(rows)
}

func (m Model) renderLogBlock() string {
	content := stTitle.Render("log") + "\n" + m.vp.View()
	return boxStyle.Width(m.width - boxOverhead).Render(content)
}

// renderHints draws the keybinding footer: keys in white, descriptions dim.
func (m Model) renderHints() string {
	if m.quitPending {
		return "  " + stWarn.Render("press q / ctrl+c again to quit")
	}
	pauseDesc := "pause"
	if m.paused {
		pauseDesc = "resume"
	}
	hint := func(key, desc string) string {
		return stText.Render(key) + stDim.Render(": "+desc)
	}
	sep := stDim.Render("   ")
	return "  " + hint("p", pauseDesc) + sep + hint("a", "amend") + sep + hint("q", "quit")
}

package ui

import (
	"strings"
	"time"

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"quill-commit/config"
	"quill-commit/watcher"
)

var (
	stDim     = lipgloss.NewStyle().Foreground(lipgloss.Color("#808080"))
	stText    = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4D4D4"))
	stTitle   = lipgloss.NewStyle().Foreground(lipgloss.Color("#6C9BD2")).Bold(true)
	stAccent2 = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4842A"))
	stWarn    = lipgloss.NewStyle().Foreground(lipgloss.Color("#D4A82A"))
	boxStyle  = lipgloss.NewStyle().
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
type deferredEventMsg watcher.Event

// Model is the Bubble Tea model. It holds view state and delegates message
// handling, presentation logic, and rendering to focused helpers.
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

	// errorRaw holds the raw git/hook error text from the last EventCommitError.
	// errorFix holds the AI-suggested fix from the subsequent EventErrorExplain.
	errorRaw         string
	errorFix         string
	showDetail       bool
	statusLockedUntil time.Time

	handlers  *UpdateHandlers
	presenter *Presenter
	renderer  *Renderer
}

// New creates a new UI model for the given config and watcher channels.
func New(cfg config.Config, events <-chan watcher.Event, cmds chan<- watcher.Cmd) Model {
	m := Model{
		cfg:       cfg,
		events:    events,
		cmds:      cmds,
		nextCheck: time.Now().Add(time.Duration(cfg.Interval * float64(time.Minute))),
	}
	m.presenter = newPresenter(cfg)
	m.renderer = newRenderer()
	m.handlers = newUpdateHandlers(&m)
	return m
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
	m.handlers.m = &m // redirect handlers to the current local copy (value receiver)
	var cmds []tea.Cmd

	switch msg := msg.(type) {
	case quitResetMsg:
		m.quitPending = false

	case tea.KeyMsg:
		if cmd := m.handlers.HandleKey(msg); cmd != nil {
			return m, cmd
		}

	case tea.WindowSizeMsg:
		m.handlers.HandleWindowSize(msg)

	case tickMsg:
		cmds = append(cmds, m.handlers.HandleTick())

	case spinnerMsg:
		cmds = append(cmds, m.handlers.HandleSpinner())

	case eventMsg:
		cmds = append(cmds, m.handlers.HandleEvent(msg))

	case deferredEventMsg:
		cmds = append(cmds, m.handlers.HandleDeferredEvent(msg))

	case headHashMsg:
		m.handlers.HandleHeadHash(msg)
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
	middle := m.renderer.LogBlock(&m)
	if m.showDetail && m.errorRaw != "" {
		middle = m.renderer.DetailOverlay(&m)
	}
	return m.renderer.JoinView(
		m.renderer.Status(&m),
		middle,
		m.renderer.Hints(&m),
	)
}

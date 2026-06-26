package ui

import (
	"fmt"
	"strings"
	"time"

	"github.com/charmbracelet/lipgloss"
)

var stDetail = lipgloss.NewStyle().Foreground(lipgloss.Color("#A0A0A0"))

// Renderer draws the UI components.
type Renderer struct{}

func newRenderer() *Renderer {
	return &Renderer{}
}

// Status renders the top info box.
func (r *Renderer) Status(m *Model) string {
	remaining := time.Until(m.nextCheck)
	var nextStr string
	spinnerDim := stDim.Render(spinnerFrames[m.spinnerFrame])
	spinnerAccent := stAccent2.Render(spinnerFrames[m.spinnerFrame])
	switch {
	case m.paused:
		nextStr = stWarn.Render("PAUSED")
	case m.amending:
		nextStr = spinnerAccent + " " + stAccent2.Render("rewriting...")
	case m.sending:
		nextStr = spinnerAccent + " " + stAccent2.Render("pondering...")
	case remaining <= 0:
		nextStr = spinnerDim + " " + stText.Render("checking...")
	case m.stabilizing:
		durStr := fmt.Sprintf("%ds", int(remaining.Seconds()))
		if int(remaining.Minutes()) > 0 {
			durStr = fmt.Sprintf("%dm %ds", int(remaining.Minutes()), int(remaining.Seconds())%60)
		}
		nextStr = spinnerDim + " " + stText.Render("watching ("+durStr+")")
	default:
		durStr := fmt.Sprintf("%ds", int(remaining.Seconds()))
		if int(remaining.Minutes()) > 0 {
			durStr = fmt.Sprintf("%dm %ds", int(remaining.Minutes()), int(remaining.Seconds())%60)
		}
		nextStr = spinnerDim + " " + stDim.Render("waiting ("+durStr+")")
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

	// Clamp lastCommit to one line so the status block height is predictable.
	// label(12) + sep(2) + box padding/border(4) = 18 chars overhead.
	if maxW := m.width - 18; maxW > 4 && lipgloss.Width(lastCommit) > maxW {
		lastCommit = lipgloss.NewStyle().MaxWidth(maxW - 1).Render(lastCommit) + stDim.Render("…")
	}

	lbl := func(s string) string { return stDim.Render(fmt.Sprintf("%-12s", s)) }
	delaysRow := stText.Render(fmt.Sprintf("%d / %d", m.delayCounter, m.cfg.MaxDelays))
	if !m.cfg.Delayed() {
		delaysRow = stDim.Render("off")
	}
	rows := strings.Join([]string{
		stTitle.Render("info"),
		lbl("status") + "  " + nextStr,
		lbl("delays") + "  " + delaysRow,
		lbl("last commit") + "  " + lastCommit,
	}, "\n")

	return boxStyle.Width(m.width - boxOverhead).Render(rows)
}

// LogBlock renders the scrollable log box.
func (r *Renderer) LogBlock(m *Model) string {
	inner := m.width - boxOverhead

	start := m.logOffset
	if start < 0 {
		start = 0
	}
	if start > len(m.log) {
		start = len(m.log)
	}
	end := start + m.vpHeight
	if end > len(m.log) {
		end = len(m.log)
	}
	visibleLines := m.log[start:end]

	renderedLines := make([]string, m.vpHeight)
	for i := 0; i < m.vpHeight; i++ {
		if i < len(visibleLines) {
			renderedLines[i] = visibleLines[i]
		} else {
			renderedLines[i] = ""
		}
	}

	for i, line := range renderedLines {
		if lipgloss.Width(line) > m.vpWidth {
			renderedLines[i] = lipgloss.NewStyle().MaxWidth(m.vpWidth - 1).Render(line) + stDim.Render("…")
		}
	}

	logView := strings.Join(renderedLines, "\n")

	var sb strings.Builder
	sb.WriteString(stTitle.Render("log") + "\n")
	sb.WriteString(logView)
	if m.blockedEntry != "" {
		entry := m.blockedEntry
		if lipgloss.Width(entry) > inner {
			entry = lipgloss.NewStyle().MaxWidth(inner - 1).Render(entry) + stDim.Render("…")
		}
		sb.WriteString("\n" + entry)
	}
	return boxStyle.Width(inner).Render(sb.String())
}

// Hints renders the keybinding footer.
func (r *Renderer) Hints(m *Model) string {
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
	base := "  " + hint("p", pauseDesc) + sep + hint("a", "amend") + sep + hint("q", "quit")
	if m.errorRaw != "" {
		detailDesc := "details"
		if m.showDetail {
			detailDesc = "close"
		}
		base += sep + hint("ctrl+o", detailDesc)
	}
	return base
}

// DetailOverlay renders the error detail pane that replaces the log block.
func (r *Renderer) DetailOverlay(m *Model) string {
	var sb strings.Builder
	sb.WriteString(stTitle.Render("error detail") + "\n")

	lines := strings.Split(m.errorRaw, "\n")
	maxLines := m.vpHeight
	if m.errorFix != "" {
		maxLines -= 3
	}
	if maxLines < 2 {
		maxLines = 2
	}

	if len(lines) > maxLines {
		for _, l := range lines[:maxLines] {
			sb.WriteString(stDetail.Render(l) + "\n")
		}
		sb.WriteString(stDim.Render(fmt.Sprintf("... (%d more lines)", len(lines)-maxLines)))
	} else {
		for i, l := range lines {
			sb.WriteString(stDetail.Render(l))
			if i < len(lines)-1 {
				sb.WriteString("\n")
			}
		}
	}

	if m.errorFix != "" {
		sb.WriteString("\n\n" + stTitle.Render("suggestion") + "\n" + stText.Render(m.errorFix))
	}

	return boxStyle.Width(m.width - boxOverhead).Render(sb.String())
}

// JoinView composes the three blocks vertically.
func (r *Renderer) JoinView(parts ...string) string {
	return lipgloss.JoinVertical(lipgloss.Left, parts...)
}

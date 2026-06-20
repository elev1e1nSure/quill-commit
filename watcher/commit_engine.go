package watcher

import (
	"errors"
	"strings"

	"quill-commit/ai"
)

var errNoDiff = errors.New("no diff")

// CommitEngine performs git operations for committing, splitting, and amending.
type CommitEngine struct {
	git        gitOps
	emit       func(EventKind, string)
	emitDetail func(EventKind, string, string)
}

// newCommitEngine creates a CommitEngine using the provided git adapter.
func newCommitEngine(g gitOps, emit func(EventKind, string), emitDetail func(EventKind, string, string)) *CommitEngine {
	return &CommitEngine{git: g, emit: emit, emitDetail: emitDetail}
}

// Commit stages all changes and creates a single commit with the given message.
func (ce *CommitEngine) Commit(message string) error {
	diff, err := ce.git.Diff()
	if err != nil {
		ce.emit(EventError, formatGitError("git diff before commit", err))
		return err
	}
	if diff == "" {
		ce.emit(EventSkip, "diff cleared before commit, skipping")
		return errNoDiff
	}
	if err := ce.git.Add(); err != nil {
		ce.emit(EventError, formatGitError("git add", err))
		return err
	}
	if err := ce.git.Commit(message); err != nil {
		ce.emitDetail(EventCommitError, "commit blocked", err.Error())
		return err
	}
	ce.emit(EventCommit, message)
	return nil
}

// Split commits each group sequentially, then sweeps any leftover changes into
// a final commit so nothing is left behind.
func (ce *CommitEngine) Split(groups []ai.CommitGroup) error {
	committed := false
	for _, g := range groups {
		cleanFiles := cleanFileList(g.Files)
		if len(cleanFiles) == 0 || g.Message == "" {
			continue
		}
		if err := ce.git.AddPaths(cleanFiles); err != nil {
			ce.emit(EventError, formatGitError("split: git add "+strings.Join(cleanFiles, ", "), err))
			continue
		}
		if err := ce.git.Commit(g.Message); err != nil {
			ce.emitDetail(EventCommitError, "commit blocked", err.Error())
			return err
		}
		ce.emit(EventCommit, g.Message)
		committed = true
	}

	// Sweep any leftover changes the model didn't assign to a group.
	leftover, err := ce.git.Diff()
	if err == nil && leftover != "" {
		if err := ce.git.Add(); err == nil {
			if err := ce.git.Commit("chore: commit remaining changes"); err == nil {
				ce.emit(EventCommit, "chore: commit remaining changes")
				committed = true
			}
		}
	}

	if !committed {
		// Nothing landed — fall back so the cycle doesn't silently lose work.
		return ce.Commit("auto: fallback commit")
	}
	return nil
}

// Amend applies an amended commit message. If hasDiff is true it stages changes first.
func (ce *CommitEngine) Amend(message string, hasDiff bool) error {
	if hasDiff {
		if err := ce.git.Add(); err != nil {
			ce.emit(EventError, formatGitError("amend: git add", err))
			return err
		}
	}
	if err := ce.git.AmendCommit(message); err != nil {
		ce.emit(EventError, formatGitError("amend: git commit --amend", err))
		return err
	}
	ce.emit(EventAmend, message)
	return nil
}

func cleanFileList(files []string) []string {
	var clean []string
	for _, f := range files {
		f = strings.TrimSpace(f)
		if f != "" {
			clean = append(clean, f)
		}
	}
	return clean
}

func formatGitError(op string, err error) string {
	return op + ": " + err.Error()
}

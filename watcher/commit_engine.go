package watcher

import (
	"errors"
	"fmt"
	"strings"

	"quill-commit/ai"
)

var errNoDiff = errors.New("no diff")

// CommitEngine performs git operations for committing, splitting, and amending.
type CommitEngine struct {
	git        gitOps
	repoRoot   string
	emit       func(EventKind, string)
	emitDetail func(EventKind, string, string)
}

// newCommitEngine creates a CommitEngine using the provided git adapter.
func newCommitEngine(g gitOps, repoRoot string, emit func(EventKind, string), emitDetail func(EventKind, string, string)) *CommitEngine {
	return &CommitEngine{git: g, repoRoot: repoRoot, emit: emit, emitDetail: emitDetail}
}

// Commit stages only included changes and creates a single commit with the given message.
func (ce *CommitEngine) Commit(message string) error {
	res, err := ce.git.DiffEx(ce.repoRoot)
	if err != nil {
		ce.emit(EventError, formatGitError("git diff before commit", err))
		return err
	}
	if res.Diff == "" || len(res.IncludedFiles) == 0 {
		ce.emit(EventSkip, "diff cleared before commit, skipping")
		return errNoDiff
	}
	if err := ce.git.AddPaths(res.IncludedFiles); err != nil {
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
	res, err := ce.git.DiffEx(ce.repoRoot)
	if err != nil {
		ce.emit(EventError, formatGitError("git diff before split", err))
		return err
	}
	includedSet := make(map[string]struct{}, len(res.IncludedFiles))
	for _, f := range res.IncludedFiles {
		includedSet[f] = struct{}{}
	}

	committed := false
	for _, g := range groups {
		cleanFiles := cleanFileList(g.Files)
		if len(cleanFiles) == 0 || g.Message == "" {
			continue
		}
		// Only stage files that passed the filters.
		allowed := []string{}
		for _, f := range cleanFiles {
			if _, ok := includedSet[f]; ok {
				allowed = append(allowed, f)
			} else {
				ce.emit(EventInfo, fmt.Sprintf("split: skipping filtered file %s", f))
			}
		}
		if len(allowed) == 0 {
			continue
		}
		if err := ce.git.AddPaths(allowed); err != nil {
			ce.emit(EventError, formatGitError("split: git add "+strings.Join(allowed, ", "), err))
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
	leftoverRes, err := ce.git.DiffEx(ce.repoRoot)
	if err == nil && leftoverRes.Diff != "" && len(leftoverRes.IncludedFiles) > 0 {
		if err := ce.git.AddPaths(leftoverRes.IncludedFiles); err == nil {
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

// Amend applies an amended commit message. If hasDiff is true it stages included changes first.
func (ce *CommitEngine) Amend(message string, hasDiff bool) error {
	if hasDiff {
		res, err := ce.git.DiffEx(ce.repoRoot)
		if err != nil {
			ce.emit(EventError, formatGitError("amend: git diff", err))
			return err
		}
		if len(res.IncludedFiles) > 0 {
			if err := ce.git.AddPaths(res.IncludedFiles); err != nil {
				ce.emit(EventError, formatGitError("amend: git add", err))
				return err
			}
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

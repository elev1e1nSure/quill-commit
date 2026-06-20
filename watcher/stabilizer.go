package watcher

import (
	"fmt"
	"time"

	"quill-commit/config"
)

// Stabilizer waits until the git diff stops changing.
type Stabilizer struct {
	cfg     config.Config
	git     gitOps
	sleepFn func(time.Duration)
}

// newStabilizer creates a Stabilizer.
func newStabilizer(cfg config.Config, g gitOps, sleepFn func(time.Duration)) *Stabilizer {
	return &Stabilizer{cfg: cfg, git: g, sleepFn: sleepFn}
}

// Stabilize fetches the current diff and waits until it is unchanged for the
// configured stabilization period. onChange is called each time the diff
// changes during stabilization. It returns the stable diff, the new prevDiff
// value, and true on success. On error or empty diff it returns empty strings
// and false.
func (st *Stabilizer) Stabilize(prevDiff string, onChange func()) (stableDiff string, newPrevDiff string, ok bool) {
	diff, err := st.git.Diff()
	if err != nil {
		return "", "", false
	}

	if diff == "" {
		return "", "", false
	}

	for diff != prevDiff {
		if onChange != nil {
			onChange()
		}
		st.sleepFn(2 * time.Second)
		prevDiff = diff
		st.sleepFn(time.Duration(st.cfg.Stabilize * float64(time.Minute)))
		diff, err = st.git.Diff()
		if err != nil {
			return "", "", false
		}
		if diff == "" {
			return "", "", false
		}
	}

	return diff, diff, true
}

func formatDuration(minutes float64) string {
	d := time.Duration(minutes * float64(time.Minute))
	if d < time.Minute {
		return fmt.Sprintf("%ds", int(d.Seconds()))
	}
	m := int(d.Minutes())
	s := int(d.Seconds()) % 60
	if s == 0 {
		return fmt.Sprintf("%dm", m)
	}
	return fmt.Sprintf("%dm%ds", m, s)
}

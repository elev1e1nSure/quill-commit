package watcher

import (
	"context"
	"sync/atomic"
	"time"

	"quill-commit/config"
)

// Scheduler runs the periodic tick loop and handles pause/resume commands.
// Commands it does not recognize are forwarded to cmdHandler.
type Scheduler struct {
	interval   time.Duration
	paused     atomic.Bool
	cmds       <-chan Cmd
	tickFn     func()
	cmdHandler func(Cmd)
}

// newScheduler creates a Scheduler.
func newScheduler(cfg config.Config, cmds <-chan Cmd, tickFn func(), cmdHandler func(Cmd)) *Scheduler {
	interval := cfg.Interval
	if interval <= 0 {
		interval = config.DefaultInterval
	}
	return &Scheduler{
		interval:   time.Duration(interval * float64(time.Minute)),
		cmds:       cmds,
		tickFn:     tickFn,
		cmdHandler: cmdHandler,
	}
}

// IsPaused reports whether the scheduler is currently paused.
func (s *Scheduler) IsPaused() bool {
	return s.paused.Load()
}

// Run blocks until ctx is cancelled. It ticks on the configured interval and
// processes pause/resume commands.
func (s *Scheduler) Run(ctx context.Context) {
	ticker := time.NewTicker(s.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if !s.paused.Load() {
				s.tickFn()
			}
		case cmd := <-s.cmds:
			s.handleCmd(cmd)
		}
	}
}

func (s *Scheduler) handleCmd(cmd Cmd) {
	switch cmd.Kind {
	case CmdPause:
		s.paused.Store(true)
	case CmdResume:
		s.paused.Store(false)
	default:
		if s.cmdHandler != nil {
			s.cmdHandler(cmd)
		}
	}
}

package ui

import (
	"context"
	"errors"
	"log"
	"os/exec"
	"strings"
	"sync"
	"time"

	"github.com/gammons/slk/internal/config"
)

type inputSourceRunner interface {
	Current(ctx context.Context) (string, error)
	Select(ctx context.Context, source string) error
}

type commandInputSourceRunner struct {
	path string
}

func (r commandInputSourceRunner) Current(ctx context.Context) (string, error) {
	out, err := exec.CommandContext(ctx, r.path).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func (r commandInputSourceRunner) Select(ctx context.Context, source string) error {
	if source == "" {
		return nil
	}
	return exec.CommandContext(ctx, r.path, source).Run()
}

type inputSourceSwitcher struct {
	enabled           bool
	normalInputSource string
	restoreInsert     bool
	timeout           time.Duration
	runner            inputSourceRunner
	async             bool
	work              chan func()

	mu                   sync.Mutex
	previousInsertSource string
	previousFinderSource string
	disabled             bool
}

func newInputSourceSwitcher(cfg config.IMEConfig) *inputSourceSwitcher {
	if !cfg.AutoSwitch {
		return nil
	}
	normal := strings.TrimSpace(cfg.NormalInputSource)
	if normal == "" {
		normal = "com.apple.keylayout.ABC"
	}
	timeout := time.Duration(cfg.TimeoutMS) * time.Millisecond
	if timeout <= 0 {
		timeout = 200 * time.Millisecond
	}
	restore := true
	if cfg.RestoreInsert != nil {
		restore = *cfg.RestoreInsert
	}

	path, err := resolveInputSourceCommand(cfg.SwitcherCommand)
	if err != nil {
		log.Printf("ime switcher disabled: %v", err)
		return nil
	}

	s := &inputSourceSwitcher{
		enabled:           true,
		normalInputSource: normal,
		restoreInsert:     restore,
		timeout:           timeout,
		runner:            commandInputSourceRunner{path: path},
		async:             true,
		work:              make(chan func(), 32),
	}
	go s.runWorker()
	return s
}

func resolveInputSourceCommand(configured string) (string, error) {
	configured = strings.TrimSpace(configured)
	if configured != "" {
		path, err := exec.LookPath(configured)
		if err != nil {
			return "", err
		}
		return path, nil
	}
	for _, candidate := range []string{"macism", "im-select"} {
		if path, err := exec.LookPath(candidate); err == nil {
			return path, nil
		}
	}
	return "", errors.New("macism or im-select not found")
}

func (s *inputSourceSwitcher) OnModeChange(prev, next Mode) {
	if s == nil || !s.enabled || prev == next {
		return
	}
	if prev == ModeInsert && next == ModeNormal {
		s.dispatch(s.switchToNormal)
		return
	}
	if prev != ModeInsert && next == ModeInsert {
		s.dispatch(s.restoreForInsert)
		return
	}
	// Channel finder (Ctrl+T) only matches single-byte ASCII keys, so
	// Korean / CJK queries are silently dropped. Force the input source
	// to the configured English layout on entry and restore the prior
	// source on exit. Stored separately from previousInsertSource so
	// the two restore paths don't clobber each other (a stash from
	// Insert→Normal is not the same as a stash from
	// Normal→ChannelFinder).
	if prev != ModeChannelFinder && next == ModeChannelFinder {
		s.dispatch(s.enterChannelFinder)
		return
	}
	if prev == ModeChannelFinder && next != ModeChannelFinder {
		s.dispatch(s.leaveChannelFinder)
		return
	}
}

func (s *inputSourceSwitcher) dispatch(fn func()) {
	if !s.async {
		fn()
		return
	}
	if s.work == nil {
		return
	}
	select {
	case s.work <- fn:
	default:
		log.Printf("ime switcher: dropping mode transition because worker queue is full")
	}
}

func (s *inputSourceSwitcher) runWorker() {
	for fn := range s.work {
		fn()
	}
}

func (s *inputSourceSwitcher) switchToNormal() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disabled {
		return
	}

	if s.restoreInsert {
		if current, err := s.current(); err != nil {
			log.Printf("ime switcher: current input source: %v", err)
		} else if current != "" {
			s.previousInsertSource = current
		}
	}
	if err := s.selectSource(s.normalInputSource); err != nil {
		log.Printf("ime switcher: switch to %q: %v", s.normalInputSource, err)
	}
}

func (s *inputSourceSwitcher) restoreForInsert() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disabled || !s.restoreInsert || s.previousInsertSource == "" {
		return
	}
	if err := s.selectSource(s.previousInsertSource); err != nil {
		log.Printf("ime switcher: restore %q: %v", s.previousInsertSource, err)
	}
}

func (s *inputSourceSwitcher) enterChannelFinder() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disabled {
		return
	}
	// Always stash so leaveChannelFinder can put it back even when
	// restoreInsert is disabled — the user-facing promise here is
	// "force English for the duration of the overlay", and the
	// restore is independent from the Insert-mode restore policy.
	if current, err := s.current(); err != nil {
		log.Printf("ime switcher: current input source: %v", err)
	} else if current != "" {
		s.previousFinderSource = current
	}
	if err := s.selectSource(s.normalInputSource); err != nil {
		log.Printf("ime switcher: switch to %q: %v", s.normalInputSource, err)
	}
}

func (s *inputSourceSwitcher) leaveChannelFinder() {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.disabled || s.previousFinderSource == "" {
		return
	}
	target := s.previousFinderSource
	s.previousFinderSource = ""
	if err := s.selectSource(target); err != nil {
		log.Printf("ime switcher: restore %q: %v", target, err)
	}
}

func (s *inputSourceSwitcher) current() (string, error) {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	return s.runner.Current(ctx)
}

func (s *inputSourceSwitcher) selectSource(source string) error {
	ctx, cancel := context.WithTimeout(context.Background(), s.timeout)
	defer cancel()
	return s.runner.Select(ctx, source)
}

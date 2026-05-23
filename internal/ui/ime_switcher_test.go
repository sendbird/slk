package ui

import (
	"context"
	"errors"
	"testing"
	"time"
)

type fakeInputSourceRunner struct {
	currents   []string
	currentErr error
	selectErr  error
	selected   []string
}

func (f *fakeInputSourceRunner) Current(ctx context.Context) (string, error) {
	if f.currentErr != nil {
		return "", f.currentErr
	}
	if len(f.currents) == 0 {
		return "", nil
	}
	current := f.currents[0]
	f.currents = f.currents[1:]
	return current, nil
}

func (f *fakeInputSourceRunner) Select(ctx context.Context, source string) error {
	f.selected = append(f.selected, source)
	return f.selectErr
}

func testInputSourceSwitcher(r *fakeInputSourceRunner) *inputSourceSwitcher {
	return &inputSourceSwitcher{
		enabled:           true,
		normalInputSource: "com.apple.keylayout.ABC",
		restoreInsert:     true,
		timeout:           time.Second,
		runner:            r,
		async:             false,
	}
}

func TestInputSourceSwitcherInsertToNormalStoresAndSwitches(t *testing.T) {
	r := &fakeInputSourceRunner{currents: []string{"com.apple.inputmethod.Korean.2SetKorean"}}
	s := testInputSourceSwitcher(r)

	s.OnModeChange(ModeInsert, ModeNormal)

	if got, want := s.previousInsertSource, "com.apple.inputmethod.Korean.2SetKorean"; got != want {
		t.Fatalf("previousInsertSource = %q, want %q", got, want)
	}
	if got, want := r.selected, []string{"com.apple.keylayout.ABC"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("selected = %#v, want %#v", got, want)
	}
}

func TestInputSourceSwitcherNormalToInsertRestoresPrevious(t *testing.T) {
	r := &fakeInputSourceRunner{}
	s := testInputSourceSwitcher(r)
	s.previousInsertSource = "com.apple.inputmethod.Korean.2SetKorean"

	s.OnModeChange(ModeNormal, ModeInsert)

	if got, want := r.selected, []string{"com.apple.inputmethod.Korean.2SetKorean"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("selected = %#v, want %#v", got, want)
	}
}

func TestInputSourceSwitcherRestoreDisabled(t *testing.T) {
	r := &fakeInputSourceRunner{currents: []string{"com.apple.inputmethod.Korean.2SetKorean"}}
	s := testInputSourceSwitcher(r)
	s.restoreInsert = false

	s.OnModeChange(ModeInsert, ModeNormal)
	s.OnModeChange(ModeNormal, ModeInsert)

	if s.previousInsertSource != "" {
		t.Fatalf("previousInsertSource = %q, want empty", s.previousInsertSource)
	}
	if got, want := r.selected, []string{"com.apple.keylayout.ABC"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("selected = %#v, want %#v", got, want)
	}
}

func TestInputSourceSwitcherSameModeNoop(t *testing.T) {
	r := &fakeInputSourceRunner{currents: []string{"com.apple.inputmethod.Korean.2SetKorean"}}
	s := testInputSourceSwitcher(r)

	s.OnModeChange(ModeNormal, ModeNormal)
	s.OnModeChange(ModeInsert, ModeInsert)

	if len(r.selected) != 0 {
		t.Fatalf("selected = %#v, want none", r.selected)
	}
	if s.previousInsertSource != "" {
		t.Fatalf("previousInsertSource = %q, want empty", s.previousInsertSource)
	}
}

func TestInputSourceSwitcherMissingPreviousNoopOnInsert(t *testing.T) {
	r := &fakeInputSourceRunner{}
	s := testInputSourceSwitcher(r)

	s.OnModeChange(ModeNormal, ModeInsert)

	if len(r.selected) != 0 {
		t.Fatalf("selected = %#v, want none", r.selected)
	}
}

func TestInputSourceSwitcherCurrentFailureStillSwitchesNormal(t *testing.T) {
	r := &fakeInputSourceRunner{currentErr: errors.New("boom")}
	s := testInputSourceSwitcher(r)

	s.OnModeChange(ModeInsert, ModeNormal)

	if s.previousInsertSource != "" {
		t.Fatalf("previousInsertSource = %q, want empty", s.previousInsertSource)
	}
	if got, want := r.selected, []string{"com.apple.keylayout.ABC"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("selected = %#v, want %#v", got, want)
	}
}

func TestInputSourceSwitcherChannelFinderEnterStoresAndSwitchesEnglish(t *testing.T) {
	r := &fakeInputSourceRunner{currents: []string{"com.apple.inputmethod.Korean.2SetKorean"}}
	s := testInputSourceSwitcher(r)

	s.OnModeChange(ModeNormal, ModeChannelFinder)

	if got, want := s.previousFinderSource, "com.apple.inputmethod.Korean.2SetKorean"; got != want {
		t.Fatalf("previousFinderSource = %q, want %q", got, want)
	}
	if got, want := r.selected, []string{"com.apple.keylayout.ABC"}; len(got) != len(want) || got[0] != want[0] {
		t.Fatalf("selected = %#v, want %#v", got, want)
	}
}

func TestInputSourceSwitcherChannelFinderLeaveRestoresPrevious(t *testing.T) {
	r := &fakeInputSourceRunner{currents: []string{"com.apple.inputmethod.Korean.2SetKorean"}}
	s := testInputSourceSwitcher(r)

	s.OnModeChange(ModeNormal, ModeChannelFinder)
	s.OnModeChange(ModeChannelFinder, ModeNormal)

	if s.previousFinderSource != "" {
		t.Fatalf("previousFinderSource = %q, want cleared after restore", s.previousFinderSource)
	}
	if got, want := r.selected, []string{"com.apple.keylayout.ABC", "com.apple.inputmethod.Korean.2SetKorean"}; len(got) != len(want) || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("selected = %#v, want %#v", got, want)
	}
}

// The channel-finder stash must NOT be clobbered by an Insert→Normal
// transition happening in between (e.g., user enters Insert briefly,
// returns to Normal — the channel finder slot belongs to ChannelFinder).
func TestInputSourceSwitcherChannelFinderSlotDoesNotClobberInsertSlot(t *testing.T) {
	r := &fakeInputSourceRunner{
		currents: []string{
			"com.apple.inputmethod.Korean.2SetKorean", // captured by Insert→Normal
			"com.apple.keylayout.ABC",                 // captured by Normal→ChannelFinder
		},
	}
	s := testInputSourceSwitcher(r)

	s.OnModeChange(ModeInsert, ModeNormal)
	s.OnModeChange(ModeNormal, ModeChannelFinder)

	if got, want := s.previousInsertSource, "com.apple.inputmethod.Korean.2SetKorean"; got != want {
		t.Fatalf("previousInsertSource = %q, want %q", got, want)
	}
	if got, want := s.previousFinderSource, "com.apple.keylayout.ABC"; got != want {
		t.Fatalf("previousFinderSource = %q, want %q", got, want)
	}
}

func TestInputSourceSwitcherChannelFinderLeaveWithoutEnterNoop(t *testing.T) {
	r := &fakeInputSourceRunner{}
	s := testInputSourceSwitcher(r)

	// Never entered ChannelFinder; a stray leave (e.g. mode flipped
	// programmatically) must not call Select.
	s.OnModeChange(ModeChannelFinder, ModeNormal)

	if len(r.selected) != 0 {
		t.Fatalf("selected = %#v, want none", r.selected)
	}
}

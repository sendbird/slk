package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// Regression for the user-reported bug where pressing Enter on a
// channel row collapsed the section instead of opening the channel.
// Cause: handleEnter delegated to sidebar.ToggleCollapseSelected,
// which was widened for the spacebar affordance to also collapse a
// row's parent section. Enter must keep its "open this channel"
// meaning on channel rows.
func TestEnterOnChannelRow_OpensChannel(t *testing.T) {
	app := NewApp()
	app.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})
	// Expand Channels (collapsed by default) so the cursor can land on C1.
	app.sidebar.ToggleCollapse("Channels")
	// Nav: Threads → Activity → Channels header → C1.
	app.sidebar.MoveDown() // Activity
	app.sidebar.MoveDown() // Channels header
	app.sidebar.MoveDown() // C1
	if got := app.sidebar.SelectedID(); got != "C1" {
		t.Fatalf("setup: cursor should be on C1, got %q", got)
	}

	app.focusedPanel = PanelSidebar
	app.SetMode(ModeNormal)

	if app.sidebar.IsCollapsed("Channels") {
		t.Fatalf("setup: Channels should be expanded")
	}

	cmd := app.handleNormalMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	selected, ok := queuedChannelSelectedMsg(cmd)
	if !ok {
		t.Fatalf("expected Enter on C1 to queue ChannelSelectedMsg")
	}
	if selected.ID != "C1" {
		t.Errorf("queued ChannelSelectedMsg.ID = %q, want C1", selected.ID)
	}
	if app.sidebar.IsCollapsed("Channels") {
		t.Errorf("Enter on a channel row must NOT collapse the parent section")
	}
}

// Sibling test: Enter on a section header should still toggle the
// section. (This preserves the keyboard-only collapse path the help
// text documents.)
func TestEnterOnSectionHeader_TogglesSection(t *testing.T) {
	app := NewApp()
	app.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})
	app.sidebar.MoveDown() // Activity
	app.sidebar.MoveDown() // Channels header
	if name, ok := app.sidebar.IsSectionHeaderSelected(); !ok || name != "Channels" {
		t.Fatalf("setup: cursor should be on Channels header, got name=%q ok=%v", name, ok)
	}

	app.focusedPanel = PanelSidebar
	app.SetMode(ModeNormal)

	was := app.sidebar.IsCollapsed("Channels")
	cmd := app.handleNormalMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	if cmd != nil {
		if msg := cmd(); msg != nil {
			t.Errorf("Enter on a section header must not queue a channel-selected msg, got %T", msg)
		}
	}
	if app.sidebar.IsCollapsed("Channels") == was {
		t.Errorf("Enter on Channels header should toggle collapse state")
	}
}

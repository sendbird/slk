package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// TestSpacebarTogglesSection_OnDMHeader is the regression test for the
// user-reported bug where pressing space on the Direct Messages header
// did not collapse the section. The keymap binds " " to ToggleSection,
// the App.handleNormalMode dispatches to sidebar.ToggleCollapseSelected
// when focusedPanel == PanelSidebar, and the sidebar's
// IsSectionHeaderSelected accepts default-named headers — so a space
// keypress while focused on the sidebar with the cursor on the Direct
// Messages header must flip its collapsed state.
func TestSpacebarTogglesSection_OnDMHeader(t *testing.T) {
	app := NewApp()
	app.SetChannels([]sidebar.ChannelItem{
		{ID: "D1", Name: "alice", Type: "dm"},
	})
	// Move cursor onto the Direct Messages header. Nav order with a
	// single DM and no DMs section collapsed by default:
	// Threads → Activity → Direct Messages header → D1.
	app.sidebar.MoveDown() // Activity
	app.sidebar.MoveDown() // Direct Messages header
	if name, ok := app.sidebar.IsSectionHeaderSelected(); !ok || name == "" {
		t.Fatalf("setup: cursor should be on a section header, got name=%q ok=%v", name, ok)
	}

	// Focus must be on the sidebar for the toggle path to run.
	app.focusedPanel = PanelSidebar
	app.SetMode(ModeNormal)

	wasCollapsed := app.sidebar.IsCollapsed("Direct Messages")
	app.handleNormalMode(tea.KeyPressMsg{Code: ' ', Text: " "})

	if app.sidebar.IsCollapsed("Direct Messages") == wasCollapsed {
		t.Fatalf("space on DM header did not toggle collapse state (still %v)", wasCollapsed)
	}
}

func TestSpacebarTogglesSection_OnChannelsHeader(t *testing.T) {
	app := NewApp()
	app.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})
	app.sidebar.MoveDown() // Activity
	app.sidebar.MoveDown() // Channels header (collapsed by default)
	if name, ok := app.sidebar.IsSectionHeaderSelected(); !ok || name == "" {
		t.Fatalf("setup: cursor should be on a section header, got name=%q ok=%v", name, ok)
	}

	app.focusedPanel = PanelSidebar
	app.SetMode(ModeNormal)

	wasCollapsed := app.sidebar.IsCollapsed("Channels")
	app.handleNormalMode(tea.KeyPressMsg{Code: ' ', Text: " "})

	if app.sidebar.IsCollapsed("Channels") == wasCollapsed {
		t.Fatalf("space on Channels header did not toggle collapse state (still %v)", wasCollapsed)
	}
}

package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/ui/sidebar"
)

// TestApp_ClickOnChannelsHeaderTogglesCollapse asserts that a left
// mouse click on the Channels section header in the sidebar toggles
// its collapsed state, mirroring the Enter-on-header keyboard path.
func TestApp_ClickOnChannelsHeaderTogglesCollapse(t *testing.T) {
	a := NewApp()
	a.width = 120
	a.height = 30
	a.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})
	_ = a.View()

	// Channels section starts collapsed by default. Find the header's
	// sidebar-local row by walking ClickAt; the row whose cursor is on
	// the "Channels" header is what we want.
	headerY := -1
	for y := 0; y < 20; y++ {
		_, _ = a.sidebar.ClickAt(y)
		if name, ok := a.sidebar.IsSectionHeaderSelected(); ok && name == "Channels" {
			headerY = y
			break
		}
	}
	if headerY < 0 {
		t.Fatalf("Channels header row not found in sidebar")
	}

	wasCollapsed := a.sidebar.IsCollapsed("Channels")
	// Mouse Y is sidebar-local + 1 (the app strips the top border).
	_, cmd := a.Update(tea.MouseClickMsg{X: a.layoutRailWidth + 1, Y: headerY + 1, Button: tea.MouseLeft})
	if cmd != nil {
		_ = drainBatch(cmd)
	}

	if a.sidebar.IsCollapsed("Channels") == wasCollapsed {
		t.Errorf("clicking the Channels header did not toggle collapse state (still %v)", wasCollapsed)
	}

	// A second click should flip it back.
	wasCollapsed = a.sidebar.IsCollapsed("Channels")
	_, cmd = a.Update(tea.MouseClickMsg{X: a.layoutRailWidth + 1, Y: headerY + 1, Button: tea.MouseLeft})
	if cmd != nil {
		_ = drainBatch(cmd)
	}
	if a.sidebar.IsCollapsed("Channels") == wasCollapsed {
		t.Errorf("second click did not flip Channels collapse state back")
	}
}

// TestApp_ClickOnDirectMessagesHeaderTogglesCollapse asserts that a
// click on the Direct Messages header toggles its collapsed state.
func TestApp_ClickOnDirectMessagesHeaderTogglesCollapse(t *testing.T) {
	a := NewApp()
	a.width = 120
	a.height = 30
	a.SetChannels([]sidebar.ChannelItem{
		{ID: "D1", Name: "alice", Type: "dm"},
	})
	_ = a.View()

	headerY := -1
	for y := 0; y < 20; y++ {
		_, _ = a.sidebar.ClickAt(y)
		if name, ok := a.sidebar.IsSectionHeaderSelected(); ok && name == "Direct Messages" {
			headerY = y
			break
		}
	}
	if headerY < 0 {
		t.Fatalf("Direct Messages header row not found in sidebar")
	}

	wasCollapsed := a.sidebar.IsCollapsed("Direct Messages")
	_, cmd := a.Update(tea.MouseClickMsg{X: a.layoutRailWidth + 1, Y: headerY + 1, Button: tea.MouseLeft})
	if cmd != nil {
		_ = drainBatch(cmd)
	}
	if a.sidebar.IsCollapsed("Direct Messages") == wasCollapsed {
		t.Errorf("clicking the Direct Messages header did not toggle collapse state (still %v)", wasCollapsed)
	}
}

// TestApp_ClickOnChannelRowStillOpensChannel guards against the
// header-click change accidentally swallowing channel-row clicks.
func TestApp_ClickOnChannelRowStillOpensChannel(t *testing.T) {
	a := NewApp()
	a.width = 120
	a.height = 30
	a.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})
	// Expand Channels (it starts collapsed) so the channel row is
	// reachable.
	a.sidebar.ToggleCollapse("Channels")
	_ = a.View()

	rowY := -1
	for y := 0; y < 20; y++ {
		if item, ok := a.sidebar.ClickAt(y); ok && item.ID == "C1" {
			rowY = y
			break
		}
	}
	if rowY < 0 {
		t.Fatalf("C1 row not found in sidebar")
	}

	_, cmd := a.Update(tea.MouseClickMsg{X: a.layoutRailWidth + 1, Y: rowY + 1, Button: tea.MouseLeft})
	selected, ok := queuedChannelSelectedMsg(cmd)
	if !ok {
		t.Fatalf("expected click on C1 row to queue ChannelSelectedMsg")
	}
	if selected.ID != "C1" {
		t.Errorf("queued ChannelSelectedMsg.ID = %q, want C1", selected.ID)
	}
	if a.sidebar.IsCollapsed("Channels") {
		t.Errorf("clicking a channel row must not collapse its parent section")
	}
}

// TestApp_MouseWheelOverSidebarMovesFocusAndCursor asserts that a
// wheel notch with the cursor over the sidebar (a) moves keyboard
// focus to PanelSidebar and (b) only mutates the sidebar's cursor,
// leaving the messages pane alone.
func TestApp_MouseWheelOverSidebarMovesFocusAndCursor(t *testing.T) {
	a := NewApp()
	a.width = 120
	a.height = 30
	a.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})
	a.sidebar.ToggleCollapse("Channels") // expand so cursor can land on C1
	a.focusedPanel = PanelMessages       // simulate keyboard focus elsewhere
	_ = a.View()

	priorMsgVer := a.messagepane.Version()

	_, _ = a.Update(tea.MouseWheelMsg{
		X:      a.layoutRailWidth + 1,
		Y:      4,
		Button: tea.MouseWheelDown,
	})

	if a.focusedPanel != PanelSidebar {
		t.Errorf("focusedPanel after sidebar wheel = %v, want PanelSidebar", a.focusedPanel)
	}
	if a.messagepane.Version() != priorMsgVer {
		t.Errorf("messagepane.Version changed on sidebar wheel; should be untouched")
	}
}

// TestApp_MouseWheelOverMessagesMovesFocus asserts that wheeling over
// the messages pane moves focus to PanelMessages.
func TestApp_MouseWheelOverMessagesMovesFocus(t *testing.T) {
	a := NewApp()
	a.width = 120
	a.height = 30
	a.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})
	a.focusedPanel = PanelSidebar
	_ = a.View()

	_, _ = a.Update(tea.MouseWheelMsg{
		X:      a.layoutSidebarEnd + 1,
		Y:      5,
		Button: tea.MouseWheelDown,
	})

	if a.focusedPanel != PanelMessages {
		t.Errorf("focusedPanel after messages wheel = %v, want PanelMessages", a.focusedPanel)
	}
}

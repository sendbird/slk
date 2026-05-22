package sidebar

import (
	"testing"
)

func TestToggleCollapseSelected_OnSectionHeader(t *testing.T) {
	m := New([]ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})
	// Channels section starts collapsed; navigate cursor to its header.
	// Nav order: Threads, Activity, Direct Messages? (only if any DM), Channels header.
	// We only have a channel, so nav is: Threads, Activity, Channels header.
	m.MoveDown() // Activity
	m.MoveDown() // Channels header
	if name, ok := m.IsSectionHeaderSelected(); !ok || name != defaultChannelsSection {
		t.Fatalf("setup: expected Channels header selected, got name=%q ok=%v", name, ok)
	}
	if !m.IsCollapsed(defaultChannelsSection) {
		t.Fatalf("setup: expected Channels collapsed by default")
	}
	if !m.ToggleCollapseSelected() {
		t.Fatalf("expected toggle to succeed on Channels header")
	}
	if m.IsCollapsed(defaultChannelsSection) {
		t.Errorf("expected Channels section expanded after toggle")
	}
}

func TestToggleCollapseSelected_OnChannelRow_CollapsesParentSection(t *testing.T) {
	// Cursor on a channel row should toggle that row's parent section,
	// so users don't need to land precisely on the header line to hide
	// the section.
	m := New([]ChannelItem{
		{ID: "D1", Name: "alice", Type: "dm"},
	})
	// DMs are expanded by default. Nav: Threads, Activity, Direct Messages header, D1.
	m.MoveDown() // Activity
	m.MoveDown() // Direct Messages header
	m.MoveDown() // D1
	if got := m.SelectedID(); got != "D1" {
		t.Fatalf("setup: expected D1 selected, got %q", got)
	}
	if m.IsCollapsed(defaultDMSection) {
		t.Fatalf("setup: DMs expected expanded by default")
	}
	if !m.ToggleCollapseSelected() {
		t.Fatalf("expected toggle to succeed when cursor is on a channel row")
	}
	if !m.IsCollapsed(defaultDMSection) {
		t.Errorf("expected Direct Messages section collapsed after toggling from a DM row")
	}
}

func TestToggleCollapseSelected_OnThreadsRow_Noop(t *testing.T) {
	m := New([]ChannelItem{{ID: "C1", Name: "general", Type: "channel"}})
	// Default cursor is on Threads row.
	if !m.IsThreadsSelected() {
		t.Fatalf("setup: expected Threads selected by default")
	}
	if m.ToggleCollapseSelected() {
		t.Errorf("toggle on Threads row should be a no-op")
	}
}

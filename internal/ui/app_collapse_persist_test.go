package ui

import (
	"testing"

	"github.com/gammons/slk/internal/ui/sidebar"
)

// TestWorkspaceReady_AppliesPersistedCollapseState exercises the
// WorkspaceReadyMsg → sidebar.ApplyPersistedCollapse path end to end.
func TestWorkspaceReady_AppliesPersistedCollapseState(t *testing.T) {
	app := NewApp()
	if !app.sidebar.IsCollapsed("Channels") {
		t.Fatalf("setup: Channels expected collapsed by default")
	}

	app.Update(WorkspaceReadyMsg{
		TeamID:        "T1",
		TeamName:      "Acme",
		Channels:      []sidebar.ChannelItem{{ID: "C1", Name: "general", Type: "channel"}},
		InitialActive: true,
		CollapsedSections: map[string]bool{
			"Channels":        false, // user expanded last session
			"Direct Messages": true,  // user collapsed last session
		},
	})

	if app.sidebar.IsCollapsed("Channels") {
		t.Errorf("Channels = collapsed after restore; want expanded")
	}
	if !app.sidebar.IsCollapsed("Direct Messages") {
		t.Errorf("Direct Messages = expanded after restore; want collapsed")
	}
}

// TestSidebarCollapsePersister_FiresOnToggle confirms that toggling a
// section from the App invokes the registered persister with the new
// state. main.go's wiring writes that to SQLite, so this is the
// regression guard for the wiring contract.
func TestSidebarCollapsePersister_FiresOnToggle(t *testing.T) {
	app := NewApp()
	app.SetChannels([]sidebar.ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
	})

	type call struct {
		key       string
		collapsed bool
	}
	var calls []call
	app.SetSidebarCollapsePersister(func(key string, collapsed bool) {
		calls = append(calls, call{key, collapsed})
	})

	app.sidebar.ToggleCollapse("Channels")

	if len(calls) != 1 {
		t.Fatalf("persister fired %d times, want 1", len(calls))
	}
	if calls[0].key != "Channels" {
		t.Errorf("persister key = %q, want Channels", calls[0].key)
	}
	// Channels was collapsed → toggling expands → collapsed=false.
	if calls[0].collapsed {
		t.Errorf("persister collapsed = true, want false (toggle expanded)")
	}
}

package sidebar

import "testing"

func TestApplyPersistedCollapse_OverridesDefaults(t *testing.T) {
	// Channels and Apps are collapsed by default; persisting
	// Channels=false (user expanded it last session) must override.
	m := New([]ChannelItem{{ID: "C1", Name: "general", Type: "channel"}})
	if !m.IsCollapsed("Channels") {
		t.Fatalf("setup: Channels expected collapsed by default")
	}

	m.ApplyPersistedCollapse(map[string]bool{
		"Channels":        false,
		"Direct Messages": true,
	})

	if m.IsCollapsed("Channels") {
		t.Errorf("Channels = collapsed, want expanded after restore")
	}
	if !m.IsCollapsed("Direct Messages") {
		t.Errorf("Direct Messages = expanded, want collapsed after restore")
	}
	// Apps wasn't in the persisted map; its default (collapsed) is kept.
	if !m.IsCollapsed("Apps") {
		t.Errorf("Apps default = expanded, want collapsed (untouched defaults must survive)")
	}
}

func TestApplyPersistedCollapse_NilMapIsSafe(t *testing.T) {
	m := New([]ChannelItem{{ID: "C1", Name: "general", Type: "channel"}})
	// Channels collapsed by default; nil restore must preserve it.
	m.ApplyPersistedCollapse(nil)
	if !m.IsCollapsed("Channels") {
		t.Errorf("Channels collapsed default lost after nil restore")
	}
}

func TestApplyPersistedCollapse_DoesNotFireOnChangeCallback(t *testing.T) {
	// Restoration is not a toggle — the persister callback (which
	// writes to SQLite) must not fire while we're hydrating from
	// SQLite ourselves, otherwise the workspace would feedback-loop
	// rewriting its own freshly-loaded rows.
	m := New([]ChannelItem{{ID: "C1", Name: "general", Type: "channel"}})
	fired := 0
	m.SetOnCollapseChange(func(string, bool) { fired++ })

	m.ApplyPersistedCollapse(map[string]bool{"Channels": false})

	if fired != 0 {
		t.Errorf("onCollapseChange fired %d times during restore; want 0", fired)
	}
}

func TestToggleCollapse_FiresOnChangeCallback(t *testing.T) {
	// A user toggle must invoke the persister with the new state.
	m := New([]ChannelItem{{ID: "C1", Name: "general", Type: "channel"}})
	var lastKey string
	var lastCollapsed bool
	fired := 0
	m.SetOnCollapseChange(func(key string, collapsed bool) {
		lastKey = key
		lastCollapsed = collapsed
		fired++
	})

	// Channels starts collapsed by default → toggling expands it.
	m.ToggleCollapse("Channels")
	if fired != 1 {
		t.Fatalf("onCollapseChange fired %d times, want 1", fired)
	}
	if lastKey != "Channels" || lastCollapsed != false {
		t.Errorf("callback args = (%q, %v), want (%q, %v)", lastKey, lastCollapsed, "Channels", false)
	}

	// Toggle again → collapses again.
	m.ToggleCollapse("Channels")
	if fired != 2 {
		t.Fatalf("onCollapseChange fired %d times after 2nd toggle, want 2", fired)
	}
	if lastCollapsed != true {
		t.Errorf("after 2nd toggle, collapsed = %v, want true", lastCollapsed)
	}
}

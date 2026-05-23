package cache

import "testing"

func TestSidebarSectionCollapsed_RoundTrip(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	// Empty state → empty map.
	got, err := db.GetSidebarSectionCollapsed("T1")
	if err != nil {
		t.Fatalf("GetSidebarSectionCollapsed empty: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("empty workspace = %v, want empty", got)
	}

	// Persist a few values.
	if err := db.SetSidebarSectionCollapsed("T1", "Channels", true); err != nil {
		t.Fatalf("Set Channels=true: %v", err)
	}
	if err := db.SetSidebarSectionCollapsed("T1", "Direct Messages", false); err != nil {
		t.Fatalf("Set DMs=false: %v", err)
	}
	if err := db.SetSidebarSectionCollapsed("T1", "Apps", true); err != nil {
		t.Fatalf("Set Apps=true: %v", err)
	}

	got, err = db.GetSidebarSectionCollapsed("T1")
	if err != nil {
		t.Fatalf("GetSidebarSectionCollapsed: %v", err)
	}
	if want := map[string]bool{"Channels": true, "Direct Messages": false, "Apps": true}; !mapEq(got, want) {
		t.Errorf("got = %v, want %v", got, want)
	}

	// Toggling overwrites.
	if err := db.SetSidebarSectionCollapsed("T1", "Channels", false); err != nil {
		t.Fatalf("toggle Channels=false: %v", err)
	}
	got, _ = db.GetSidebarSectionCollapsed("T1")
	if got["Channels"] != false {
		t.Errorf("Channels = %v after toggle, want false", got["Channels"])
	}
}

func TestSidebarSectionCollapsed_PerWorkspaceIsolation(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	if err := db.UpsertWorkspace(Workspace{ID: "T2", Name: "T2"}); err != nil {
		t.Fatalf("UpsertWorkspace T2: %v", err)
	}

	if err := db.SetSidebarSectionCollapsed("T1", "Channels", true); err != nil {
		t.Fatalf("T1 set: %v", err)
	}
	if err := db.SetSidebarSectionCollapsed("T2", "Channels", false); err != nil {
		t.Fatalf("T2 set: %v", err)
	}

	gotT1, _ := db.GetSidebarSectionCollapsed("T1")
	gotT2, _ := db.GetSidebarSectionCollapsed("T2")
	if gotT1["Channels"] != true {
		t.Errorf("T1 Channels = %v, want true", gotT1["Channels"])
	}
	if gotT2["Channels"] != false {
		t.Errorf("T2 Channels = %v, want false", gotT2["Channels"])
	}
}

func TestSidebarSectionCollapsed_RejectsEmptyKey(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	if err := db.SetSidebarSectionCollapsed("T1", "", true); err == nil {
		t.Error("empty section_key should be rejected")
	}
	if err := db.SetSidebarSectionCollapsed("", "Channels", true); err == nil {
		t.Error("empty workspace_id should be rejected")
	}
}

func mapEq(a, b map[string]bool) bool {
	if len(a) != len(b) {
		return false
	}
	for k, v := range a {
		if b[k] != v {
			return false
		}
	}
	return true
}

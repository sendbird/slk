package cache

import "testing"

func TestSetAndGetChannelMentionCount(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true})

	// Default is 0 for a freshly upserted channel.
	if got := db.GetChannelMentionCount("C1"); got != 0 {
		t.Errorf("default mention_count = %d, want 0", got)
	}

	if err := db.SetChannelMentionCount("C1", 7); err != nil {
		t.Fatalf("SetChannelMentionCount: %v", err)
	}
	if got := db.GetChannelMentionCount("C1"); got != 7 {
		t.Errorf("after set, mention_count = %d, want 7", got)
	}

	// Negative values clamp to 0 so callers can shovel Slack payloads
	// in without sanitizing.
	if err := db.SetChannelMentionCount("C1", -3); err != nil {
		t.Fatalf("SetChannelMentionCount(-3): %v", err)
	}
	if got := db.GetChannelMentionCount("C1"); got != 0 {
		t.Errorf("after set(-3), mention_count = %d, want 0", got)
	}

	// Missing rows return 0 rather than an error — matches GetChannelLatestSyncedTS.
	if got := db.GetChannelMentionCount("C-nonexistent"); got != 0 {
		t.Errorf("missing channel mention_count = %d, want 0", got)
	}
}

func TestIncrementChannelMentionCount(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true})

	got, err := db.IncrementChannelMentionCount("C1")
	if err != nil {
		t.Fatalf("first increment: %v", err)
	}
	if got != 1 {
		t.Errorf("after 1st increment = %d, want 1", got)
	}
	got, err = db.IncrementChannelMentionCount("C1")
	if err != nil {
		t.Fatalf("second increment: %v", err)
	}
	if got != 2 {
		t.Errorf("after 2nd increment = %d, want 2", got)
	}
}

func TestIncrementChannelMentionCountIfUnread_NoOpWhenRead(t *testing.T) {
	// Closes the WS-handler race documented on
	// IncrementChannelMentionCountIfUnread: if channel_marked clears
	// has_unread between OnMessage's two writes, the late increment
	// must not resurrect the badge. Verify by leaving has_unread=0
	// and confirming mention_count stays 0.
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true})

	// Simulate post-mark-read state: has_unread=0, mention_count=0.
	if err := db.UpdateChannelReadState("C1", "1.0", false); err != nil {
		t.Fatalf("UpdateChannelReadState: %v", err)
	}

	got, err := db.IncrementChannelMentionCountIfUnread("C1")
	if err != nil {
		t.Fatalf("IncrementChannelMentionCountIfUnread: %v", err)
	}
	if got != 0 {
		t.Errorf("incrementing a read channel = %d, want 0", got)
	}
}

func TestIncrementChannelMentionCountIfUnread_BumpsWhenUnread(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true})

	if err := db.UpdateChannelReadState("C1", "", true); err != nil {
		t.Fatalf("UpdateChannelReadState: %v", err)
	}
	got, err := db.IncrementChannelMentionCountIfUnread("C1")
	if err != nil {
		t.Fatalf("IncrementChannelMentionCountIfUnread: %v", err)
	}
	if got != 1 {
		t.Errorf("incrementing an unread channel = %d, want 1", got)
	}
}

func TestUpdateChannelReadState_ResetsMentionCountWhenRead(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true})

	// Prime an unread state with a mention count.
	if err := db.BatchUpdateChannelReadState([]ChannelReadStateUpdate{
		{ChannelID: "C1", LastReadTS: "1.0", HasUnread: true, MentionCount: 5},
	}); err != nil {
		t.Fatalf("BatchUpdate: %v", err)
	}
	state, err := db.GetChannelReadState("C1")
	if err != nil {
		t.Fatalf("GetChannelReadState: %v", err)
	}
	if state.MentionCount != 5 {
		t.Fatalf("setup precondition failed: MentionCount = %d, want 5", state.MentionCount)
	}

	// Mark read: mention_count must reset to 0 alongside has_unread.
	if err := db.UpdateChannelReadState("C1", "2.0", false); err != nil {
		t.Fatalf("mark-read UpdateChannelReadState: %v", err)
	}
	state, err = db.GetChannelReadState("C1")
	if err != nil {
		t.Fatalf("GetChannelReadState after read: %v", err)
	}
	if state.MentionCount != 0 {
		t.Errorf("MentionCount after mark-read = %d, want 0", state.MentionCount)
	}
	if state.HasUnread {
		t.Errorf("HasUnread after mark-read = true, want false")
	}
}

func TestUpdateChannelReadState_StillUnreadPreservesMentionCount(t *testing.T) {
	// A "set has_unread=true" call (e.g. a fresh inbound message that
	// the realtime handler issues right before bumping mention_count)
	// must not clobber the existing count.
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true})

	if err := db.BatchUpdateChannelReadState([]ChannelReadStateUpdate{
		{ChannelID: "C1", LastReadTS: "1.0", HasUnread: true, MentionCount: 4},
	}); err != nil {
		t.Fatalf("BatchUpdate: %v", err)
	}

	// Mirror the realtime handler's call shape: empty TS, hasUnread=true.
	if err := db.UpdateChannelReadState("C1", "", true); err != nil {
		t.Fatalf("UpdateChannelReadState: %v", err)
	}
	if got := db.GetChannelMentionCount("C1"); got != 4 {
		t.Errorf("MentionCount after empty-TS unread set = %d, want 4 (preserved)", got)
	}
}

func TestGetWorkspaceReadState_IncludesLatestAndMention(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()
	db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true})

	if err := db.SetChannelMentionCount("C1", 3); err != nil {
		t.Fatalf("SetChannelMentionCount: %v", err)
	}
	if err := db.SetChannelLatestSyncedTS("C1", "1700000000.000123"); err != nil {
		t.Fatalf("SetChannelLatestSyncedTS: %v", err)
	}
	if err := db.UpdateChannelReadState("C1", "1700000000.000000", true); err != nil {
		t.Fatalf("UpdateChannelReadState: %v", err)
	}

	states, err := db.GetWorkspaceReadState("T1")
	if err != nil {
		t.Fatalf("GetWorkspaceReadState: %v", err)
	}
	got, ok := states["C1"]
	if !ok {
		t.Fatalf("C1 missing from workspace state map")
	}
	if got.LatestTS != "1700000000.000123" {
		t.Errorf("LatestTS = %q, want 1700000000.000123", got.LatestTS)
	}
	if got.MentionCount != 3 {
		t.Errorf("MentionCount = %d, want 3", got.MentionCount)
	}
	if got.LastReadTS != "1700000000.000000" || !got.HasUnread {
		t.Errorf("existing fields = %+v, want LastReadTS=1700000000.000000 HasUnread=true", got)
	}
}

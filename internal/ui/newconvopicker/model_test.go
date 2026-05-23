package newconvopicker

import (
	"strings"
	"testing"
)

func TestEmptyQueryNoAutoHighlight(t *testing.T) {
	m := New()
	m.SetChannels([]Item{{ID: "C1", Name: "general", Type: "channel", Joined: true}})
	m.SetUsers([]Item{{ID: "U1", Name: "doogie min", Username: "doogie"}})
	m.Open()

	if m.selected != -1 {
		t.Fatalf("expected selected=-1 on empty query, got %d", m.selected)
	}
	// Enter on empty query with no chips → no-op.
	if got := m.HandleKey("enter"); got != nil {
		t.Fatalf("expected nil result on empty-query Enter, got %+v", got)
	}
}

func TestUserEnterAddsChipAndClearsQuery(t *testing.T) {
	m := New()
	m.SetUsers([]Item{{ID: "U1", Name: "doogie min", Username: "doogie"}})
	m.Open()
	// Type to highlight the user row.
	m.HandleKey("d")
	m.HandleKey("o")
	if m.selected < 0 {
		t.Fatalf("expected highlight after typing, got selected=%d", m.selected)
	}
	got := m.HandleKey("enter")
	if got != nil {
		t.Fatalf("expected nil (chip added, picker stays open), got %+v", got)
	}
	chips := m.Chips()
	if len(chips) != 1 || chips[0].ID != "U1" {
		t.Fatalf("expected single chip U1, got %+v", chips)
	}
	if m.Query() != "" {
		t.Fatalf("expected query cleared, got %q", m.Query())
	}
	if !m.IsVisible() {
		t.Fatalf("picker should remain open after chipping")
	}
}

func TestChannelEnterReturnsResultWhenNoChips(t *testing.T) {
	m := New()
	m.SetChannels([]Item{{ID: "C1", Name: "slk", Type: "channel", Joined: true}})
	m.Open()
	m.HandleKey("s")
	got := m.HandleKey("enter")
	if got == nil || got.Channel == nil {
		t.Fatalf("expected ChannelResult, got %+v", got)
	}
	if got.Channel.ID != "C1" || got.Channel.Name != "slk" {
		t.Fatalf("wrong channel: %+v", got.Channel)
	}
}

func TestChannelEnterSilentlyIgnoredWithChips(t *testing.T) {
	m := New()
	m.SetChannels([]Item{{ID: "C1", Name: "slk", Type: "channel", Joined: true}})
	m.SetUsers([]Item{{ID: "U1", Name: "doogie", Username: "doogie"}})
	m.Open()
	// Chip user first.
	m.HandleKey("d")
	m.HandleKey("enter")
	if len(m.Chips()) != 1 {
		t.Fatalf("setup: expected one chip, got %d", len(m.Chips()))
	}
	// Now type the channel name and try to commit.
	m.HandleKey("s")
	got := m.HandleKey("enter")
	if got != nil {
		t.Fatalf("expected nil (silent ignore), got %+v", got)
	}
}

func TestEmptyQueryEnterSubmitsChips(t *testing.T) {
	m := New()
	m.SetUsers([]Item{
		{ID: "U1", Name: "doogie min", Username: "doogie"},
		{ID: "U2", Name: "gavgin jeong", Username: "gavgin"},
	})
	m.Open()
	m.HandleKey("d")
	m.HandleKey("enter")
	m.HandleKey("g")
	m.HandleKey("enter")
	if len(m.Chips()) != 2 {
		t.Fatalf("setup: expected two chips, got %d", len(m.Chips()))
	}
	got := m.HandleKey("enter")
	if got == nil || got.Channel != nil {
		t.Fatalf("expected Users result, got %+v", got)
	}
	if len(got.Users) != 2 || got.Users[0].ID != "U1" || got.Users[1].ID != "U2" {
		t.Fatalf("wrong user list: %+v", got.Users)
	}
}

func TestBackspaceRuneAware(t *testing.T) {
	m := New()
	m.SetUsers([]Item{{ID: "U1", Name: "홍길동", Username: "hong"}})
	m.Open()
	m.HandleKey("홍")
	m.HandleKey("길")
	m.HandleKey("동")
	if m.Query() != "홍길동" {
		t.Fatalf("query: got %q", m.Query())
	}
	m.HandleKey("backspace")
	if m.Query() != "홍길" {
		t.Fatalf("after one backspace, expected 홍길, got %q", m.Query())
	}
}

func TestBackspaceEmptyQueryPopsChip(t *testing.T) {
	m := New()
	m.SetUsers([]Item{{ID: "U1", Name: "doogie", Username: "doogie"}})
	m.Open()
	m.HandleKey("d")
	m.HandleKey("enter")
	if len(m.Chips()) != 1 {
		t.Fatalf("setup")
	}
	m.HandleKey("backspace") // query is empty → pop chip
	if len(m.Chips()) != 0 {
		t.Fatalf("expected chip removed, got %d chips", len(m.Chips()))
	}
}

func TestDupChipPrevention(t *testing.T) {
	m := New()
	m.SetUsers([]Item{{ID: "U1", Name: "doogie", Username: "doogie"}})
	m.Open()
	m.HandleKey("d")
	m.HandleKey("enter")
	// Try to add the same user again.
	m.HandleKey("d")
	got := m.HandleKey("enter")
	if got != nil {
		t.Fatalf("expected nil (no-op, no match since chipped), got %+v", got)
	}
	if len(m.Chips()) != 1 {
		t.Fatalf("expected still single chip, got %d", len(m.Chips()))
	}
}

func TestExistingDMUserShortCircuitsAtSubmission(t *testing.T) {
	m := New()
	// Channel pool has a dm row for U1 (should be suppressed in the merge).
	m.SetChannels([]Item{{ID: "D9", Name: "doogie min", Type: "dm"}})
	m.SetUsers([]Item{{ID: "U1", Name: "doogie min", Username: "doogie", DMChannelID: "D9"}})
	m.Open()
	m.HandleKey("d")
	// First Enter chips the user (chip-build path, doesn't short-circuit even
	// though DMChannelID is known — important for mpim building from
	// known-DM users).
	got := m.HandleKey("enter")
	if got != nil {
		t.Fatalf("expected nil (chip added), got %+v", got)
	}
	if len(m.Chips()) != 1 {
		t.Fatalf("expected 1 chip, got %d", len(m.Chips()))
	}
	// Empty-query Enter is the submission gesture; the single-chip
	// short-circuit fires here and returns the cached channel.
	got = m.HandleKey("enter")
	if got == nil || got.Channel == nil {
		t.Fatalf("expected ChannelResult via short-circuit, got %+v", got)
	}
	if got.Channel.ID != "D9" {
		t.Fatalf("expected D9 (existing DM), got %s", got.Channel.ID)
	}
	if got.DMUserID != "U1" {
		t.Fatalf("expected DMUserID=U1 for sidebar patch, got %q", got.DMUserID)
	}
}

func TestExistingDMRowSuppressed(t *testing.T) {
	m := New()
	m.SetChannels([]Item{{ID: "D9", Name: "doogie min", Type: "dm"}})
	m.SetUsers([]Item{{ID: "U1", Name: "doogie min", Username: "doogie", DMChannelID: "D9"}})
	m.Open()
	m.HandleKey("d")
	if len(m.filtered) != 1 {
		t.Fatalf("expected 1 merged row (user only), got %d", len(m.filtered))
	}
	if m.merged[m.filtered[0]].Kind != KindUser {
		t.Fatalf("expected user row, got %+v", m.merged[m.filtered[0]])
	}
}

func TestCtrlEnterSubmitsAnyTime(t *testing.T) {
	m := New()
	m.SetUsers([]Item{{ID: "U1", Name: "doogie", Username: "doogie"}})
	m.Open()
	m.HandleKey("d")
	m.HandleKey("enter") // chip U1
	// Type into query so it's not empty; ctrl+enter still submits.
	m.HandleKey("x")
	got := m.HandleKey("ctrl+enter")
	if got == nil || got.Channel != nil {
		t.Fatalf("expected Users result, got %+v", got)
	}
	if len(got.Users) != 1 {
		t.Fatalf("expected 1 user submitted, got %d", len(got.Users))
	}
}

func TestEscClosesPicker(t *testing.T) {
	m := New()
	m.Open()
	m.HandleKey("esc")
	if m.IsVisible() {
		t.Fatalf("expected hidden after esc")
	}
}

func TestPatchUserName(t *testing.T) {
	m := New()
	m.SetUsers([]Item{{ID: "U1", Name: "U1", Username: ""}})
	m.PatchUserName("U1", "doogie min")
	if m.users[0].Name != "doogie min" {
		t.Fatalf("patch failed: %+v", m.users[0])
	}
}

func TestUpsertChannel(t *testing.T) {
	m := New()
	m.SetChannels([]Item{{ID: "C1", Name: "old", Type: "channel"}})
	m.UpsertChannel(Item{ID: "C1", Name: "new", Type: "channel"})
	if len(m.channels) != 1 || m.channels[0].Name != "new" {
		t.Fatalf("upsert replace failed: %+v", m.channels)
	}
	m.UpsertChannel(Item{ID: "C2", Name: "fresh", Type: "channel"})
	if len(m.channels) != 2 {
		t.Fatalf("upsert append failed: %+v", m.channels)
	}
}

func TestViewOverlayRendersHeaderAndChips(t *testing.T) {
	m := New()
	m.SetChannels([]Item{{ID: "C1", Name: "slk", Type: "channel", Joined: true}})
	m.SetUsers([]Item{{ID: "U1", Name: "Doogie Min", Username: "doogie"}})
	m.Open()
	view := m.ViewOverlay(80, 24, "background")
	if !strings.Contains(view, "New message") {
		t.Fatalf("expected 'New message' header in view, got:\n%s", view)
	}
	// Type to chip user → render should now include the chip.
	m.HandleKey("d")
	m.HandleKey("o")
	m.HandleKey("enter") // chip
	view = m.ViewOverlay(80, 24, "background")
	if !strings.Contains(view, "Doogie Min") {
		t.Fatalf("expected chip 'Doogie Min' in view, got:\n%s", view)
	}
}

package activityview

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

func TestViewEmptyMentionsNoActivity(t *testing.T) {
	m := New(nil, "")
	out := m.View(5, 30)
	if !strings.Contains(strings.ToLower(out), "no activity") {
		t.Fatalf("empty view should mention no activity, got:\n%s", out)
	}
}

func TestClickAndUnreadCount(t *testing.T) {
	m := New(nil, "")
	m.SetItems([]cache.ActivityItem{
		{Kind: "mention", ChannelID: "C1", TS: "1.0", Text: "a", Unread: true},
		{Kind: "dm", ChannelID: "C2", TS: "2.0", Text: "b", Unread: false},
	})
	if m.UnreadCount() != 1 {
		t.Fatalf("UnreadCount = %d, want 1", m.UnreadCount())
	}
	if !m.ClickAt(4) {
		t.Fatal("expected click on second card row to select an item")
	}
	if got := m.SelectedIndex(); got != 1 {
		t.Fatalf("SelectedIndex = %d, want 1", got)
	}
}

func TestViewShowsActorContextAndPreview(t *testing.T) {
	m := New(map[string]string{"U2": "Jane"}, "USELF")
	m.SetItems([]cache.ActivityItem{{
		Kind:        "thread_reply",
		ChannelID:   "C1",
		ChannelName: "proj-feed",
		ChannelType: "channel",
		TS:          "1700000410.000000",
		UserID:      "U2",
		Text:        "thread update body",
		Unread:      true,
	}})
	out := m.View(6, 60)
	for _, want := range []string{"Jane", "Thread", "proj-feed", "thread update body"} {
		if !strings.Contains(out, want) {
			t.Fatalf("view missing %q:\n%s", want, out)
		}
	}
	// Unread renders as a numeric badge (" 1 ") rather than a literal
	// label — see unreadBadgeStyle in renderHeader.
	if !strings.Contains(out, " 1 ") {
		t.Fatalf("view missing unread badge:\n%s", out)
	}
}

func TestContextLabelDMOmitsChannel(t *testing.T) {
	m := New(map[string]string{"U2": "Jane"}, "USELF")
	m.SetItems([]cache.ActivityItem{{
		Kind:        "mention",
		ChannelID:   "D1",
		ChannelName: "Jane",
		ChannelType: "dm",
		TS:          "1700000200.000000",
		UserID:      "U2",
		Text:        "hi there",
	}})
	out := m.View(6, 60)
	if !strings.Contains(out, "Direct message") {
		t.Fatalf("expected DM context label, got:\n%s", out)
	}
	if strings.Contains(out, "Mention in") {
		t.Fatalf("DM context should not include channel ref:\n%s", out)
	}
}

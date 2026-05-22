package sidebar

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

// fakeReader returns the read-state map the sidebar consults when
// sorting and rendering. Keeping the fixture inline makes the
// assertions in each test independent of bootstrap order.
func fakeReader(m map[string]cache.ReadState) func() map[string]cache.ReadState {
	return func() map[string]cache.ReadState { return m }
}

func TestSidebar_DMSort_MostRecentFirst(t *testing.T) {
	// Slack-provided order is alphabetic; the sidebar must lift the DM
	// with the freshest message to the top.
	items := []ChannelItem{
		{ID: "D1", Name: "alice", Type: "dm"},
		{ID: "D2", Name: "bob", Type: "dm"},
		{ID: "D3", Name: "carol", Type: "dm"},
	}
	m := New(items)
	// Direct Messages section is expanded by default.
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"D1": {LatestTS: "1700000001.000000"},
		"D2": {LatestTS: "1700000003.000000"},
		"D3": {LatestTS: "1700000002.000000"},
	}))

	view := m.View(40, 30)
	posBob := strings.Index(view, "bob")
	posCarol := strings.Index(view, "carol")
	posAlice := strings.Index(view, "alice")
	if posBob == -1 || posCarol == -1 || posAlice == -1 {
		t.Fatalf("all DMs should render; view=\n%s", view)
	}
	if !(posBob < posCarol && posCarol < posAlice) {
		t.Errorf("expected DM order bob → carol → alice by recency; got positions bob=%d carol=%d alice=%d",
			posBob, posCarol, posAlice)
	}
}

func TestSidebar_DMSort_EmptyTSStableLast(t *testing.T) {
	// DMs that have no observed messages must stay below all DMs that
	// do, and amongst themselves they keep Slack's original order.
	items := []ChannelItem{
		{ID: "D1", Name: "alice", Type: "dm"},
		{ID: "D2", Name: "bob", Type: "dm"},
		{ID: "D3", Name: "carol", Type: "dm"},
	}
	m := New(items)
	// Direct Messages section is expanded by default; no toggle needed.
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"D1": {LatestTS: ""},
		"D2": {LatestTS: "1700000005.000000"},
		"D3": {LatestTS: ""},
	}))

	view := m.View(40, 30)
	posBob := strings.Index(view, "bob")
	posAlice := strings.Index(view, "alice")
	posCarol := strings.Index(view, "carol")
	if !(posBob < posAlice && posAlice < posCarol) {
		t.Errorf("expected order bob → alice → carol (recency, then original); got positions bob=%d alice=%d carol=%d",
			posBob, posAlice, posCarol)
	}
}

func TestSidebar_GroupDM_SortsByRecency(t *testing.T) {
	// Group DMs (mpim) live in the same Direct Messages section and
	// must obey the same recency rule.
	items := []ChannelItem{
		{ID: "G1", Name: "alice, bob", Type: "group_dm"},
		{ID: "G2", Name: "carol, dave", Type: "group_dm"},
	}
	m := New(items)
	// Direct Messages section is expanded by default.
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"G1": {LatestTS: "1700000001.000000"},
		"G2": {LatestTS: "1700000099.000000"},
	}))

	view := m.View(40, 30)
	if strings.Index(view, "carol") > strings.Index(view, "alice") {
		t.Errorf("expected group_dm carol,dave above alice,bob by recency; view=\n%s", view)
	}
}

func TestSidebar_ChannelMentions_LiftToTopAndRenderBadge(t *testing.T) {
	// Two channels: random has 3 mentions, general has none. random
	// must lift to the top and render a "•3" badge.
	items := []ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
		{ID: "C2", Name: "random", Type: "channel"},
	}
	m := New(items)
	m.ToggleCollapse(defaultChannelsSection) // expand channels
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"C1": {HasUnread: false},
		"C2": {HasUnread: true, MentionCount: 3},
	}))

	view := m.View(40, 30)
	if !strings.Contains(view, "•3") {
		t.Errorf("expected mention badge •3 in view; got\n%s", view)
	}
	if strings.Index(view, "random") > strings.Index(view, "general") {
		t.Errorf("expected random (mention) above general; view=\n%s", view)
	}
}

func TestSidebar_ChannelMentions_CappedAt99(t *testing.T) {
	items := []ChannelItem{{ID: "C1", Name: "ops", Type: "channel"}}
	m := New(items)
	m.ToggleCollapse(defaultChannelsSection)
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"C1": {HasUnread: true, MentionCount: 250},
	}))

	view := m.View(40, 30)
	if !strings.Contains(view, "•99+") {
		t.Errorf("expected capped badge •99+ in view; got\n%s", view)
	}
}

func TestSidebar_MutedChannel_NoMentionBadgeOrDot(t *testing.T) {
	// Muted channels must stay quiet — no dot, no badge — even when
	// the read state has unread + mentions on them.
	items := []ChannelItem{{ID: "C1", Name: "noise", Type: "channel", IsMuted: true}}
	m := New(items)
	m.ToggleCollapse(defaultChannelsSection)
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"C1": {HasUnread: true, MentionCount: 5},
	}))

	view := m.View(40, 30)
	if strings.Contains(view, "•5") || strings.Contains(view, "●") {
		t.Errorf("muted channel should suppress badge AND unread dot; view=\n%s", view)
	}
}

func TestSidebar_DMSort_DoesNotReorderChannels(t *testing.T) {
	// Sanity: changing DM LatestTS values must not perturb the
	// Channels section's existing order. Earlier index wins for ties.
	items := []ChannelItem{
		{ID: "C1", Name: "general", Type: "channel"},
		{ID: "C2", Name: "random", Type: "channel"},
		{ID: "D1", Name: "alice", Type: "dm"},
	}
	m := New(items)
	// Channels section starts collapsed; DM section starts expanded.
	m.ToggleCollapse(defaultChannelsSection)
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"D1": {LatestTS: "1700000099.000000"},
	}))

	view := m.View(40, 30)
	if strings.Index(view, "general") > strings.Index(view, "random") {
		t.Errorf("Channels order must remain general → random; view=\n%s", view)
	}
}

func TestSidebar_MutedChannelWithMentions_DoesNotFloat(t *testing.T) {
	// A muted channel with a stale mention_count must not lift above
	// non-muted siblings. The render gate already hides the badge;
	// the sort gate (effectiveMentionCount) must agree, otherwise the
	// muted row floats to the top with no visible reason.
	items := []ChannelItem{
		{ID: "C1", Name: "ops", Type: "channel"},
		{ID: "C2", Name: "noise", Type: "channel", IsMuted: true},
	}
	m := New(items)
	m.ToggleCollapse(defaultChannelsSection)
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"C1": {HasUnread: true, MentionCount: 0},
		"C2": {HasUnread: true, MentionCount: 5}, // muted: should not lift
	}))

	view := m.View(40, 30)
	posOps := strings.Index(view, "ops")
	posNoise := strings.Index(view, "noise")
	if !(posOps < posNoise) {
		t.Errorf("expected non-muted ops above muted noise; view=\n%s", view)
	}
}

func TestSidebar_StaleMentionOnReadChannel_DoesNotFloat(t *testing.T) {
	// Race-safety net: if mention_count is somehow > 0 while
	// HasUnread is false (e.g., a future code path leaves them out
	// of sync), the channel must NOT float above an unread sibling.
	// effectiveMentionCount treats has_unread=false as "no mention."
	items := []ChannelItem{
		{ID: "C1", Name: "ops", Type: "channel"},
		{ID: "C2", Name: "stale", Type: "channel"},
	}
	m := New(items)
	m.ToggleCollapse(defaultChannelsSection)
	m.SetReadStateReader(fakeReader(map[string]cache.ReadState{
		"C1": {HasUnread: true, MentionCount: 0},
		"C2": {HasUnread: false, MentionCount: 3}, // stale; should be ignored
	}))

	view := m.View(40, 30)
	if strings.Index(view, "ops") > strings.Index(view, "stale") {
		t.Errorf("stale mention_count on read channel must not lift; view=\n%s", view)
	}
}

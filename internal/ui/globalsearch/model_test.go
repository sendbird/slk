package globalsearch

import (
	"strings"
	"testing"
)

func newWithItems(t *testing.T, items []Item) *Model {
	t.Helper()
	mv := New()
	m := &mv
	m.SetItems(items)
	m.Open()
	return m
}

func filteredNames(m *Model) []string {
	out := make([]string, 0, len(m.flat))
	for _, idx := range m.flat {
		out = append(out, m.items[idx].Name)
	}
	return out
}

func sectionNames(m *Model, cat string) []string {
	out := make([]string, 0, len(m.sectionItems[cat]))
	for _, idx := range m.sectionItems[cat] {
		out = append(out, m.items[idx].Name)
	}
	return out
}

func TestEmptyQueryShowsAllSectionedAndPinsSynthetic(t *testing.T) {
	m := newWithItems(t, []Item{
		{ID: "C1", Name: "eng-deploy", Type: "channel", Joined: true},
		{ID: "U1", Name: "Hyojin Park", Type: "dm", Joined: true},
		{ID: "C2", Name: "release-deploy", Type: "channel", Joined: true},
	})
	m.SetSyntheticItems([]Item{
		{ID: ThreadsViewID, Name: "Threads", Type: "threads", Joined: true},
		{ID: ActivityViewID, Name: "Activity", Type: "activity", Joined: true},
	})
	m.Open()

	if got := m.sectionOrder; len(got) == 0 || got[0] != CategorySynthetic {
		t.Fatalf("synthetic must be first section, got %v", got)
	}
	if got := sectionNames(m, CategorySynthetic); len(got) != 2 || got[0] != "Threads" || got[1] != "Activity" {
		t.Fatalf("synthetic section: got %v want [Threads Activity]", got)
	}
	if got := sectionNames(m, CategoryChannel); len(got) != 2 {
		t.Fatalf("channel section size: got %v", got)
	}
	if got := sectionNames(m, CategoryPerson); len(got) != 1 || got[0] != "Hyojin Park" {
		t.Fatalf("person section: got %v", got)
	}
}

func TestQueryFiltersAcrossSections(t *testing.T) {
	m := newWithItems(t, []Item{
		{ID: "C1", Name: "eng-deploy", Type: "channel", Joined: true},
		{ID: "C2", Name: "marketing", Type: "channel", Joined: true},
		{ID: "U1", Name: "Deploy Bot", Type: "dm", Joined: true},
		{ID: "U2", Name: "Hyojin", Type: "dm", Joined: true},
	})

	for _, r := range "deploy" {
		m.HandleKey(string(r))
	}

	names := filteredNames(m)
	if len(names) != 2 {
		t.Fatalf("expected 2 results across sections, got %v", names)
	}
	if got := sectionNames(m, CategoryChannel); len(got) != 1 || got[0] != "eng-deploy" {
		t.Fatalf("channel section: got %v", got)
	}
	if got := sectionNames(m, CategoryPerson); len(got) != 1 || got[0] != "Deploy Bot" {
		t.Fatalf("person section: got %v", got)
	}
}

func TestPrefixBeatsSubstringInsideSection(t *testing.T) {
	m := newWithItems(t, []Item{
		{ID: "C1", Name: "subteam-deploy", Type: "channel", Joined: true},
		{ID: "C2", Name: "deploy", Type: "channel", Joined: true},
		{ID: "C3", Name: "redeploy", Type: "channel", Joined: true},
	})

	for _, r := range "deploy" {
		m.HandleKey(string(r))
	}

	got := sectionNames(m, CategoryChannel)
	if len(got) == 0 || got[0] != "deploy" {
		t.Fatalf("prefix match must come first, got %v", got)
	}
}

func TestJoinedRankedAboveBrowseableInChannelSection(t *testing.T) {
	m := newWithItems(t, []Item{
		{ID: "C1", Name: "deploy-prod", Type: "channel", Joined: false},
		{ID: "C2", Name: "deploy-dev", Type: "channel", Joined: true},
	})

	for _, r := range "deploy" {
		m.HandleKey(string(r))
	}

	got := sectionNames(m, CategoryChannel)
	if len(got) < 2 || got[0] != "deploy-dev" {
		t.Fatalf("joined channel must rank first, got %v", got)
	}
}

func TestSectionResultsBounded(t *testing.T) {
	items := make([]Item, 0, 12)
	for i := 0; i < 12; i++ {
		items = append(items, Item{
			ID:     "C" + string(rune('A'+i)),
			Name:   "deploy-" + string(rune('a'+i)),
			Type:   "channel",
			Joined: true,
		})
	}
	m := newWithItems(t, items)
	for _, r := range "deploy" {
		m.HandleKey(string(r))
	}
	if got := len(sectionNames(m, CategoryChannel)); got != maxPerSection {
		t.Fatalf("channel section must be bounded to %d, got %d", maxPerSection, got)
	}
}

func TestNavigationAndSelection(t *testing.T) {
	m := newWithItems(t, []Item{
		{ID: "C1", Name: "alpha", Type: "channel", Joined: true},
		{ID: "C2", Name: "beta", Type: "channel", Joined: true},
		{ID: "U1", Name: "Carol", Type: "dm", Joined: true},
	})

	if got := m.selected; got != 0 {
		t.Fatalf("initial selected: got %d", got)
	}
	m.HandleKey("down")
	m.HandleKey("down")
	if got := m.selected; got != 2 {
		t.Fatalf("after two downs selected: got %d", got)
	}
	res := m.HandleKey("enter")
	if res == nil || res.ID != "U1" {
		t.Fatalf("enter on 3rd row: got %+v", res)
	}
}

func TestEscapeCloses(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "x", Type: "channel", Joined: true}})
	if !m.IsVisible() {
		t.Fatalf("should be visible after Open")
	}
	m.HandleKey("esc")
	if m.IsVisible() {
		t.Fatalf("esc must hide overlay")
	}
}

func TestBackspaceTrimsQuery(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "foo", Type: "channel", Joined: true}})
	m.HandleKey("f")
	m.HandleKey("o")
	m.HandleKey("o")
	if got := m.query; got != "foo" {
		t.Fatalf("query after typing: got %q", got)
	}
	m.HandleKey("backspace")
	if got := m.query; got != "fo" {
		t.Fatalf("query after backspace: got %q", got)
	}
}

func TestSyntheticPinnedEvenAcrossSetItems(t *testing.T) {
	mv := New()
	m := &mv
	m.SetSyntheticItems([]Item{{ID: ThreadsViewID, Name: "Threads", Type: "threads"}})
	m.SetItems([]Item{{ID: "C1", Name: "general", Type: "channel", Joined: true}})
	m.Open()

	if got := sectionNames(m, CategorySynthetic); len(got) != 1 || got[0] != "Threads" {
		t.Fatalf("synthetic survived SetItems: got %v", got)
	}
	if got := sectionNames(m, CategoryChannel); len(got) != 1 || got[0] != "general" {
		t.Fatalf("channel section after SetItems: got %v", got)
	}
}

func TestSetBrowseablePreservesJoined(t *testing.T) {
	mv := New()
	m := &mv
	m.SetItems([]Item{{ID: "C1", Name: "joined-channel", Type: "channel", Joined: true}})
	m.SetBrowseable([]Item{{ID: "C2", Name: "public-channel", Type: "channel"}})
	m.Open()

	got := sectionNames(m, CategoryChannel)
	if len(got) != 2 {
		t.Fatalf("both joined and browseable visible: got %v", got)
	}
	// joined ranks first under empty query when q is non-empty; under empty
	// query order is LastVisited DESC then name. Joined-Channel has no
	// LastVisited, so name order applies: "joined-channel" < "public-channel"
	// alphabetically.
	if got[0] != "joined-channel" {
		t.Fatalf("joined channel first: got %v", got)
	}
}

func TestMarkJoinedFlipsBit(t *testing.T) {
	mv := New()
	m := &mv
	m.SetBrowseable([]Item{{ID: "C1", Name: "browseable", Type: "channel"}})
	m.MarkJoined("C1")
	m.Open()
	for _, idx := range m.sectionItems[CategoryChannel] {
		if !m.items[idx].Joined {
			t.Fatalf("MarkJoined should set Joined=true on items[%d]", idx)
		}
	}
}

func TestUpdateLastVisitedReordersUnderEmptyQuery(t *testing.T) {
	mv := New()
	m := &mv
	m.SetItems([]Item{
		{ID: "C1", Name: "alpha", Type: "channel", Joined: true},
		{ID: "C2", Name: "beta", Type: "channel", Joined: true},
	})
	m.Open()

	m.UpdateLastVisited("C2", 100)
	got := sectionNames(m, CategoryChannel)
	if len(got) < 2 || got[0] != "beta" {
		t.Fatalf("most recent visit must rank first: got %v", got)
	}
}

func TestRenderBoxShowsSectionHeaders(t *testing.T) {
	m := newWithItems(t, []Item{
		{ID: "C1", Name: "eng-deploy", Type: "channel", Joined: true},
		{ID: "U1", Name: "Hyojin", Type: "dm", Joined: true},
	})
	rendered := m.View(80)
	if !strings.Contains(rendered, "Channels") {
		t.Fatalf("missing Channels header:\n%s", rendered)
	}
	if !strings.Contains(rendered, "People") {
		t.Fatalf("missing People header:\n%s", rendered)
	}
	if !strings.Contains(rendered, "eng-deploy") {
		t.Fatalf("missing channel name:\n%s", rendered)
	}
	if !strings.Contains(rendered, "Hyojin") {
		t.Fatalf("missing person name:\n%s", rendered)
	}
}

func TestRenderEmptyQueryShowsHint(t *testing.T) {
	mv := New()
	m := &mv
	m.Open()
	rendered := m.View(80)
	if !strings.Contains(rendered, "Type to search") {
		t.Fatalf("empty-no-items state must hint:\n%s", rendered)
	}
}

func TestRenderNoResultsLabel(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "alpha", Type: "channel", Joined: true}})
	for _, r := range "zzz" {
		m.HandleKey(string(r))
	}
	rendered := m.View(80)
	if !strings.Contains(rendered, "No results") {
		t.Fatalf("expected No results label:\n%s", rendered)
	}
}

func TestKoreanQueryFiltersByName(t *testing.T) {
	m := newWithItems(t, []Item{
		{ID: "C1", Name: "엔지니어링", Type: "channel", Joined: true},
		{ID: "C2", Name: "디자인", Type: "channel", Joined: true},
	})

	m.HandleKey("엔")
	if got := m.Query(); got != "엔" {
		t.Fatalf("Korean query must be accepted: got %q", got)
	}
	got := sectionNames(m, CategoryChannel)
	if len(got) != 1 || got[0] != "엔지니어링" {
		t.Fatalf("Korean query result: got %v", got)
	}
}

func TestBackspaceTrimsOneRuneNotOneByte(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "엔지", Type: "channel", Joined: true}})
	m.HandleKey("엔")
	m.HandleKey("지")
	if got := m.Query(); got != "엔지" {
		t.Fatalf("setup precondition: got %q", got)
	}
	m.HandleKey("backspace")
	if got := m.Query(); got != "엔" {
		t.Fatalf("backspace must drop one rune, got %q", got)
	}
}

func TestAppTypeRoutesToPeopleSection(t *testing.T) {
	m := newWithItems(t, []Item{
		{ID: "U1", Name: "Hyojin", Type: "dm", Joined: true},
		{ID: "B1", Name: "DeployBot", Type: "app", Joined: true},
	})
	got := sectionNames(m, CategoryPerson)
	if len(got) != 2 {
		t.Fatalf("People section must include app DMs: got %v", got)
	}
}

func TestSetItemsAfterOpenClampsSelected(t *testing.T) {
	items := make([]Item, 0, 20)
	for i := 0; i < 20; i++ {
		items = append(items, Item{
			ID:     "C" + string(rune('A'+i)),
			Name:   "channel-" + string(rune('a'+i)),
			Type:   "channel",
			Joined: true,
		})
	}
	m := newWithItems(t, items)

	// Move selection to the bottom of the flat list (bounded by
	// maxPerSection).
	for i := 0; i < 100; i++ {
		m.HandleKey("down")
	}
	if got := m.selected; got != len(m.flat)-1 {
		t.Fatalf("setup precondition: selected=%d flat=%d", got, len(m.flat))
	}

	// Shrink the list out from under the overlay (workspace switch / browse
	// reload). Without the clamp, Enter would index out of bounds.
	m.SetItems([]Item{{ID: "C1", Name: "only", Type: "channel", Joined: true}})

	res := m.HandleKey("enter")
	if res == nil {
		t.Fatalf("enter must still return a result after shrinking")
	}
	if res.ID != "C1" {
		t.Fatalf("enter result after shrink: got %+v", res)
	}
}

func TestEnterIsSafeWhenFilteredEmpty(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "alpha", Type: "channel", Joined: true}})
	for _, r := range "zzzz" {
		m.HandleKey(string(r))
	}
	if res := m.HandleKey("enter"); res != nil {
		t.Fatalf("enter with no matches must return nil, got %+v", res)
	}
}

func TestControlKeyStringsDoNotPolluteQuery(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "alpha", Type: "channel", Joined: true}})
	// Some terminals report unrecognised keys with multi-byte string
	// representations. The query must not absorb them.
	m.HandleKey("ctrl+l")
	m.HandleKey("alt+x")
	if got := m.Query(); got != "" {
		t.Fatalf("control key strings leaked into query: got %q", got)
	}
}

func TestSetMessageResultsAddsSection(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "deploy", Type: "channel", Joined: true}})
	m.HandleKey("d")
	m.HandleKey("e")
	m.HandleKey("p")

	m.SetMessageResults(m.Query(), []Item{
		{ID: "T1", Name: "jinku — deploy is failing", ChannelID: "C1", MessageTS: "1.0"},
		{ID: "T2", Name: "hyojin — deploying tonight", ChannelID: "C1", MessageTS: "2.0"},
	})

	got := sectionNames(m, CategoryMessage)
	if len(got) != 2 {
		t.Fatalf("expected 2 message hits, got %v", got)
	}
	// Order from the server must be preserved — no client-side
	// fuzzy re-ranking on remote rows.
	if got[0] != "jinku — deploy is failing" {
		t.Fatalf("server order must be preserved: got %v", got)
	}
}

func TestSetMessageResultsDroppedWhenQueryChanged(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "deploy", Type: "channel", Joined: true}})
	m.HandleKey("d")
	staleQuery := m.Query()
	m.HandleKey("e")
	m.HandleKey("p")

	m.SetMessageResults(staleQuery, []Item{
		{ID: "T1", Name: "stale-result", ChannelID: "C1"},
	})
	if got := sectionNames(m, CategoryMessage); len(got) != 0 {
		t.Fatalf("stale remote results must be dropped: got %v", got)
	}
}

func TestQueryChangeClearsRemoteResults(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "deploy", Type: "channel", Joined: true}})
	m.HandleKey("d")
	m.SetMessageResults(m.Query(), []Item{
		{ID: "T1", Name: "first-query-hit", ChannelID: "C1"},
	})
	if got := sectionNames(m, CategoryMessage); len(got) != 1 {
		t.Fatalf("setup precondition: got %v", got)
	}
	// Next keystroke must flush stale remote rows immediately so
	// they don't briefly render alongside the new query while
	// awaiting the next round-trip.
	m.HandleKey("e")
	if got := sectionNames(m, CategoryMessage); len(got) != 0 {
		t.Fatalf("remote results must clear on query change: got %v", got)
	}
}

func TestOpenClearsRemoteResults(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "deploy", Type: "channel", Joined: true}})
	m.HandleKey("d")
	m.SetMessageResults(m.Query(), []Item{{ID: "T1", Name: "hit", ChannelID: "C1"}})
	m.Close()
	m.Open()
	if got := sectionNames(m, CategoryMessage); len(got) != 0 {
		t.Fatalf("Open must clear remote results: got %v", got)
	}
}

func TestSetFileResultsAddsSection(t *testing.T) {
	m := newWithItems(t, []Item{{ID: "C1", Name: "deploy", Type: "channel", Joined: true}})
	m.HandleKey("d")
	m.SetFileResults(m.Query(), []Item{
		{ID: "F1", Name: "deploy.png", Subtitle: "image/png", Permalink: "https://x"},
	})
	got := sectionNames(m, CategoryFile)
	if len(got) != 1 || got[0] != "deploy.png" {
		t.Fatalf("file section: got %v", got)
	}
}

func TestEnterOnMessageResultCarriesChannelMetadata(t *testing.T) {
	m := newWithItems(t, []Item{})
	m.HandleKey("d")
	m.SetMessageResults(m.Query(), []Item{
		{ID: "T1", Name: "jinku — deploy", ChannelID: "C42", ChannelName: "eng-deploy", MessageTS: "9.99", Permalink: "https://"},
	})
	// First selectable row should be the message hit.
	res := m.HandleKey("enter")
	if res == nil {
		t.Fatalf("enter on message hit returned nil")
	}
	if res.ChannelID != "C42" || res.MessageTS != "9.99" {
		t.Fatalf("Result carries channel/ts: got %+v", res)
	}
	if res.Type != "message" {
		t.Fatalf("Result.Type: got %q want \"message\"", res.Type)
	}
}

func TestScopeLabelRenderedSeparateFromQuery(t *testing.T) {
	mv := New()
	m := &mv
	m.Configure("Search", "Search messages in this conversation…", "in #eng-deploy")
	m.Open()

	rendered := m.View(80)
	if !strings.Contains(rendered, "in #eng-deploy") {
		t.Fatalf("scope label must be rendered in the overlay:\n%s", rendered)
	}
	if strings.Contains(rendered, "in:eng-deploy") {
		t.Fatalf("wire syntax must stay hidden from the UI:\n%s", rendered)
	}
}

func TestCursorAvailableForKoreanFocusedInput(t *testing.T) {
	mv := New()
	m := &mv
	m.Open()
	m.HandleKey("한")

	if c := m.Cursor(80, 24); c == nil {
		t.Fatal("focused search overlay must expose a real terminal cursor for IME composition")
	}
}

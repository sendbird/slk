package main

import (
	"context"
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/service"
	slk "github.com/gammons/slk/internal/slack"
	"github.com/gammons/slk/internal/ui/channelfinder"
	"github.com/gammons/slk/internal/ui/newconvopicker"
	"github.com/gammons/slk/internal/ui/sidebar"
	"github.com/slack-go/slack"
)

func TestBuildChannelItem_DM(t *testing.T) {
	wctx := &WorkspaceContext{
		BotUserIDs:        map[string]bool{},
		UserNames:         map[string]string{"U123": "alice"},
		UserNamesByHandle: map[string]string{"alice": "alice"},
	}
	cfg := config.Config{}
	ch := slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{
				ID:   "D1",
				IsIM: true,
				User: "U123",
			},
		},
	}
	item, _ := buildChannelItem(ch, wctx, cfg, "T1")
	if item.ID != "D1" {
		t.Errorf("ID = %q, want D1", item.ID)
	}
	if item.Type != "dm" {
		t.Errorf("Type = %q, want dm", item.Type)
	}
	if item.Name != "alice" {
		t.Errorf("Name = %q, want alice", item.Name)
	}
	if item.DMUserID != "U123" {
		t.Errorf("DMUserID = %q, want U123", item.DMUserID)
	}
}

func TestBuildChannelItem_GroupDM(t *testing.T) {
	wctx := &WorkspaceContext{
		BotUserIDs:        map[string]bool{},
		UserNames:         map[string]string{},
		UserNamesByHandle: map[string]string{"alice": "Alice", "bob": "Bob"},
	}
	cfg := config.Config{}
	ch := slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{
				ID:     "G1",
				IsMpIM: true,
			},
			Name: "mpdm-alice--bob-1",
		},
	}
	item, _ := buildChannelItem(ch, wctx, cfg, "T1")
	if item.Type != "group_dm" {
		t.Errorf("Type = %q, want group_dm", item.Type)
	}
	if item.Name != "Alice, Bob" {
		t.Errorf("Name = %q, want %q", item.Name, "Alice, Bob")
	}
}

func TestBuildChannelItem_Channel(t *testing.T) {
	wctx := &WorkspaceContext{
		BotUserIDs:        map[string]bool{},
		UserNames:         map[string]string{},
		UserNamesByHandle: map[string]string{},
	}
	cfg := config.Config{}
	ch := slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{ID: "C1"},
			Name:         "general",
		},
	}
	item, _ := buildChannelItem(ch, wctx, cfg, "T1")
	if item.Type != "channel" {
		t.Errorf("Type = %q, want channel", item.Type)
	}
	if item.Name != "general" {
		t.Errorf("Name = %q, want general", item.Name)
	}
}

// fakeSectionsClient implements service.SectionsClient for tests; it
// returns a fixed slice of sections so we can construct a real
// *service.SectionStore (Bootstrap-driven) from a known mapping.
type fakeSectionsClient struct {
	sections []slk.SidebarSection
}

func (f *fakeSectionsClient) GetChannelSections(_ context.Context) ([]slk.SidebarSection, error) {
	return f.sections, nil
}

// bootstrappedStore returns a Ready() *service.SectionStore whose
// channelToSection map is built from the supplied (sectionID -> []channelID)
// pairs. All synthetic sections use Type="channels" so they pass
// includeInSidebar's filter (not that it matters for SectionForChannel).
func bootstrappedStore(t *testing.T, mapping map[string][]string) *service.SectionStore {
	t.Helper()
	secs := make([]slk.SidebarSection, 0, len(mapping))
	for id, chans := range mapping {
		secs = append(secs, slk.SidebarSection{
			ID:         id,
			Name:       id,
			Type:       "channels",
			ChannelIDs: chans,
		})
	}
	store := service.NewSectionStore()
	if err := store.Bootstrap(context.Background(), &fakeSectionsClient{sections: secs}); err != nil {
		t.Fatalf("bootstrap: %v", err)
	}
	return store
}

func TestBuildChannelItem_StoreReady_StoreWins(t *testing.T) {
	cfg := config.Config{
		Sections: map[string]config.SectionDef{
			"Globbed": {Channels: []string{"alerts*"}, Order: 1},
		},
	}
	wctx := &WorkspaceContext{
		SectionStore:      bootstrappedStore(t, map[string][]string{"L_SLACK": {"C1"}}),
		UserNames:         map[string]string{},
		UserNamesByHandle: map[string]string{},
		BotUserIDs:        map[string]bool{},
	}
	ch := slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{ID: "C1", NameNormalized: "alerts-prod"},
			Name:         "alerts-prod",
		},
	}
	item, _ := buildChannelItem(ch, wctx, cfg, "T1")
	if item.Section != "L_SLACK" {
		t.Errorf("Section = %q, want L_SLACK (store wins over glob)", item.Section)
	}
}

func TestBuildChannelItem_StoreReady_StoreMisses_FallsToGlob(t *testing.T) {
	cfg := config.Config{
		Sections: map[string]config.SectionDef{
			"Globbed": {Channels: []string{"alerts*"}, Order: 1},
		},
	}
	// Bootstrap a Ready store with no entry for C1; the resolver should
	// fall through to config-glob matching.
	wctx := &WorkspaceContext{
		SectionStore:      bootstrappedStore(t, map[string][]string{}),
		UserNames:         map[string]string{},
		UserNamesByHandle: map[string]string{},
		BotUserIDs:        map[string]bool{},
	}
	ch := slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{ID: "C1"},
			Name:         "alerts-prod",
		},
	}
	item, _ := buildChannelItem(ch, wctx, cfg, "T1")
	if item.Section != "Globbed" {
		t.Errorf("Section = %q, want Globbed (store had no match)", item.Section)
	}
}

func TestBuildChannelItem_StoreNil_UsesGlob(t *testing.T) {
	cfg := config.Config{
		Sections: map[string]config.SectionDef{
			"Globbed": {Channels: []string{"alerts*"}, Order: 1},
		},
	}
	wctx := &WorkspaceContext{
		SectionStore:      nil,
		UserNames:         map[string]string{},
		UserNamesByHandle: map[string]string{},
		BotUserIDs:        map[string]bool{},
	}
	ch := slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{ID: "C1"},
			Name:         "alerts-prod",
		},
	}
	item, _ := buildChannelItem(ch, wctx, cfg, "T1")
	if item.Section != "Globbed" {
		t.Errorf("Section = %q, want Globbed", item.Section)
	}
}

func TestBuildChannelItem_StoreNotReady_UsesGlob(t *testing.T) {
	cfg := config.Config{
		Sections: map[string]config.SectionDef{
			"Globbed": {Channels: []string{"alerts*"}, Order: 1},
		},
	}
	// Fresh store (never bootstrapped) reports Ready()==false; the
	// resolver must skip it even though we'd otherwise expect a match.
	wctx := &WorkspaceContext{
		SectionStore:      service.NewSectionStore(),
		UserNames:         map[string]string{},
		UserNamesByHandle: map[string]string{},
		BotUserIDs:        map[string]bool{},
	}
	ch := slack.Channel{
		GroupConversation: slack.GroupConversation{
			Conversation: slack.Conversation{ID: "C1"},
			Name:         "alerts-prod",
		},
	}
	item, _ := buildChannelItem(ch, wctx, cfg, "T1")
	if item.Section != "Globbed" {
		t.Errorf("Section = %q, want Globbed (store not ready, even though it has a mapping)", item.Section)
	}
}

func TestBuildPickerUsers_ExcludesSelfAndBots(t *testing.T) {
	users := []cache.User{
		{ID: "U_ME", DisplayName: "me"},
		{ID: "U_BOT", DisplayName: "the bot", IsBot: true},
		{ID: "U_DOOGIE", DisplayName: "doogie min", Name: "doogie"},
		{ID: "U_GARV", DisplayName: "", Name: "gavgin"}, // fallback to Name
		{ID: "U_NOID", DisplayName: "", Name: ""},       // fallback to ID
	}
	got := buildPickerUsers(users, "U_ME")
	if len(got) != 3 {
		t.Fatalf("expected 3 items (self + bot excluded), got %d: %+v", len(got), got)
	}
	byID := map[string]newconvopicker.Item{}
	for _, it := range got {
		byID[it.ID] = it
	}
	if _, exists := byID["U_ME"]; exists {
		t.Errorf("self user must be excluded")
	}
	if _, exists := byID["U_BOT"]; exists {
		t.Errorf("bot user must be excluded")
	}
	if byID["U_DOOGIE"].Name != "doogie min" {
		t.Errorf("DisplayName should win for doogie, got %q", byID["U_DOOGIE"].Name)
	}
	if byID["U_GARV"].Name != "gavgin" {
		t.Errorf("Name should be fallback when DisplayName is empty, got %q", byID["U_GARV"].Name)
	}
	if byID["U_NOID"].Name != "U_NOID" {
		t.Errorf("ID should be final fallback, got %q", byID["U_NOID"].Name)
	}
}

func TestApplyDMChannelIDs_MapsUserIDs(t *testing.T) {
	items := []newconvopicker.Item{
		{ID: "U1", Kind: newconvopicker.KindUser, Name: "alice"},
		{ID: "U2", Kind: newconvopicker.KindUser, Name: "bob"},
		{ID: "C1", Kind: newconvopicker.KindChannel, Name: "general"},
	}
	out := applyDMChannelIDs(items, map[string]string{"U1": "D1"})
	if out[0].DMChannelID != "D1" {
		t.Errorf("U1 should have DMChannelID=D1, got %q", out[0].DMChannelID)
	}
	if out[1].DMChannelID != "" {
		t.Errorf("U2 should remain unset, got %q", out[1].DMChannelID)
	}
	if out[2].DMChannelID != "" {
		t.Errorf("channel rows must not get DMChannelID")
	}
}

func TestDMChannelByUser_OnlyDMRows(t *testing.T) {
	items := []sidebar.ChannelItem{
		{ID: "C1", Type: "channel"},
		{ID: "D1", Type: "dm", DMUserID: "U1"},
		{ID: "D2", Type: "dm", DMUserID: ""}, // unbound, skip
		{ID: "G1", Type: "group_dm"},
	}
	got := dmChannelByUser(items)
	if len(got) != 1 {
		t.Fatalf("expected 1 mapping, got %d: %+v", len(got), got)
	}
	if got["U1"] != "D1" {
		t.Errorf("expected U1=>D1, got %+v", got)
	}
}

func TestUpsertWctxChannel_AppendsAndReplaces(t *testing.T) {
	wctx := &WorkspaceContext{}
	si1 := sidebar.ChannelItem{ID: "D9", Name: "doogie", Type: "dm", DMUserID: "U1"}
	fi1 := channelfinder.Item{ID: "D9", Name: "doogie", Type: "dm", Joined: true}
	upsertWctxChannel(wctx, si1, fi1, []string{"U1"})
	if len(wctx.Channels) != 1 || wctx.Channels[0].ID != "D9" {
		t.Fatalf("expected Channels to contain D9, got %+v", wctx.Channels)
	}
	if len(wctx.FinderItems) != 1 || wctx.FinderItems[0].ID != "D9" {
		t.Fatalf("expected FinderItems to contain D9, got %+v", wctx.FinderItems)
	}

	// Second call with the same ID should replace, not append.
	si2 := sidebar.ChannelItem{ID: "D9", Name: "doogie min", Type: "dm", DMUserID: "U1"}
	fi2 := channelfinder.Item{ID: "D9", Name: "doogie min", Type: "dm", Joined: true}
	upsertWctxChannel(wctx, si2, fi2, []string{"U1"})
	if len(wctx.Channels) != 1 {
		t.Fatalf("expected Channels still length 1 after replace, got %d", len(wctx.Channels))
	}
	if wctx.Channels[0].Name != "doogie min" {
		t.Errorf("expected updated name, got %q", wctx.Channels[0].Name)
	}
}

func TestUpsertWctxChannel_PatchesPickerUserDMChannelID(t *testing.T) {
	wctx := &WorkspaceContext{
		PickerUsers: []newconvopicker.Item{
			{ID: "U1", Kind: newconvopicker.KindUser, Name: "doogie"},
			{ID: "U2", Kind: newconvopicker.KindUser, Name: "other"},
		},
	}
	si := sidebar.ChannelItem{ID: "D9", Name: "doogie", Type: "dm", DMUserID: "U1"}
	fi := channelfinder.Item{ID: "D9", Name: "doogie", Type: "dm", Joined: true}
	upsertWctxChannel(wctx, si, fi, []string{"U1"})

	if wctx.PickerUsers[0].DMChannelID != "D9" {
		t.Fatalf("expected U1.DMChannelID=D9, got %q", wctx.PickerUsers[0].DMChannelID)
	}
	if wctx.PickerUsers[1].DMChannelID != "" {
		t.Errorf("U2 should remain untouched, got %q", wctx.PickerUsers[1].DMChannelID)
	}
}

func TestUpsertWctxChannel_GroupDMSkipsPickerPatch(t *testing.T) {
	wctx := &WorkspaceContext{
		PickerUsers: []newconvopicker.Item{
			{ID: "U1", Kind: newconvopicker.KindUser, Name: "doogie"},
		},
	}
	si := sidebar.ChannelItem{ID: "G7", Name: "doogie, gavgin", Type: "group_dm"}
	fi := channelfinder.Item{ID: "G7", Name: "doogie, gavgin", Type: "group_dm", Joined: true}
	upsertWctxChannel(wctx, si, fi, []string{"U1", "U2"})

	if wctx.PickerUsers[0].DMChannelID != "" {
		t.Errorf("group_dm must not patch user.DMChannelID (1:1 only); got %q", wctx.PickerUsers[0].DMChannelID)
	}
}

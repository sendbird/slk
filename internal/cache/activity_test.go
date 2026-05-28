package cache

import "testing"

func TestListActivityItems_MentionThreadUnreadAndPriority(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	const selfID = "USELF"
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	must(db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true}))
	must(db.UpsertChannel(Channel{ID: "C2", WorkspaceID: "T1", Name: "design", Type: "channel", IsMember: true}))
	// C3 is a DM so its latest top-level message is eligible for the
	// Activity feed even after the conversation is read.
	must(db.UpsertChannel(Channel{ID: "C3", WorkspaceID: "T1", Name: "Direct: Pat", Type: "dm", IsMember: true}))

	must(db.UpdateChannelReadState("C1", "1700000000.000000", true))
	must(db.UpdateChannelReadState("C2", "1700000100.000000", false))
	must(db.UpdateChannelReadState("C3", "1700000200.000000", true))

	// Mention should still outrank the other categories, with DMs above
	// threads once the mention row is accounted for.
	must(db.UpsertMessage(Message{TS: "1700000300.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "hey <@USELF>", ThreadTS: ""}))

	// Self-authored mention should be excluded.
	must(db.UpsertMessage(Message{TS: "1700000310.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: selfID, Text: "I mentioned <@USELF>", ThreadTS: ""}))

	// Thread reply.
	must(db.UpsertMessage(Message{TS: "1700000400.000000", ChannelID: "C2", WorkspaceID: "T1", UserID: "U3", Text: "parent", ThreadTS: "1700000390.000000"}))
	must(db.UpsertMessage(Message{TS: "1700000410.000000", ChannelID: "C2", WorkspaceID: "T1", UserID: "U4", Text: "reply in thread", ThreadTS: "1700000390.000000"}))
	must(db.UpsertThreadSubscription("T1", "C2", "1700000390.000000", "1700000405.000000", true))

	// Unread top-level message.
	must(db.UpsertMessage(Message{TS: "1700000500.000000", ChannelID: "C3", WorkspaceID: "T1", UserID: "U5", Text: "fresh unread", ThreadTS: ""}))

	items, err := db.ListActivityItems("T1", selfID, 20)
	if err != nil {
		t.Fatalf("ListActivityItems: %v", err)
	}

	if len(items) != 3 {
		t.Fatalf("want 3 activity items, got %d: %+v", len(items), items)
	}

	if items[0].Kind != "mention" || items[0].TS != "1700000300.000000" {
		t.Fatalf("first item = %+v, want mention at 1700000300.000000", items[0])
	}
	if items[1].Kind != "dm" || items[1].TS != "1700000500.000000" {
		t.Fatalf("second item = %+v, want DM at 1700000500.000000", items[1])
	}
	if items[2].Kind != "thread_reply" || items[2].TS != "1700000410.000000" {
		t.Fatalf("third item = %+v, want thread_reply at 1700000410.000000", items[2])
	}
}

func TestListActivityItems_DedupPrefersMention(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}
	must(db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: "channel", IsMember: true}))
	must(db.UpdateChannelReadState("C1", "1700000000.000000", true))
	must(db.UpsertMessage(Message{TS: "1700000600.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "ping <@USELF>", ThreadTS: ""}))

	items, err := db.ListActivityItems("T1", "USELF", 20)
	if err != nil {
		t.Fatalf("ListActivityItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 deduped item, got %d: %+v", len(items), items)
	}
	if items[0].Kind != "mention" {
		t.Fatalf("kind = %q, want mention", items[0].Kind)
	}
}

func TestListActivityItems_ThreadGroupedByLatestReply(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	const selfID = "USELF"
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	must(db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "covista", Type: "channel", IsMember: true}))
	must(db.UpdateChannelReadState("C1", "1700000000.000000", false))
	must(db.UpsertMessage(Message{TS: "1700000100.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U1", Text: "parent", ThreadTS: "1700000100.000000"}))
	must(db.UpsertMessage(Message{TS: "1700000200.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "first reply", ThreadTS: "1700000100.000000"}))
	must(db.UpsertMessage(Message{TS: "1700000300.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U3", Text: "latest reply", ThreadTS: "1700000100.000000"}))
	must(db.UpsertThreadSubscription("T1", "C1", "1700000100.000000", "1700000150.000000", true))

	items, err := db.ListActivityItems("T1", selfID, 20)
	if err != nil {
		t.Fatalf("ListActivityItems: %v", err)
	}
	// Both replies are unread+subscribed but Slack collapses them
	// into a single Activity row keyed on the latest reply time.
	if len(items) != 1 {
		t.Fatalf("want 1 grouped thread item, got %d: %+v", len(items), items)
	}
	if items[0].Kind != "thread_reply" || items[0].TS != "1700000300.000000" {
		t.Fatalf("grouped item = %+v, want thread_reply at 1700000300.000000", items[0])
	}
}

func TestListActivityItems_ReadDMStillAppears(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	const selfID = "USELF"
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	must(db.UpsertChannel(Channel{ID: "D1", WorkspaceID: "T1", Name: "Taeyeon", Type: "dm", IsMember: true}))
	must(db.UpdateChannelReadState("D1", "1700000200.000000", false))
	must(db.UpsertMessage(Message{TS: "1700000100.000000", ChannelID: "D1", WorkspaceID: "T1", UserID: "U2", Text: "older note", ThreadTS: ""}))
	must(db.UpsertMessage(Message{TS: "1700000300.000000", ChannelID: "D1", WorkspaceID: "T1", UserID: selfID, Text: "my reply", ThreadTS: ""}))

	items, err := db.ListActivityItems("T1", selfID, 20)
	if err != nil {
		t.Fatalf("ListActivityItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 DM item, got %d: %+v", len(items), items)
	}
	if items[0].Kind != "dm" || items[0].TS != "1700000100.000000" || items[0].Unread {
		t.Fatalf("DM item = %+v, want read row at peer message ts", items[0])
	}
}

func TestListActivityItems_ReadActiveThreadStillAppears(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	const selfID = "USELF"
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	must(db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "covista", Type: "channel", IsMember: true}))
	must(db.UpdateChannelReadState("C1", "1700001000.000000", false))
	must(db.UpsertMessage(Message{TS: "1700000100.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U1", Text: "parent", ThreadTS: "1700000100.000000"}))
	must(db.UpsertMessage(Message{TS: "1700000200.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "U2", Text: "reply", ThreadTS: "1700000100.000000"}))
	must(db.UpsertThreadSubscription("T1", "C1", "1700000100.000000", "1700000200.000000", true))

	items, err := db.ListActivityItems("T1", selfID, 20)
	if err != nil {
		t.Fatalf("ListActivityItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 active thread row, got %d: %+v", len(items), items)
	}
	if items[0].Kind != "thread_reply" || items[0].TS != "1700000200.000000" || items[0].Unread {
		t.Fatalf("thread item = %+v, want read active thread at latest reply ts", items[0])
	}
}

func TestListActivityItems_ChannelFirehoseExcluded(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	const selfID = "USELF"
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	// Regular channel the user is a member of, with a noisy bot
	// posting unread messages — Slack does NOT surface these in
	// Activity unless the user is @-mentioned. We mirror that.
	must(db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "calls-notifier", Type: "channel", IsMember: true}))
	must(db.UpdateChannelReadState("C1", "1700000000.000000", true))
	must(db.UpsertMessage(Message{TS: "1700000100.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "UBOT", Text: "incoming call", ThreadTS: ""}))
	must(db.UpsertMessage(Message{TS: "1700000200.000000", ChannelID: "C1", WorkspaceID: "T1", UserID: "UBOT", Text: "another call", ThreadTS: ""}))

	// DM with unread messages should still appear.
	must(db.UpsertChannel(Channel{ID: "D1", WorkspaceID: "T1", Name: "Pat", Type: "dm", IsMember: true}))
	must(db.UpdateChannelReadState("D1", "1700000000.000000", true))
	must(db.UpsertMessage(Message{TS: "1700000400.000000", ChannelID: "D1", WorkspaceID: "T1", UserID: "U2", Text: "hi", ThreadTS: ""}))

	items, err := db.ListActivityItems("T1", selfID, 20)
	if err != nil {
		t.Fatalf("ListActivityItems: %v", err)
	}
	for _, it := range items {
		if it.ChannelID == "C1" {
			t.Fatalf("regular channel firehose should not appear in Activity: %+v", it)
		}
	}
	if len(items) != 1 || items[0].ChannelID != "D1" {
		t.Fatalf("want only the DM activity, got %d items: %+v", len(items), items)
	}
}

func TestListActivityItems_NewerPrivateNotificationDoesNotBuryDMOrThread(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	const selfID = "USELF"
	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	must(db.UpsertChannel(Channel{ID: "P1", WorkspaceID: "T1", Name: "alerts", Type: "private", IsMember: true}))
	must(db.UpsertChannel(Channel{ID: "D1", WorkspaceID: "T1", Name: "Taeyeon", Type: "dm", IsMember: true}))
	must(db.UpsertChannel(Channel{ID: "C1", WorkspaceID: "T1", Name: "covista", Type: "channel", IsMember: true}))

	must(db.UpdateChannelReadState("P1", "1700000000.000000", true))
	must(db.UpdateChannelReadState("D1", "1700000000.000000", true))
	must(db.UpdateChannelReadState("C1", "1700000000.000000", false))

	must(db.UpsertMessage(Message{
		TS:          "1700000900.000000",
		ChannelID:   "P1",
		WorkspaceID: "T1",
		UserID:      "UBOT",
		Text:        "PagerDuty noise",
		ThreadTS:    "",
		Subtype:     "bot_message",
	}))
	must(db.UpsertMessage(Message{
		TS:          "1700000800.000000",
		ChannelID:   "D1",
		WorkspaceID: "T1",
		UserID:      "U2",
		Text:        "taeyeon ping",
		ThreadTS:    "",
	}))
	must(db.UpsertMessage(Message{
		TS:          "1700000700.000000",
		ChannelID:   "C1",
		WorkspaceID: "T1",
		UserID:      "U3",
		Text:        "parent",
		ThreadTS:    "1700000600.000000",
	}))
	must(db.UpsertMessage(Message{
		TS:          "1700000600.000000",
		ChannelID:   "C1",
		WorkspaceID: "T1",
		UserID:      "U4",
		Text:        "thread parent",
		ThreadTS:    "1700000600.000000",
	}))
	must(db.UpsertThreadSubscription("T1", "C1", "1700000600.000000", "1700000650.000000", true))

	items, err := db.ListActivityItems("T1", selfID, 20)
	if err != nil {
		t.Fatalf("ListActivityItems: %v", err)
	}
	if len(items) != 3 {
		t.Fatalf("want 3 items, got %d: %+v", len(items), items)
	}
	if items[0].Kind != "dm" || items[0].ChannelID != "D1" {
		t.Fatalf("first item = %+v, want DM before notification", items[0])
	}
	if items[1].Kind != "thread_reply" || items[1].ChannelID != "C1" {
		t.Fatalf("second item = %+v, want thread before notification", items[1])
	}
	if items[2].Kind != "notification" || items[2].ChannelID != "P1" {
		t.Fatalf("third item = %+v, want notification last", items[2])
	}
}

func TestListActivityItems_PrivateUserGroupMentionTreatedAsMention(t *testing.T) {
	db := setupDBWithWorkspace(t)
	defer db.Close()

	must := func(err error) {
		if err != nil {
			t.Fatal(err)
		}
	}

	must(db.UpsertChannel(Channel{ID: "P1", WorkspaceID: "T1", Name: "ops", Type: "private", IsMember: true}))
	must(db.UpdateChannelReadState("P1", "1700000000.000000", true))
	must(db.UpsertMessage(Message{
		TS:          "1700000100.000000",
		ChannelID:   "P1",
		WorkspaceID: "T1",
		UserID:      "U2",
		Text:        "<!subteam^S123|@oncalls> heads up",
		ThreadTS:    "",
	}))

	items, err := db.ListActivityItems("T1", "USELF", 20)
	if err != nil {
		t.Fatalf("ListActivityItems: %v", err)
	}
	if len(items) != 1 {
		t.Fatalf("want 1 item, got %d: %+v", len(items), items)
	}
	if items[0].Kind != "mention" {
		t.Fatalf("item = %+v, want mention", items[0])
	}
}

package main

import (
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/slack-go/slack"
)

// Tests for the OnMessage mention_count bump path. The bump is gated
// by multiple conditions (edited, self-sender, channel type, has_unread)
// and each gate has a concrete failure mode if missing — see the
// commit message on 604b94d and the codex review that motivated the
// edited/self-user guards.

func mentionHandler(t *testing.T, channelType, selfID string) (*rtmEventHandler, *cache.DB) {
	t.Helper()
	db := newTestDB(t)
	_ = db.UpsertChannel(cache.Channel{ID: "C1", WorkspaceID: "T1", Name: "general", Type: channelType})
	return &rtmEventHandler{
		db:              db,
		wsCtx:           &WorkspaceContext{},
		isActive:        func() bool { return true },
		activeChannelID: func() string { return "C2" }, // inactive => mark unread
		channelTypes:    map[string]string{"C1": channelType},
		currentUserID:   selfID,
		workspaceID:     "T1",
	}, db
}

func TestOnMessage_MentionBumpsCount(t *testing.T) {
	h, db := mentionHandler(t, "channel", "USELF")
	h.OnMessage("C1", "U1", "1.001", "<@USELF> ping", "", "", false, nil, slack.Blocks{}, nil)

	if got := db.GetChannelMentionCount("C1"); got != 1 {
		t.Errorf("mention_count = %d, want 1", got)
	}
}

func TestOnMessage_EditedDoesNotBumpMention(t *testing.T) {
	// An edited message must not retrigger the mention bump — that
	// double-counts a single unread mention and can synthesize a
	// brand-new badge on a message that was already read.
	h, db := mentionHandler(t, "channel", "USELF")
	h.OnMessage("C1", "U1", "1.001", "<@USELF> ping", "", "", true /*edited*/, nil, slack.Blocks{}, nil)

	if got := db.GetChannelMentionCount("C1"); got != 0 {
		t.Errorf("edited mention bumped mention_count = %d, want 0", got)
	}
}

func TestOnMessage_SelfAuthor_DoesNotBumpMention(t *testing.T) {
	// Slack never counts your own message as a mention of yourself,
	// even when the text contains your @-handle (e.g., posted from
	// the official client and echoed via WS).
	h, db := mentionHandler(t, "channel", "USELF")
	h.OnMessage("C1", "USELF", "1.001", "<@USELF> heads up to me", "", "", false, nil, slack.Blocks{}, nil)

	if got := db.GetChannelMentionCount("C1"); got != 0 {
		t.Errorf("self-authored mention bumped mention_count = %d, want 0", got)
	}
}

func TestOnMessage_DMMention_DoesNotBumpMention(t *testing.T) {
	// 1:1 DMs are excluded because client.counts.Ims carries no
	// mention_count on the wire — incrementing here would diverge
	// from Slack's authoritative value on the next bootstrap.
	h, db := mentionHandler(t, "dm", "USELF")
	h.OnMessage("C1", "U1", "1.001", "<@USELF> ping", "", "", false, nil, slack.Blocks{}, nil)

	if got := db.GetChannelMentionCount("C1"); got != 0 {
		t.Errorf("DM mention bumped mention_count = %d, want 0", got)
	}
}

func TestOnMessage_NoSelfMention_DoesNotBump(t *testing.T) {
	// Plain message without an @-mention must not bump.
	h, db := mentionHandler(t, "channel", "USELF")
	h.OnMessage("C1", "U1", "1.001", "just chatting", "", "", false, nil, slack.Blocks{}, nil)

	if got := db.GetChannelMentionCount("C1"); got != 0 {
		t.Errorf("non-mention bumped mention_count = %d, want 0", got)
	}
}

func TestOnMessage_GroupDMMention_DoesNotBumpMention(t *testing.T) {
	// Group DMs (mpim) are excluded from the realtime bump path
	// because the sidebar's mention sort/render only handles public
	// + private channels. Writing here would accumulate values that
	// nothing displays — a foot-gun for whoever extends the read
	// path next.
	h, db := mentionHandler(t, "group_dm", "USELF")
	h.OnMessage("C1", "U1", "1.001", "<@USELF> ping", "", "", false, nil, slack.Blocks{}, nil)

	if got := db.GetChannelMentionCount("C1"); got != 0 {
		t.Errorf("group_dm mention bumped mention_count = %d, want 0", got)
	}
}

func TestOnMessage_AppMention_DoesNotBumpMention(t *testing.T) {
	// Mirrors group_dm: apps don't render mention badges, so they
	// must not write to mention_count either.
	h, db := mentionHandler(t, "app", "USELF")
	h.OnMessage("C1", "U1", "1.001", "<@USELF> ping", "", "", false, nil, slack.Blocks{}, nil)

	if got := db.GetChannelMentionCount("C1"); got != 0 {
		t.Errorf("app mention bumped mention_count = %d, want 0", got)
	}
}

func TestOnMessage_PrivateChannelMention_BumpsCount(t *testing.T) {
	// Private channels are part of the channel-kind read path; they
	// must bump just like public channels.
	h, db := mentionHandler(t, "private", "USELF")
	h.OnMessage("C1", "U1", "1.001", "<@USELF> ping", "", "", false, nil, slack.Blocks{}, nil)

	if got := db.GetChannelMentionCount("C1"); got != 1 {
		t.Errorf("private channel mention bumped mention_count = %d, want 1", got)
	}
}

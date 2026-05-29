package main

import (
	"encoding/json"
	"os"
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/slack-go/slack"
)

// TestRealCultureFlyPayload_FullPipeline drives the EXACT raw_json captured
// from the live cache.db for the forwarded message
// C06DXGR74AY/1780025488.126509 (4190 bytes, including the attachment
// `blocks` field that the trimmed fixture omits). It proves the real
// payload still yields a LegacyAttachment whose SourceURL is the original
// CAJ5UJ8PR/1779978601.179149 permalink — the exact thing the P jump reads.
func TestRealCultureFlyPayload_FullPipeline(t *testing.T) {
	raw, err := os.ReadFile("testdata/culturefly_real.json")
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	const wantSourceURL = "https://sendbird.slack.com/archives/CAJ5UJ8PR/p1779978601179149?thread_ts=1779978601.179149&cid=CAJ5UJ8PR"

	// Direct parse path.
	var m slack.Message
	if err := json.Unmarshal(raw, &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	atts := extractLegacyAttachments(m.Attachments)
	if len(atts) != 1 {
		t.Fatalf("extractLegacyAttachments: got %d, want 1", len(atts))
	}
	if atts[0].SourceURL != wantSourceURL {
		t.Fatalf("SourceURL = %q, want %q", atts[0].SourceURL, wantSourceURL)
	}

	// Full cached-render pipeline (stored row -> MessageItem).
	db := newCacheForTest(t)
	if err := db.UpsertMessage(cache.Message{
		TS:          "1780025488.126509",
		ChannelID:   "C06DXGR74AY",
		WorkspaceID: "T1",
		UserID:      "U02BZCVBBPH",
		RawJSON:     string(raw),
		CreatedAt:   1,
	}); err != nil {
		t.Fatal(err)
	}
	items := loadCachedMessages(db, "USELF", "C06DXGR74AY", nil, "3:04 PM", nil)
	if len(items) != 1 {
		t.Fatalf("loadCachedMessages: got %d, want 1", len(items))
	}
	if len(items[0].LegacyAttachments) != 1 {
		t.Fatalf("rendered MessageItem: got %d legacy attachments, want 1", len(items[0].LegacyAttachments))
	}
	if got := items[0].LegacyAttachments[0].SourceURL; got != wantSourceURL {
		t.Fatalf("rendered SourceURL = %q, want %q", got, wantSourceURL)
	}
}

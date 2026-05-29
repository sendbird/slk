package main

import (
	"encoding/json"
	"testing"

	"github.com/gammons/slk/internal/cache"
	"github.com/slack-go/slack"
)

// realCultureFlyRawJSON is the actual cached raw_json (trimmed) for the
// forwarded message C06DXGR74AY/1780025488.126509, whose attachment
// `from_url` points at the original CAJ5UJ8PR/1779978601.179149.
const realCultureFlyRawJSON = `{
	"type":"message",
	"ts":"1780025488.126509",
	"user":"U02BZCVBBPH",
	"attachments":[{
		"color":"D0D0D0",
		"fallback":"[May 28th, 2026 11:30 PM] scott.slover: Congrats",
		"text":"Congrats to a quick AI WIN with CultureFly!!",
		"author_name":"Scott Slover",
		"from_url":"https://sendbird.slack.com/archives/CAJ5UJ8PR/p1779978601179149?thread_ts=1779978601.179149&cid=CAJ5UJ8PR",
		"ts":1779978601.179149
	}]
}`

const realCultureFlySourceURL = "https://sendbird.slack.com/archives/CAJ5UJ8PR/p1779978601179149?thread_ts=1779978601.179149&cid=CAJ5UJ8PR"

// TestExtractLegacyAttachments_PreservesFromURL verifies the real
// production payload shape (a quoted/forwarded message attachment with
// a `from_url` source permalink) survives the slack.Message ->
// blockkit.LegacyAttachment parse with SourceURL populated, which the P
// (jump-to-original) action depends on.
func TestExtractLegacyAttachments_PreservesFromURL(t *testing.T) {
	var m slack.Message
	if err := json.Unmarshal([]byte(realCultureFlyRawJSON), &m); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	atts := extractLegacyAttachments(m.Attachments)
	if len(atts) != 1 {
		t.Fatalf("expected 1 legacy attachment, got %d", len(atts))
	}
	if atts[0].SourceURL != realCultureFlySourceURL {
		t.Fatalf("SourceURL = %q, want %q", atts[0].SourceURL, realCultureFlySourceURL)
	}
}

// TestLoadCachedMessages_PopulatesAttachmentSourceURL drives the FULL
// cached-render pipeline a real channel open uses: a stored DB row with
// the real raw_json -> loadCachedMessages -> enrichCachedRow ->
// MessageItem. It proves MessageItem.LegacyAttachments[0].SourceURL is
// populated end-to-end, which is exactly what the P jump action reads
// to jump from the forward to the original message.
func TestLoadCachedMessages_PopulatesAttachmentSourceURL(t *testing.T) {
	db := newCacheForTest(t)
	if err := db.UpsertMessage(cache.Message{
		TS:          "1780025488.126509",
		ChannelID:   "C06DXGR74AY",
		WorkspaceID: "T1",
		UserID:      "U02BZCVBBPH",
		RawJSON:     realCultureFlyRawJSON,
		CreatedAt:   1,
	}); err != nil {
		t.Fatal(err)
	}

	items := loadCachedMessages(db, "USELF", "C06DXGR74AY", nil, "3:04 PM", nil)
	if len(items) != 1 {
		t.Fatalf("want 1 cached message, got %d", len(items))
	}
	if len(items[0].LegacyAttachments) != 1 {
		t.Fatalf("want 1 legacy attachment on rendered MessageItem, got %d", len(items[0].LegacyAttachments))
	}
	if got := items[0].LegacyAttachments[0].SourceURL; got != realCultureFlySourceURL {
		t.Fatalf("rendered MessageItem SourceURL = %q, want %q", got, realCultureFlySourceURL)
	}
}

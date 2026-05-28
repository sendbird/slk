// internal/ui/slack_permalink_test.go
package ui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
)

func TestParseSlackPermalink_TopLevelMessage(t *testing.T) {
	target, ok := parseSlackPermalink("https://sendbird.slack.com/archives/C0AMJQ2Q98S/p1779493681435679")
	if !ok {
		t.Fatal("expected permalink parse to succeed")
	}
	if target.ChannelID != "C0AMJQ2Q98S" {
		t.Errorf("ChannelID = %q, want C0AMJQ2Q98S", target.ChannelID)
	}
	if target.TS != "1779493681.435679" {
		t.Errorf("TS = %q, want 1779493681.435679", target.TS)
	}
	if target.ThreadTS != "" {
		t.Errorf("ThreadTS = %q, want empty for top-level message", target.ThreadTS)
	}
}

func TestParseSlackPermalink_ThreadReply(t *testing.T) {
	target, ok := parseSlackPermalink(
		"https://sendbird.slack.com/archives/C0AMJQ2Q98S/p1779504912102269?thread_ts=1779475870.237409&cid=C0AMJQ2Q98S")
	if !ok {
		t.Fatal("expected permalink parse to succeed")
	}
	if target.ChannelID != "C0AMJQ2Q98S" {
		t.Errorf("ChannelID = %q", target.ChannelID)
	}
	if target.TS != "1779504912.102269" {
		t.Errorf("TS = %q, want 1779504912.102269", target.TS)
	}
	if target.ThreadTS != "1779475870.237409" {
		t.Errorf("ThreadTS = %q, want 1779475870.237409", target.ThreadTS)
	}
}

func TestParseSlackPermalink_RejectsNonSlackHost(t *testing.T) {
	if _, ok := parseSlackPermalink("https://example.com/archives/C1/p1700000000000000"); ok {
		t.Fatal("non-slack host should not match")
	}
}

func TestParseSlackPermalink_RejectsPlainURL(t *testing.T) {
	if _, ok := parseSlackPermalink("https://sendbird.slack.com/team/general"); ok {
		t.Fatal("non-archive URL should not match")
	}
}

func TestOpenLinkCmd_PermalinkDispatchesChannelAndThread(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	a.SetChannelLookupFunc(func(channelID string) (string, string, bool) {
		if channelID == "C0AMJQ2Q98S" {
			return "covista-setup", "channel", true
		}
		return "", "", false
	})

	cmd := a.openLinkCmd(
		"https://sendbird.slack.com/archives/C0AMJQ2Q98S/p1779504912102269?thread_ts=1779475870.237409&cid=C0AMJQ2Q98S")
	if cmd == nil {
		t.Fatal("permalink should produce a cmd, not nil")
	}
	msgs := drainSequenced(cmd)
	sel, ok := findMsg[ChannelSelectedMsg](msgs)
	if !ok {
		t.Fatalf("expected ChannelSelectedMsg in dispatched cmds, got: %#v", msgs)
	}
	if sel.ID != "C0AMJQ2Q98S" {
		t.Errorf("ChannelSelectedMsg.ID = %q", sel.ID)
	}
	if sel.Name != "covista-setup" {
		t.Errorf("ChannelSelectedMsg.Name = %q, want covista-setup (channel lookup must populate it)", sel.Name)
	}
	thr, ok := findMsg[ThreadOpenedMsg](msgs)
	if !ok {
		t.Fatalf("expected ThreadOpenedMsg in dispatched cmds, got: %#v", msgs)
	}
	if thr.ChannelID != "C0AMJQ2Q98S" || thr.ThreadTS != "1779475870.237409" {
		t.Errorf("ThreadOpenedMsg = %+v", thr)
	}
	if a.pendingJumpChannelID != "C0AMJQ2Q98S" || a.pendingJumpTS != "1779504912.102269" {
		t.Errorf("pending jump = %q/%q", a.pendingJumpChannelID, a.pendingJumpTS)
	}
}

func TestOpenLinkCmd_NonPermalinkDoesNotDispatchChannel(t *testing.T) {
	a := NewApp()
	cmd := a.openLinkCmd("https://example.com/some/page")
	if cmd == nil {
		// External-open cmds return nil when executed (they call out
		// to the OS in a goroutine), but openLinkCmd itself returns a
		// non-nil tea.Cmd wrapping openDefaultAppCmd. Nil here means
		// nothing got returned at all — that would silently swallow
		// the click, so flag it.
		t.Fatal("non-permalink link should still produce an external-open cmd")
	}
	msgs := drainSequenced(cmd)
	if _, ok := findMsg[ChannelSelectedMsg](msgs); ok {
		t.Fatal("non-permalink should not dispatch ChannelSelectedMsg")
	}
	if _, ok := findMsg[ThreadOpenedMsg](msgs); ok {
		t.Fatal("non-permalink should not dispatch ThreadOpenedMsg")
	}
	// And no in-app jump state should be touched.
	if a.pendingJumpChannelID != "" || a.pendingJumpTS != "" {
		t.Fatalf("pendingJump touched: %q/%q", a.pendingJumpChannelID, a.pendingJumpTS)
	}
}

// guarantee reflect-typed inspection works for unexported sequenceMsg
// — if bubbletea ever exports a typed name we'll catch it here.
func TestDrainSequenced_HandlesSequenceMsg(t *testing.T) {
	cmd := tea.Sequence(
		func() tea.Msg { return ChannelSelectedMsg{ID: "C1"} },
		func() tea.Msg { return ThreadOpenedMsg{ChannelID: "C1", ThreadTS: "1.0"} },
	)
	msgs := drainSequenced(cmd)
	if len(msgs) != 2 {
		t.Fatalf("expected 2 drained msgs, got %d (%v)", len(msgs), msgs)
	}
	if reflect.TypeOf(msgs[0]).Name() != "ChannelSelectedMsg" {
		t.Errorf("first msg = %T, want ChannelSelectedMsg", msgs[0])
	}
	if reflect.TypeOf(msgs[1]).Name() != "ThreadOpenedMsg" {
		t.Errorf("second msg = %T, want ThreadOpenedMsg", msgs[1])
	}
}

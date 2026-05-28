// internal/ui/activity_preview_click_test.go
package ui

import (
	"reflect"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui/messages"
)

// drainSequenced walks tea.BatchMsg / sequence cmds and returns every
// leaf message they would dispatch. Sequence cmds use an unexported
// bubbletea type (sequenceMsg []Cmd); we reflect-fall back to that
// shape rather than tightly coupling to it.
func drainSequenced(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	msg := cmd()
	if msg == nil {
		return nil
	}
	if batch, ok := msg.(tea.BatchMsg); ok {
		var out []tea.Msg
		for _, c := range batch {
			out = append(out, drainSequenced(c)...)
		}
		return out
	}
	// Fall back to reflect for sequenceMsg ([]tea.Cmd).
	v := reflect.ValueOf(msg)
	if v.Kind() == reflect.Slice && v.Type().Elem().Kind() == reflect.Func {
		var out []tea.Msg
		for i := 0; i < v.Len(); i++ {
			c, ok := v.Index(i).Interface().(tea.Cmd)
			if !ok {
				continue
			}
			out = append(out, drainSequenced(c)...)
		}
		return out
	}
	return []tea.Msg{msg}
}

// findMsg returns the first message in msgs that matches the given
// type via reflect.TypeOf. Used because tea.Cmd output can be either
// a BatchMsg or a sequenceMsg that wraps the actual payloads.
func findMsg[T any](msgs []tea.Msg) (T, bool) {
	var zero T
	target := reflect.TypeOf(zero)
	for _, m := range msgs {
		if reflect.TypeOf(m) == target {
			return m.(T), true
		}
	}
	return zero, false
}

// activityPreviewClickFixture wires an App into ViewActivity with one
// thread activity item and a preview pane populated with the thread's
// parent+reply, then renders so panelAt has real layout bounds. The
// caller can compute click coordinates from the returned bounds.
type activityPreviewClickFixture struct {
	app           *App
	previewStartX int // first pane-local column inside the preview region
	previewEndX   int // exclusive
}

func newActivityPreviewClickFixture(t *testing.T) *activityPreviewClickFixture {
	t.Helper()
	a := NewApp()
	a.width = 200
	a.height = 40
	a.activeTeamID = "T1"
	a.view = ViewActivity
	a.focusedPanel = PanelMessages

	a.activityView.SetItems([]cache.ActivityItem{{
		Kind:        "thread_reply",
		ChannelID:   "C1",
		ChannelName: "covista-setup",
		ChannelType: "channel",
		TS:          "2.0",
		ThreadTS:    "1.0",
		UserID:      "U2",
		Text:        "reply body",
	}})
	a.activityPreview.SetChannel("covista-setup", "")
	a.activityPreview.SetChannelType("channel")
	a.activityPreview.SetMessages([]messages.MessageItem{{
		TS:         "1.0",
		ThreadTS:   "1.0",
		UserID:     "U1",
		UserName:   "Root",
		Text:       "parent body",
		ReplyCount: 2,
		Timestamp:  "12:00 PM",
	}, {
		TS:        "2.0",
		ThreadTS:  "1.0",
		UserID:    "U2",
		UserName:  "Jane",
		Text:      "reply body",
		Timestamp: "12:01 PM",
	}})

	// First render populates layoutMsgEnd / layoutActivityPreviewEnd
	// and primes the messages.Model cache used by ClickAt.
	_ = a.View()

	if a.layoutActivityPreviewEnd <= a.layoutMsgEnd {
		t.Fatalf("preview pane was not allocated; layoutMsgEnd=%d layoutActivityPreviewEnd=%d",
			a.layoutMsgEnd, a.layoutActivityPreviewEnd)
	}
	return &activityPreviewClickFixture{
		app:           a,
		previewStartX: a.layoutMsgEnd + 1,
		previewEndX:   a.layoutActivityPreviewEnd,
	}
}

// TestActivityPreviewClick_DispatchesChannelAndThread is the core
// regression for the previously-broken activity preview click flow:
// clicking on a preview row must (a) focus the preview, (b) select
// the underlying message, and (c) dispatch ChannelSelectedMsg plus a
// ThreadOpenedMsg when the clicked message participates in a thread.
func TestActivityPreviewClick_DispatchesChannelAndThread(t *testing.T) {
	f := newActivityPreviewClickFixture(t)
	a := f.app

	// Click at any non-chrome row inside the preview. We deliberately
	// don't hard-code a specific message-row mapping (the chrome
	// height depends on the channel header rendering); instead we
	// rely on ClickAt's "pick the message whose row range contains
	// absoluteY" walk to select something deterministic.
	clickX := f.previewStartX + 2
	clickY := 6
	_, cmd := a.Update(tea.MouseClickMsg{X: clickX, Y: clickY, Button: tea.MouseLeft})

	if a.focusedPanel != PanelActivityPreview {
		t.Fatalf("focusedPanel after preview click = %v, want PanelActivityPreview", a.focusedPanel)
	}
	if cmd == nil {
		t.Fatal("activity preview click produced no cmd; expected ChannelSelectedMsg dispatch")
	}
	msgs := drainSequenced(cmd)
	sel, ok := findMsg[ChannelSelectedMsg](msgs)
	if !ok {
		t.Fatalf("expected ChannelSelectedMsg in dispatched cmds, got: %#v", msgs)
	}
	if sel.ID != "C1" {
		t.Fatalf("ChannelSelectedMsg.ID = %q, want C1", sel.ID)
	}
	// The activity item is a thread reply, so a ThreadOpenedMsg must
	// follow the channel switch.
	thr, ok := findMsg[ThreadOpenedMsg](msgs)
	if !ok {
		t.Fatalf("expected ThreadOpenedMsg in dispatched cmds, got: %#v", msgs)
	}
	if thr.ChannelID != "C1" || thr.ThreadTS != "1.0" {
		t.Fatalf("ThreadOpenedMsg = %+v, want channel=C1 threadTS=1.0", thr)
	}
	if a.pendingJumpChannelID != "C1" || a.pendingJumpTS == "" {
		t.Fatalf("pending jump not set: channel=%q ts=%q",
			a.pendingJumpChannelID, a.pendingJumpTS)
	}
}

// TestActivityPreviewEnter_DispatchesChannelAndThread exercises the
// keyboard path: focus the preview and press Enter, the same way the
// mouse click does — both routes call openSelectedActivityItem, but
// the previously-shipped handleEnter version was unverified.
func TestActivityPreviewEnter_DispatchesChannelAndThread(t *testing.T) {
	f := newActivityPreviewClickFixture(t)
	a := f.app
	a.focusedPanel = PanelActivityPreview
	a.activityPreview.SelectByIndex(1) // the reply
	_ = a.View()

	cmd := a.handleEnter()
	if cmd == nil {
		t.Fatal("Enter on activity preview returned nil cmd")
	}
	msgs := drainSequenced(cmd)
	if _, ok := findMsg[ChannelSelectedMsg](msgs); !ok {
		t.Fatalf("Enter must dispatch ChannelSelectedMsg, got: %#v", msgs)
	}
	if _, ok := findMsg[ThreadOpenedMsg](msgs); !ok {
		t.Fatalf("Enter on a thread reply must also dispatch ThreadOpenedMsg, got: %#v", msgs)
	}
}

// TestActivityPreviewClick_LinkOpensPermalinkInApp guards the
// integration between activity-preview link clicks and Slack
// permalink routing: clicking a Slack archive URL inside the
// preview must navigate in-app rather than launching the browser.
func TestActivityPreviewClick_LinkOpensPermalinkInApp(t *testing.T) {
	f := newActivityPreviewClickFixture(t)
	a := f.app
	a.SetChannelLookupFunc(func(channelID string) (string, string, bool) {
		if channelID == "C0AMJQ2Q98S" {
			return "covista-setup", "channel", true
		}
		return "", "", false
	})

	// Drive the integration via openLinkCmd directly — the mouse
	// click flow funnels through HitTestLink → openLinkCmd, but
	// HitTestLink depends on rendered link footprints that the test
	// can't easily seed without a full render dance.
	cmd := a.openLinkCmd("https://sendbird.slack.com/archives/C0AMJQ2Q98S/p1779493681435679")
	if cmd == nil {
		t.Fatal("openLinkCmd returned nil for a Slack permalink")
	}
	msgs := drainSequenced(cmd)
	sel, ok := findMsg[ChannelSelectedMsg](msgs)
	if !ok {
		t.Fatalf("Slack permalink must dispatch ChannelSelectedMsg, got: %#v", msgs)
	}
	if sel.ID != "C0AMJQ2Q98S" || sel.Name != "covista-setup" {
		t.Errorf("ChannelSelectedMsg = %+v", sel)
	}
}

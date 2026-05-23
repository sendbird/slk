package ui

import (
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/ui/messages"
)

func newThreadClickTestApp(t *testing.T, items []messages.MessageItem) *App {
	t.Helper()
	a := NewApp()
	a.width = 120
	a.height = 30
	a.activeChannelID = "C-thread-click"
	a.messagepane.SetMessages(items)
	// Force a render so layout offsets, hit-test rows, and chrome
	// height all populate the same way they do in production.
	_ = a.View()
	return a
}

// TestApp_ClickOnThreadParentOpensThreadPanel verifies that left-clicking
// a message that already has replies opens the corresponding thread in
// the right panel and dispatches a fetch for the parent's TS, all while
// keeping the user focused on the message pane (no keyboard hand-off).
func TestApp_ClickOnThreadParentOpensThreadPanel(t *testing.T) {
	a := newThreadClickTestApp(t, []messages.MessageItem{
		{
			TS:         "100.0",
			UserID:     "U1",
			UserName:   "alice",
			Text:       "kickoff",
			Timestamp:  "1:00 PM",
			ReplyCount: 3,
		},
	})

	var fetchedChannel, fetchedThread string
	a.SetThreadFetcher(func(channelID, threadTS string) tea.Msg {
		fetchedChannel = channelID
		fetchedThread = threadTS
		return ThreadRepliesLoadedMsg{ThreadTS: threadTS, Replies: nil}
	})

	pressX := a.layoutSidebarEnd + 2
	_, cmd := a.Update(tea.MouseClickMsg{X: pressX, Y: 4, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("expected a thread-fetch tea.Cmd on click of a thread parent; got nil")
	}
	_ = drainBatch(cmd)

	if !a.threadVisible {
		t.Fatal("threadVisible should be true after clicking a message with replies")
	}
	if a.focusedPanel != PanelMessages {
		t.Fatalf("focus should stay on the message pane after a mouse-driven thread open; got %v", a.focusedPanel)
	}
	if got := a.threadPanel.ThreadTS(); got != "100.0" {
		t.Errorf("threadPanel.ThreadTS() = %q, want %q", got, "100.0")
	}
	if fetchedChannel != "C-thread-click" || fetchedThread != "100.0" {
		t.Errorf("threadFetcher called with (%q, %q); want (C-thread-click, 100.0)", fetchedChannel, fetchedThread)
	}
}

// TestApp_ClickOnThreadReplyOpensParentThread verifies that left-clicking
// a reply message (ThreadTS != TS) opens its parent thread, not a new
// thread anchored on the reply's own TS.
func TestApp_ClickOnThreadReplyOpensParentThread(t *testing.T) {
	a := newThreadClickTestApp(t, []messages.MessageItem{
		{
			TS:        "200.5",
			UserID:    "U2",
			UserName:  "bob",
			Text:      "reply",
			Timestamp: "1:05 PM",
			ThreadTS:  "200.0",
		},
	})

	var fetchedThread string
	a.SetThreadFetcher(func(channelID, threadTS string) tea.Msg {
		fetchedThread = threadTS
		return ThreadRepliesLoadedMsg{ThreadTS: threadTS, Replies: nil}
	})

	pressX := a.layoutSidebarEnd + 2
	_, cmd := a.Update(tea.MouseClickMsg{X: pressX, Y: 4, Button: tea.MouseLeft})
	if cmd == nil {
		t.Fatal("expected a thread-fetch tea.Cmd on click of a reply; got nil")
	}
	_ = drainBatch(cmd)

	if !a.threadVisible {
		t.Fatal("threadVisible should be true after clicking a thread reply")
	}
	if got := a.threadPanel.ThreadTS(); got != "200.0" {
		t.Errorf("threadPanel.ThreadTS() = %q, want parent %q", got, "200.0")
	}
	if fetchedThread != "200.0" {
		t.Errorf("threadFetcher called with threadTS=%q; want parent 200.0", fetchedThread)
	}
}

// TestApp_ClickOnPlainMessageDoesNotOpenThread guards the inverse case:
// a click on a regular top-level message with no replies must not pop the
// thread panel open. Only thread-bearing cells get the auto-open.
func TestApp_ClickOnPlainMessageDoesNotOpenThread(t *testing.T) {
	a := newThreadClickTestApp(t, []messages.MessageItem{
		{
			TS:        "300.0",
			UserID:    "U3",
			UserName:  "carol",
			Text:      "no thread here",
			Timestamp: "1:10 PM",
		},
	})

	fetched := 0
	a.SetThreadFetcher(func(channelID, threadTS string) tea.Msg {
		fetched++
		return ThreadRepliesLoadedMsg{ThreadTS: threadTS, Replies: nil}
	})

	pressX := a.layoutSidebarEnd + 2
	_, cmd := a.Update(tea.MouseClickMsg{X: pressX, Y: 4, Button: tea.MouseLeft})
	_ = drainBatch(cmd)

	if a.threadVisible {
		t.Fatal("threadVisible must remain false when clicking a message with no replies")
	}
	if fetched != 0 {
		t.Errorf("threadFetcher should not be called for a plain message; got %d call(s)", fetched)
	}
}

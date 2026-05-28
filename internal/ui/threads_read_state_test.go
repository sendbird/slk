// internal/ui/threads_read_state_test.go
package ui

import (
	"sync"
	"testing"

	tea "charm.land/bubbletea/v2"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui/messages"
)

func newThreadRoot(ts string) messages.MessageItem {
	return messages.MessageItem{
		TS:       ts,
		ThreadTS: ts,
		UserID:   "U1",
		UserName: "Root",
		Text:     "parent body",
	}
}

// TestOpenSelectedThread_TriggersThreadMarker is the regression for
// the "I opened the thread but the unread count won't drop" bug:
// openSelectedThreadCmd must fire the durable threadMarker so the
// persisted per-thread last_read advances and a follow-up refresh
// doesn't re-derive Unread=true from a stale boundary.
func TestOpenSelectedThread_TriggersThreadMarker(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	// Leave view at the default (not ViewThreads) so the
	// ThreadsListLoadedMsg handler doesn't auto-open the selected
	// thread before the test gets a chance to observe the unread
	// count.
	type call struct{ channelID, threadTS, ts string }
	var mu sync.Mutex
	var calls []call
	done := make(chan struct{}, 1)
	a.SetThreadMarker(func(channelID, threadTS, ts string) {
		mu.Lock()
		calls = append(calls, call{channelID, threadTS, ts})
		mu.Unlock()
		select {
		case done <- struct{}{}:
		default:
		}
	})
	a.SetThreadFetcher(func(channelID, threadTS string) tea.Msg {
		return ThreadRepliesLoadedMsg{ChannelID: channelID, ThreadTS: threadTS, Replies: nil}
	})
	a.Update(ThreadsListLoadedMsg{
		TeamID: "T1",
		Summaries: []cache.ThreadSummary{{
			ChannelID:    "C1",
			ChannelName:  "covista",
			ChannelType:  "channel",
			ThreadTS:     "100.0",
			ParentText:   "parent",
			ParentUserID: "U2",
			ReplyCount:   3,
			LastReplyTS:  "300.0",
			LastReplyBy:  "U3",
			Unread:       true,
		}},
		SubscriptionsAvailable: true,
	})

	if a.threadsView.UnreadCount() != 1 {
		t.Fatalf("precondition: unread count = %d, want 1", a.threadsView.UnreadCount())
	}

	cmd := a.openSelectedThreadCmd(false)
	if cmd == nil {
		t.Fatal("openSelectedThreadCmd returned nil")
	}

	// Local Unread must flip immediately so the UI badge falls before
	// any network round-trip lands.
	if a.threadsView.UnreadCount() != 0 {
		t.Fatalf("post-open unread count = %d, want 0 (local MarkSelectedRead must run synchronously)", a.threadsView.UnreadCount())
	}

	// And the durable marker must be invoked so the cache's per-thread
	// last_read advances ahead of any ThreadsListDirtyMsg refresh.
	select {
	case <-done:
	default:
		// Give the goroutine a chance.
	}
	if len(calls) == 0 {
		// The marker is invoked in a goroutine; sleep briefly to
		// let it land. We avoid time.Sleep at test scope by polling
		// the channel one more time.
		select {
		case <-done:
		}
	}
	mu.Lock()
	defer mu.Unlock()
	if len(calls) != 1 {
		t.Fatalf("threadMarker calls = %d, want 1: %+v", len(calls), calls)
	}
	got := calls[0]
	if got.channelID != "C1" || got.threadTS != "100.0" {
		t.Errorf("threadMarker target = %+v, want C1/100.0", got)
	}
	if got.ts != "300.0" {
		t.Errorf("threadMarker boundary ts = %q, want 300.0 (the latest reply)", got.ts)
	}
}

// TestThreadsListDirtyMsg_DispatchesRefetch is a small guard that the
// realtime add/remove plumbing for the threads list still fires a
// re-fetch when an event marks the list dirty. This caught a previous
// regression where the handler short-circuited on a missing fetcher.
func TestThreadsListDirtyMsg_DispatchesRefetch(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	fired := 0
	a.SetThreadsListFetcher(func(teamID string) tea.Msg {
		if teamID == "T1" {
			fired++
		}
		return ThreadsListLoadedMsg{TeamID: teamID}
	})
	_, cmd := a.Update(ThreadsListDirtyMsg{TeamID: "T1"})
	if cmd == nil {
		t.Fatal("ThreadsListDirtyMsg should schedule a refetch cmd")
	}
	drainSequenced(cmd)
	if fired != 1 {
		t.Fatalf("threads fetcher fired %d times, want 1", fired)
	}
}

// TestActivityListDirtyMsg_DispatchesRefetch mirrors the threads
// case for the activity feed: dirty notifications coming from
// thread_marked / thread_subscribed / new-message events must
// re-pull the activity list so add/remove is reflected live.
func TestActivityListDirtyMsg_DispatchesRefetch(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	fired := 0
	a.SetActivityListFetcher(func(teamID string) tea.Msg {
		if teamID == "T1" {
			fired++
		}
		return ActivityListLoadedMsg{TeamID: teamID}
	})
	_, cmd := a.Update(ActivityListDirtyMsg{TeamID: "T1"})
	if cmd == nil {
		t.Fatal("ActivityListDirtyMsg should schedule a refetch cmd")
	}
	drainSequenced(cmd)
	if fired != 1 {
		t.Fatalf("activity fetcher fired %d times, want 1", fired)
	}
}

// TestThreadOpenInChannel_DispatchesChannelSwitch verifies the
// "Open in channel" header action: it must navigate the messages
// pane to the thread's parent channel AND keep the thread panel
// open at the same root. Without the ThreadOpenedMsg follow-up,
// the user would land in the channel with no thread context.
func TestThreadOpenInChannel_DispatchesChannelSwitch(t *testing.T) {
	a := NewApp()
	a.activeTeamID = "T1"
	a.SetChannelLookupFunc(func(channelID string) (string, string, bool) {
		if channelID == "C1" {
			return "covista", "channel", true
		}
		return "", "", false
	})
	parent := newThreadRoot("100.0")
	parent.ReplyCount = 2
	a.threadPanel.SetThread(parent, nil, "C1", "100.0")
	a.threadVisible = true
	// Simulate a prior open of this same thread so the dedup short-
	// circuit in openSelectedThreadCmd would normally fire — the
	// "Open in channel" flow has to reset those keys, otherwise the
	// follow-up ThreadOpenedMsg gets swallowed and the user lands in
	// the channel with the thread panel closed.
	a.lastOpenedChannelID = "C1"
	a.lastOpenedThreadTS = "100.0"

	cmd := a.openThreadInChannelCmd()
	if cmd == nil {
		t.Fatal("openThreadInChannelCmd returned nil")
	}
	if !a.threadVisible {
		t.Fatal("thread panel must stay visible after Open-in-channel; the user expects the same thread open in the channel context")
	}
	if a.lastOpenedChannelID != "" || a.lastOpenedThreadTS != "" {
		t.Fatalf("open dedup keys must reset so the follow-up ThreadOpenedMsg re-opens the thread; got %q/%q",
			a.lastOpenedChannelID, a.lastOpenedThreadTS)
	}
	if a.pendingJumpChannelID != "C1" || a.pendingJumpTS != "100.0" {
		t.Fatalf("pending jump = %q/%q, want C1/100.0", a.pendingJumpChannelID, a.pendingJumpTS)
	}
	msgs := drainSequenced(cmd)
	sel, ok := findMsg[ChannelSelectedMsg](msgs)
	if !ok {
		t.Fatalf("expected ChannelSelectedMsg, got: %#v", msgs)
	}
	if sel.ID != "C1" || sel.Name != "covista" {
		t.Fatalf("ChannelSelectedMsg = %+v, want C1/covista", sel)
	}
	thr, ok := findMsg[ThreadOpenedMsg](msgs)
	if !ok {
		t.Fatalf("expected ThreadOpenedMsg to follow ChannelSelectedMsg, got: %#v", msgs)
	}
	if thr.ChannelID != "C1" || thr.ThreadTS != "100.0" {
		t.Fatalf("ThreadOpenedMsg = %+v, want C1/100.0", thr)
	}
}

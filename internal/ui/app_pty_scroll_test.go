package ui

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/creack/pty"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui/messages"
)

func TestAppPTYThreadsViewMouseWheelScrollDoesNotPanic(t *testing.T) {
	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	const width = 120
	const height = 32
	if err := pty.Setsize(tty, &pty.Winsize{Cols: width, Rows: height}); err != nil {
		t.Fatalf("set pty size: %v", err)
	}

	var out bytes.Buffer
	readDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&out, ptmx)
		readDone <- err
	}()

	app := NewApp()
	app.width = width
	app.height = height
	app.view = ViewThreads
	app.focusedPanel = PanelMessages
	app.sidebarVisible = true
	app.threadsView.SetSummaries(threadScrollSummaries(80))
	app.threadFetcher = func(channelID, threadTS string) tea.Msg {
		return ThreadRepliesLoadedMsg{
			ChannelID: channelID,
			ThreadTS:  threadTS,
			Replies:   []messages.MessageItem{{TS: threadTS + ".1", Text: "reply"}},
		}
	}
	// Prime layout and open the initial thread exactly like the real app does
	// when entering the threads view.
	_ = app.View()
	if cmd := app.openSelectedThreadCmd(false); cmd != nil {
		for _, msg := range drainBatch(cmd) {
			_, _ = app.Update(msg)
		}
	}

	program := tea.NewProgram(
		app,
		tea.WithInput(tty),
		tea.WithOutput(tty),
		tea.WithWindowSize(width, height),
		tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}),
		tea.WithoutSignals(),
		tea.WithFilter(MouseWheelFilter),
	)

	done := make(chan error, 1)
	go func() {
		_, err := program.Run()
		done <- err
	}()

	// Give Bubble Tea time to enter raw mode and enable mouse reporting before
	// writing escape sequences into the PTY. Without this, the PTY can echo the
	// bytes as literal output instead of delivering them as input events.
	time.Sleep(80 * time.Millisecond)

	// Feed a rapid, deterministic pseudo-random up/down wheel burst through the
	// PTY master. Button 64 is wheel-up, 65 is wheel-down; coordinates are
	// 1-based terminal cells. The X coordinate lands inside the message/threads
	// pane for the configured width. Small sleeps between bursts let Bubble Tea
	// process multiple coalesced flushes while still keeping the input adversarial.
	buttons := []int{65, 65, 64, 65, 64, 64, 65, 65, 65, 64, 65, 64, 65, 65, 64, 65}
	for burst := 0; burst < 24; burst++ {
		for i := 0; i < len(buttons); i++ {
			button := buttons[(i+burst*5)%len(buttons)]
			if _, err := fmt.Fprintf(ptmx, "\x1b[<%d;50;8M", button); err != nil {
				t.Fatalf("write wheel escape: %v", err)
			}
		}
		time.Sleep(3 * time.Millisecond)
	}
	// After the adversarial random burst, send a few spaced wheel-down events.
	// If the random burst froze the event loop or wedged the wheel coalescer,
	// these won't be processed and the selection will remain at the top.
	for i := 0; i < 8; i++ {
		if _, err := fmt.Fprint(ptmx, "\x1b[<65;50;8M"); err != nil {
			t.Fatalf("write final wheel escape: %v", err)
		}
		time.Sleep(25 * time.Millisecond)
	}
	time.Sleep(500 * time.Millisecond)
	program.Quit()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("run bubble tea program: %v\noutput:\n%s", err, out.String())
		}
		if got := app.threadsView.SelectedIndex(); got == 0 {
			t.Fatalf("PTY wheel input did not move threads selection; output:\n%s", out.String())
		}
	case <-time.After(2 * time.Second):
		program.Kill()
		t.Fatalf("program did not exit after mouse-wheel scroll; output:\n%s", out.String())
	}
	_ = tty.Close()
	_ = ptmx.Close()
	if err := <-readDone; err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("read pty output: %v", err)
	}
}

func threadScrollSummaries(n int) []cache.ThreadSummary {
	summaries := make([]cache.ThreadSummary, n)
	for i := range summaries {
		idx := i + 1
		ts := fmt.Sprintf("%d.0", idx)
		summaries[i] = cache.ThreadSummary{
			ChannelID:    fmt.Sprintf("C%d", idx),
			ChannelName:  fmt.Sprintf("chan-%d", idx),
			ThreadTS:     ts,
			ParentTS:     ts,
			ParentUserID: "U1",
			ParentText:   fmt.Sprintf("parent %d", idx),
			LastReplyTS:  fmt.Sprintf("%d.1", idx),
			LastReplyBy:  "U2",
			Unread:       true,
		}
	}
	return summaries
}

func TestApp_MouseWheelBurstKeepsMouseReportingEnabled(t *testing.T) {
	app := NewApp()
	app.width = 120
	app.height = 32
	app.view = ViewThreads
	app.focusedPanel = PanelMessages
	app.threadsView.SetSummaries(threadScrollSummaries(20))
	_ = app.View()
	x := app.layoutSidebarEnd + 5
	_, _ = app.Update(tea.MouseWheelMsg{X: x, Y: 8, Button: tea.MouseWheelDown})
	if !app.pendingWheelActive {
		t.Fatal("wheel event should leave a pending coalesced flush")
	}
	if v := app.View(); v.MouseMode != tea.MouseModeCellMotion {
		t.Fatalf("mouse reporting must stay enabled during pending wheel flush; got %v", v.MouseMode)
	}
}

func TestMouseWheelFilterAbsorbsPendingWheelWithoutRenderMessage(t *testing.T) {
	app := NewApp()
	app.width = 120
	app.height = 32
	app.view = ViewThreads
	app.focusedPanel = PanelMessages
	app.threadsView.SetSummaries(threadScrollSummaries(20))
	_ = app.View()
	x := app.layoutSidebarEnd + 5

	msg := tea.MouseWheelMsg{X: x, Y: 8, Button: tea.MouseWheelDown}
	if got := MouseWheelFilter(app, msg); got == nil {
		t.Fatal("first wheel event should pass through so Update can schedule the flush")
	}
	_, cmd := app.Update(msg)
	if cmd == nil {
		t.Fatal("first wheel event should schedule a flush")
	}
	if got := MouseWheelFilter(app, tea.MouseWheelMsg{X: x, Y: 8, Button: tea.MouseWheelUp}); got != nil {
		t.Fatalf("pending wheel event should be absorbed before Bubble Tea renders; got %#v", got)
	}
	if !app.pendingWheelActive {
		t.Fatal("absorbed wheel event must keep pending flush active")
	}
}

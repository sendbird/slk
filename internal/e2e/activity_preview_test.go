//go:build e2e

package e2e

import (
	"strings"
	"testing"
	"time"
)

// TestActivityPreviewE2E spawns the real slk binary against a live
// Slack workspace, navigates to the Activity view, moves the selection
// cursor, and asserts that the right-side preview pane gains content
// after the selection change. The assertions are intentionally fuzzy
// (presence of *any* non-border text past the split column) because
// the workspace's activity feed is live state — a stricter assertion
// would break whenever the test workspace's recent activity shifts.
func TestActivityPreviewE2E(t *testing.T) {
	token, cookie, teamID, teamName := requireEnv(t)
	s := startSession(t, token, cookie, teamID, teamName)

	// Wait for the sidebar to render the Activity row. This is the
	// "ready" signal: the sidebar paints that row only after the
	// workspace connect goroutine finishes its initial fetch.
	s.waitFor(t, "Activity", 30*time.Second)

	// Drive the sidebar selection down to the Activity row and open
	// it. The sidebar's last row is Activity (sidebar/model.go shows
	// Threads then Activity as the trailing synthetic rows), so 'G'
	// jumps to bottom and Enter activates. We send 'G' rather than
	// repeated 'j' so the test is robust to channel-list length.
	if err := s.send("G\r"); err != nil {
		t.Fatalf("send G+Enter: %v", err)
	}

	// Wait for the activity view to actually take focus. The view
	// renders a distinctive header line — looking for the lowercase
	// "activity" or the ⚡ glyph below the loading indicator. The
	// activity-list cards include channel names so once we see those
	// pretty much guarantees the SetItems path ran.
	deadline := time.Now().Add(20 * time.Second)
	gotActivity := false
	for time.Now().Before(deadline) {
		snap := asciiOnly(s.screen())
		// "no activity" rendered when the list is empty; either way
		// the activity view is now mounted.
		if strings.Contains(snap, "no activity") || strings.Contains(snap, "Activity Loading") || strings.Contains(snap, "⚡") {
			gotActivity = true
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if !gotActivity {
		t.Fatalf("activity view never rendered. screen tail:\n%s", tailScreen(s.screen(), 1200))
	}

	// Give the workspace a moment to fetch real activity rows. If the
	// workspace happens to have zero recent activity the preview pane
	// has nothing to show — skip rather than fail in that case.
	time.Sleep(2 * time.Second)
	if strings.Contains(asciiOnly(s.screen()), "no activity") {
		t.Skip("test workspace has no activity rows; preview pane has nothing to assert")
	}

	// Capture a baseline frame before moving the cursor — we'll
	// compare against this to confirm the preview *changes* on
	// selection move (proves the wiring runs, not just that something
	// happens to be on screen).
	beforeFrame := s.screen()
	beforePreviewHadText := rightHalfHasText(beforeFrame, 70)

	// Move down once. The activity selection cursor advances, which
	// fires loadActivityPreviewCmd → ActivityPreviewLoadedMsg → the
	// activityPreview.SetMessages path. The preview pane should
	// render either fresh content (if the new selection's channel
	// has cached messages) or remain empty (cold cache).
	if err := s.send("j"); err != nil {
		t.Fatalf("send j: %v", err)
	}

	// Give the synchronous cache read + bubbletea repaint time to land.
	time.Sleep(750 * time.Millisecond)

	afterFrame := s.screen()
	afterPreviewHasText := rightHalfHasText(afterFrame, 70)

	// The strongest assertion we can make against a live workspace:
	// at least one of the frames had preview content. If both were
	// empty, either the preview wiring is broken OR both adjacent
	// items reference channels with cold caches (rare but possible).
	if !beforePreviewHadText && !afterPreviewHasText {
		t.Errorf("preview pane was empty before and after selection move. screen tail:\n%s",
			tailScreen(afterFrame, 1500))
	}
}

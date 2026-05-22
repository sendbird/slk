package sidebar

import (
	"strings"
	"testing"
)

func TestSidebar_BootstrapLoading_RendersSpinnerInsteadOfNoChannels(t *testing.T) {
	// While bootstrapping a workspace (no SetItems yet), the empty-
	// items branch must render the "Loading channels…" indicator so
	// the user has visible feedback that data is still in flight.
	m := New(nil)
	m.SetBootstrapLoading(true)

	view := m.View(20, 30)
	if !strings.Contains(view, "Loading channels") {
		t.Errorf("expected loading indicator while bootstrapLoading=true; got\n%s", view)
	}
	if strings.Contains(view, "No channels") {
		t.Errorf("expected NO 'No channels' placeholder while loading; got\n%s", view)
	}
}

func TestSidebar_BootstrapLoading_ClearedBySetItems(t *testing.T) {
	// First SetItems call (any) flips bootstrapLoading off so the
	// "No channels" placeholder takes over when the workspace
	// genuinely has zero items.
	m := New(nil)
	m.SetBootstrapLoading(true)
	m.SetItems(nil)

	view := m.View(20, 30)
	if strings.Contains(view, "Loading channels") {
		t.Errorf("expected loading indicator to clear after SetItems; got\n%s", view)
	}
	if !strings.Contains(view, "No channels") {
		t.Errorf("expected 'No channels' placeholder after SetItems(nil); got\n%s", view)
	}
}

package threadsview

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

func TestThreadsView_LoadingHidesEmptyText(t *testing.T) {
	m := New(nil, "U1")
	m.SetLoading(true)
	out := m.View(10, 30)
	if !strings.Contains(out, "Loading threads") {
		t.Errorf("expected 'Loading threads' indicator while loading=true; got\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "no threads") {
		t.Errorf("'no threads' must be suppressed while loading; got\n%s", out)
	}
}

func TestThreadsView_LoadingClearedBySetSummaries(t *testing.T) {
	m := New(nil, "U1")
	m.SetLoading(true)
	m.SetSummaries([]cache.ThreadSummary{})

	out := m.View(10, 30)
	if strings.Contains(out, "Loading threads") {
		t.Errorf("loading indicator should clear after SetSummaries; got\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "no threads") {
		t.Errorf("expected 'no threads' after empty SetSummaries; got\n%s", out)
	}
}

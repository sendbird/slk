package activityview

import (
	"strings"
	"testing"

	"github.com/gammons/slk/internal/cache"
)

func TestActivityView_LoadingHidesEmptyText(t *testing.T) {
	m := New(nil, "U1")
	m.SetLoading(true)
	out := m.View(10, 30)
	if !strings.Contains(out, "Loading activity") {
		t.Errorf("expected 'Loading activity' indicator while loading=true; got\n%s", out)
	}
	if strings.Contains(strings.ToLower(out), "no activity") {
		t.Errorf("'no activity' must be suppressed while loading; got\n%s", out)
	}
}

func TestActivityView_LoadingClearedBySetItems(t *testing.T) {
	m := New(nil, "U1")
	m.SetLoading(true)
	m.SetItems([]cache.ActivityItem{})

	out := m.View(10, 30)
	if strings.Contains(out, "Loading activity") {
		t.Errorf("loading indicator should clear after SetItems; got\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "no activity") {
		t.Errorf("expected 'no activity' after empty SetItems; got\n%s", out)
	}
}

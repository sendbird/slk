package ui

import (
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"

	"github.com/gammons/slk/internal/ui/presencemenu"
	"github.com/gammons/slk/internal/ui/themeswitcher"
)

// The theme switcher, presence menu, and file picker all navigate with j/k and
// filter on ASCII only. Under a Korean input source their navigation keys
// arrived as jamo and did nothing at all: the shortcut did not match, and the
// filter rejected the character too. These assert the jamo now reaches the same
// binding its Latin key does.

// jamoPress builds the key event a Korean input source delivers for one jamo.
func jamoPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func latinPress(r rune) tea.KeyPressMsg {
	return tea.KeyPressMsg{Code: r, Text: string(r)}
}

func TestThemeSwitcherHangulNavigatesLikeLatin(t *testing.T) {
	// `ㅓ` is the physical `j`.
	viaJamo := themeSelectionAfter(t, jamoPress('ㅓ'))
	viaLatin := themeSelectionAfter(t, latinPress('j'))

	if viaJamo == "" || viaLatin == "" {
		t.Fatalf("no theme landed (jamo=%q latin=%q); the fixture is not selecting", viaJamo, viaLatin)
	}
	if viaJamo != viaLatin {
		t.Fatalf("ㅓ selected %q, but j selects %q", viaJamo, viaLatin)
	}
	// And navigation actually moved: otherwise both would be the first item and
	// the comparison above would pass without proving anything.
	if viaLatin == "first" {
		t.Fatalf("j left the cursor on the first theme (%q); it should have moved", viaLatin)
	}
}

// themeSelectionAfter opens the theme switcher, sends one key, then confirms —
// the chosen theme is the only observable the package exposes.
func themeSelectionAfter(t *testing.T, msg tea.KeyPressMsg) string {
	t.Helper()
	app := NewApp()
	app.themeSwitcher.SetItems([]string{"first", "second", "third"})
	app.themeSwitcher.OpenWithScope(themeswitcher.ScopeGlobal, "test")
	app.SetMode(ModeThemeSwitcher)

	var chosen string
	app.themeSaveFn = func(name string, _ themeswitcher.ThemeScope) { chosen = name }

	app.handleThemeSwitcherMode(msg)
	app.handleThemeSwitcherMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	return chosen
}

func TestPresenceMenuHangulNavigatesLikeLatin(t *testing.T) {
	viaJamo := presenceActionAfter(t, jamoPress('ㅓ'))
	viaLatin := presenceActionAfter(t, latinPress('j'))

	if viaJamo != viaLatin {
		t.Fatalf("ㅓ selected action %v, but j selects %v", viaJamo, viaLatin)
	}
}

// presenceActionAfter opens the presence menu, sends one key, then confirms.
func presenceActionAfter(t *testing.T, msg tea.KeyPressMsg) presencemenu.Action {
	t.Helper()
	app := NewApp()
	app.presenceMenu.OpenWith("workspace", "active", false, time.Time{})
	app.SetMode(ModePresenceMenu)

	var got presencemenu.Action
	app.setStatusFn = func(action presencemenu.Action, _ int) { got = action }

	app.handlePresenceMenuMode(msg)
	app.handlePresenceMenuMode(tea.KeyPressMsg{Code: tea.KeyEnter})
	return got
}

// The file picker's filter is ASCII-only too, so a jamo there is a shortcut,
// not a query character — it must not land in the query.
func TestFilePickerHangulDoesNotEnterTheQuery(t *testing.T) {
	app := NewApp()
	app.filePicker.OpenAt(t.TempDir())
	app.SetMode(ModeFilePicker)

	app.handleFilePickerMode(jamoPress('ㅂ'))
	if got := app.filePicker.Query(); got != "" {
		t.Fatalf("query = %q; a jamo must not be typed into an ASCII-only filter", got)
	}
}

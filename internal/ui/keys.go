// internal/ui/keys.go
package ui

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"charm.land/bubbles/v2/key"
	tea "charm.land/bubbletea/v2"
)

type KeyMap struct {
	Up                  key.Binding
	Down                key.Binding
	Left                key.Binding
	Right               key.Binding
	Enter               key.Binding
	Escape              key.Binding
	InsertMode          key.Binding
	CommandMode         key.Binding
	SearchMode          key.Binding
	Tab                 key.Binding
	ShiftTab            key.Binding
	ToggleSidebar       key.Binding
	ToggleThread        key.Binding
	FuzzyFinder         key.Binding
	FuzzyFinderAlt      key.Binding
	Top                 key.Binding
	Bottom              key.Binding
	PageUp              key.Binding
	PageDown            key.Binding
	HalfPageUp          key.Binding
	HalfPageDown        key.Binding
	Quit                key.Binding
	QuitConfirm         key.Binding
	CloseThreadView     key.Binding
	Reaction            key.Binding
	ReactionNav         key.Binding
	Edit                key.Binding
	Delete              key.Binding
	CopyPermalink       key.Binding
	OpenPreview         key.Binding
	MarkUnread          key.Binding
	WorkspaceFinder     key.Binding
	ThemeSwitcher       key.Binding
	ThemeSwitcherGlobal key.Binding
	PresenceMenu        key.Binding
	AttachFile          key.Binding
	ToggleSection       key.Binding
	NavBack             key.Binding
	NavForward          key.Binding
	Help                key.Binding
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		Up:                  key.NewBinding(key.WithKeys("k", "up"), key.WithHelp("k/up", "up")),
		Down:                key.NewBinding(key.WithKeys("j", "down"), key.WithHelp("j/down", "down")),
		Left:                key.NewBinding(key.WithKeys("h", "left"), key.WithHelp("h/left", "left")),
		Right:               key.NewBinding(key.WithKeys("l", "right"), key.WithHelp("l/right", "right")),
		Enter:               key.NewBinding(key.WithKeys("enter"), key.WithHelp("enter", "open/confirm")),
		Escape:              key.NewBinding(key.WithKeys("esc"), key.WithHelp("esc", "back")),
		InsertMode:          key.NewBinding(key.WithKeys("i", "f2"), key.WithHelp("i/F2", "insert mode")),
		CommandMode:         key.NewBinding(key.WithKeys(":"), key.WithHelp(":", "command mode")),
		SearchMode:          key.NewBinding(key.WithKeys("/"), key.WithHelp("/", "search")),
		Tab:                 key.NewBinding(key.WithKeys("tab"), key.WithHelp("tab", "next panel")),
		ShiftTab:            key.NewBinding(key.WithKeys("shift+tab"), key.WithHelp("shift+tab", "prev panel")),
		ToggleSidebar:       key.NewBinding(key.WithKeys("ctrl+b"), key.WithHelp("ctrl+b", "toggle sidebar")),
		ToggleThread:        key.NewBinding(key.WithKeys("ctrl+]"), key.WithHelp("ctrl+]", "toggle thread")),
		FuzzyFinder:         key.NewBinding(key.WithKeys("ctrl+t"), key.WithHelp("ctrl+t", "switch channel")),
		FuzzyFinderAlt:      key.NewBinding(key.WithKeys("ctrl+p"), key.WithHelp("ctrl+p", "switch channel")),
		Top:                 key.NewBinding(key.WithKeys("g"), key.WithHelp("gg", "top")),
		Bottom:              key.NewBinding(key.WithKeys("G"), key.WithHelp("G", "bottom")),
		PageUp:              key.NewBinding(key.WithKeys("pgup"), key.WithHelp("PgUp", "page up")),
		PageDown:            key.NewBinding(key.WithKeys("pgdown"), key.WithHelp("PgDn", "page down")),
		HalfPageUp:          key.NewBinding(key.WithKeys("ctrl+u"), key.WithHelp("ctrl+u", "half page up")),
		HalfPageDown:        key.NewBinding(key.WithKeys("ctrl+d"), key.WithHelp("ctrl+d", "half page down")),
		Quit:                key.NewBinding(key.WithKeys("ctrl+c"), key.WithHelp("ctrl+c", "quit (confirm)")),
		QuitConfirm:         key.NewBinding(key.WithKeys("Q"), key.WithHelp("Q", "quit (confirm)")),
		CloseThreadView:     key.NewBinding(key.WithKeys("q"), key.WithHelp("q", "close thread view")),
		Reaction:            key.NewBinding(key.WithKeys("r"), key.WithHelp("r", "add reaction")),
		ReactionNav:         key.NewBinding(key.WithKeys("R"), key.WithHelp("R", "navigate reactions")),
		Edit:                key.NewBinding(key.WithKeys("E"), key.WithHelp("E", "edit message")),
		Delete:              key.NewBinding(key.WithKeys("D"), key.WithHelp("D", "delete message")),
		CopyPermalink:       key.NewBinding(key.WithKeys("Y", "C"), key.WithHelp("Y/C", "copy permalink")),
		OpenPreview:         key.NewBinding(key.WithKeys("O", "v"), key.WithHelp("O/v", "open image preview")),
		MarkUnread:          key.NewBinding(key.WithKeys("U"), key.WithHelp("U", "mark unread")),
		WorkspaceFinder:     key.NewBinding(key.WithKeys("ctrl+w"), key.WithHelp("ctrl+w", "switch workspace")),
		ThemeSwitcher:       key.NewBinding(key.WithKeys("ctrl+y"), key.WithHelp("ctrl+y", "switch theme (per workspace)")),
		ThemeSwitcherGlobal: key.NewBinding(key.WithKeys("ctrl+shift+y"), key.WithHelp("ctrl+shift+y", "set default theme")),
		PresenceMenu:        key.NewBinding(key.WithKeys("ctrl+s"), key.WithHelp("ctrl+s", "set status")),
		AttachFile:          key.NewBinding(key.WithKeys("ctrl+a"), key.WithHelp("ctrl+a", "attach file")),
		ToggleSection:       key.NewBinding(key.WithKeys("space"), key.WithHelp("space", "toggle section")),
		NavBack:             key.NewBinding(key.WithKeys("ctrl+h"), key.WithHelp("ctrl+h", "navigate back")),
		NavForward:          key.NewBinding(key.WithKeys("ctrl+k"), key.WithHelp("ctrl+k", "navigate forward")),
		Help:                key.NewBinding(key.WithKeys("?"), key.WithHelp("?", "show keybindings")),
	}
}

type shortcutKeyMsg struct {
	tea.KeyMsg
	keyString string
}

func (m shortcutKeyMsg) String() string { return m.keyString }

func normalizeShortcutKeyMsg(msg tea.KeyMsg) tea.KeyMsg {
	keyString, ok := koreanShortcutKeyString(msg)
	if !ok {
		return msg
	}
	return shortcutKeyMsg{KeyMsg: msg, keyString: keyString}
}

func koreanShortcutKeyString(msg tea.KeyMsg) (string, bool) {
	k := msg.Key()
	r, ok := koreanShortcutRune(k.Text)
	if !ok {
		r, ok = koreanDubeolsikShortcut[k.Code]
		if !ok {
			return "", false
		}
	}
	return shortcutStringWithModifiers(r, k.Mod), true
}

func koreanShortcutRune(text string) (rune, bool) {
	if text == "" || text == " " {
		return 0, false
	}
	r, size := utf8.DecodeRuneInString(text)
	if r == utf8.RuneError || size != len(text) {
		return 0, false
	}
	mapped, ok := koreanDubeolsikShortcut[r]
	return mapped, ok
}

func shortcutStringWithModifiers(r rune, mod tea.KeyMod) string {
	withChordModifier := mod.Contains(tea.ModCtrl) ||
		mod.Contains(tea.ModAlt) ||
		mod.Contains(tea.ModMeta) ||
		mod.Contains(tea.ModHyper) ||
		mod.Contains(tea.ModSuper)

	if !withChordModifier {
		if mod.Contains(tea.ModShift) && isASCIIAlpha(r) {
			r = unicode.ToUpper(r)
		}
		return string(r)
	}

	var b strings.Builder
	if mod.Contains(tea.ModCtrl) {
		b.WriteString("ctrl+")
	}
	if mod.Contains(tea.ModAlt) {
		b.WriteString("alt+")
	}
	if mod.Contains(tea.ModShift) {
		b.WriteString("shift+")
	}
	if mod.Contains(tea.ModMeta) {
		b.WriteString("meta+")
	}
	if mod.Contains(tea.ModHyper) {
		b.WriteString("hyper+")
	}
	if mod.Contains(tea.ModSuper) {
		b.WriteString("super+")
	}
	b.WriteRune(unicode.ToLower(r))
	return b.String()
}

func isASCIIAlpha(r rune) bool {
	return (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z')
}

var koreanDubeolsikShortcut = map[rune]rune{
	'ㅂ': 'q', 'ㅃ': 'Q',
	'ㅈ': 'w', 'ㅉ': 'W',
	'ㄷ': 'e', 'ㄸ': 'E',
	'ㄱ': 'r', 'ㄲ': 'R',
	'ㅅ': 't', 'ㅆ': 'T',
	'ㅛ': 'y',
	'ㅕ': 'u',
	'ㅑ': 'i',
	'ㅐ': 'o', 'ㅒ': 'O',
	'ㅔ': 'p', 'ㅖ': 'P',
	'ㅁ': 'a',
	'ㄴ': 's',
	'ㅇ': 'd',
	'ㄹ': 'f',
	'ㅎ': 'g',
	'ㅗ': 'h',
	'ㅓ': 'j',
	'ㅏ': 'k',
	'ㅣ': 'l',
	'ㅋ': 'z',
	'ㅌ': 'x',
	'ㅊ': 'c',
	'ㅍ': 'v',
	'ㅠ': 'b',
	'ㅜ': 'n',
	'ㅡ': 'm',
}

// Package globalsearch implements the Slack-style global search overlay
// opened with `/`. The overlay groups results into sections (channels,
// people, and synthetic destinations) and offers fuzzy matching backed by
// the same ranking style as the Ctrl+T channel finder.
//
// PR1 scope: local categories only (Channels, People, plus pinned
// synthetic destinations such as Threads/Activity). Remote categories
// (Messages, Files) land in a follow-up PR but the Item / section model
// is already category-aware so adding them is additive.
package globalsearch

import (
	"fmt"
	"image/color"
	"sort"
	"strings"
	"unicode/utf8"

	"charm.land/bubbles/v2/textarea"
	tea "charm.land/bubbletea/v2"
	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/text"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/muesli/reflow/truncate"
)

// Section identifiers. The order of CategoryOrder controls render order.
const (
	CategorySynthetic = "synthetic"
	CategoryChannel   = "channel"
	CategoryPerson    = "person"
	CategoryMessage   = "message"
	CategoryFile      = "file"
)

// CategoryOrder is the fixed top-to-bottom rendering order. Sections with
// no matches are skipped in the rendered overlay but their relative
// order is stable across renders.
var CategoryOrder = []string{
	CategorySynthetic,
	CategoryChannel,
	CategoryPerson,
	CategoryMessage,
	CategoryFile,
}

// Sentinel IDs reused from the channel finder so the App can route
// synthetic destinations identically regardless of which overlay
// produced the result.
const (
	ThreadsViewID  = "__slk_view_threads"
	ActivityViewID = "__slk_view_activity"
)

// maxPerSection bounds non-synthetic sections. Synthetic destinations
// (a small fixed set seeded by the App) are not bounded.
const maxPerSection = 5

// nonJoinedColor mirrors the channel finder's dim color so non-joined
// channels read the same across both overlays.
var nonJoinedColor = lipgloss.Color("#5a5a5a")

// Item is one searchable row.
type Item struct {
	// ID is the Slack channel / DM / message / file id used by routing.
	ID string
	// Name is the primary label rendered for the row.
	Name string
	// Subtitle is rendered as a secondary line / suffix on the row
	// when present (e.g. file mime, message author, channel hint).
	Subtitle string
	// Type matches the channel finder's Type vocabulary:
	// channel, private, dm, group_dm, threads, activity, message, file.
	Type string
	// Presence is used only for DM rows: active/away.
	Presence string
	// Joined is true if the user is a member of the channel/DM.
	Joined bool
	// LastVisited drives recency ordering inside a section. Unix seconds.
	LastVisited int64
	// Synthetic pins this row into the synthetic section regardless of
	// Type.
	Synthetic bool

	// Remote-result metadata. Populated for Type=="message" and
	// Type=="file" items so the App can route Enter back to the
	// correct channel / file. Empty for local items.
	ChannelID   string
	ChannelName string
	ChannelType string
	MessageTS   string
	Permalink   string
}

// Category returns the section this item belongs to.
func (it Item) Category() string {
	if it.Synthetic {
		return CategorySynthetic
	}
	switch it.Type {
	case "channel", "private":
		return CategoryChannel
	case "dm", "group_dm", "app":
		return CategoryPerson
	case "message":
		return CategoryMessage
	case "file":
		return CategoryFile
	case "threads", "activity":
		return CategorySynthetic
	}
	return CategoryChannel
}

// Result is returned when the user picks a row with Enter.
type Result struct {
	ID     string
	Name   string
	Type   string
	Joined bool

	// Remote-result metadata. Set when the picked row was a remote
	// message or file hit so the App can navigate to channel + ts
	// (PR3) or open a file URL.
	ChannelID   string
	ChannelName string
	ChannelType string
	MessageTS   string
	Permalink   string
}

// Model is the overlay state.
type Model struct {
	items       []Item
	query       string
	visible     bool
	input       textarea.Model
	title       string
	placeholder string
	scopeLabel  string

	// filter() output:
	sectionItems map[string][]int // category -> indexes into items
	sectionOrder []string         // categories present, in CategoryOrder order
	flat         []int            // selectable indexes in render order
	selected     int              // index into flat
}

// New returns an empty overlay (hidden).
func New() Model {
	input := textarea.New()
	input.CharLimit = 2000
	input.MaxHeight = 8
	input.MinHeight = 1
	input.SetHeight(1)
	input.ShowLineNumbers = false
	input.Prompt = ""
	input.SetWidth(40)
	input.SetVirtualCursor(false)
	input.Placeholder = "Search channels, people, messages…"

	m := Model{
		input:       input,
		title:       "Search",
		placeholder: "Search channels, people, messages…",
	}
	m.RefreshStyles()
	return m
}

func ansiAttrs(bg, fg color.Color) string {
	br, bgc, bb, _ := bg.RGBA()
	fr, fgc, fb, _ := fg.RGBA()
	return fmt.Sprintf("\x1b[48;2;%d;%d;%dm\x1b[38;2;%d;%d;%dm", br>>8, bgc>>8, bb>>8, fr>>8, fgc>>8, fb>>8)
}

func inputBackground() color.Color {
	if styles.ComposeInsertBG != nil {
		return styles.ComposeInsertBG
	}
	if styles.SurfaceDark != nil {
		return styles.SurfaceDark
	}
	return styles.Background
}

func (m *Model) applyInputStyles() {
	bg := lipgloss.NewStyle().Background(inputBackground()).Foreground(styles.TextPrimary)
	s := m.input.Styles()
	s.Focused.Base = bg
	s.Focused.Text = bg
	s.Focused.CursorLine = bg
	s.Focused.EndOfBuffer = bg
	s.Focused.Prompt = bg
	s.Blurred.Base = bg
	s.Blurred.Text = bg
	s.Blurred.CursorLine = bg
	s.Blurred.EndOfBuffer = bg
	s.Blurred.Prompt = bg
	s.Focused.Placeholder = bg.Foreground(styles.TextMuted)
	s.Blurred.Placeholder = bg.Foreground(styles.TextMuted)
	m.input.SetStyles(s)
}

// RefreshStyles reapplies theme-derived textarea styles for the focused
// search input. Call after styles.Apply so IME preedit and typed text
// stay aligned with the active theme.
func (m *Model) RefreshStyles() {
	m.applyInputStyles()
}

// Configure updates the overlay chrome without altering results or visibility.
func (m *Model) Configure(title, placeholder, scopeLabel string) {
	if strings.TrimSpace(title) == "" {
		title = "Search"
	}
	if strings.TrimSpace(placeholder) == "" {
		placeholder = "Search…"
	}
	m.title = title
	m.placeholder = placeholder
	m.scopeLabel = strings.TrimSpace(scopeLabel)
	m.input.Placeholder = placeholder
}

// SetItems replaces the non-synthetic items, preserving any previously
// registered synthetic rows.
func (m *Model) SetItems(items []Item) {
	synth := m.extractSynthetic()
	remote := m.extractRemote()
	m.items = append(synth, items...)
	m.items = append(m.items, remote...)
	if m.visible {
		m.filter()
	}
}

// SetSyntheticItems replaces synthetic rows. Synthetic flag is forced
// to true on every passed item.
func (m *Model) SetSyntheticItems(items []Item) {
	keep := m.items[:0]
	for _, it := range m.items {
		if !it.Synthetic {
			keep = append(keep, it)
		}
	}
	merged := make([]Item, 0, len(items)+len(keep))
	for _, it := range items {
		it.Synthetic = true
		merged = append(merged, it)
	}
	merged = append(merged, keep...)
	m.items = merged
	if m.visible {
		m.filter()
	}
}

// SetBrowseable replaces non-joined channel rows; joined rows and
// synthetic rows are preserved.
func (m *Model) SetBrowseable(browseable []Item) {
	keep := m.items[:0]
	have := make(map[string]struct{}, len(m.items))
	for _, it := range m.items {
		if it.Joined || it.Synthetic {
			keep = append(keep, it)
			have[it.ID] = struct{}{}
		}
	}
	m.items = keep
	for _, it := range browseable {
		if _, dup := have[it.ID]; dup {
			continue
		}
		it.Joined = false
		m.items = append(m.items, it)
	}
	if m.visible {
		m.filter()
	}
}

// MarkJoined flips Joined on the matching item, if present.
func (m *Model) MarkJoined(id string) {
	for i := range m.items {
		if m.items[i].ID == id {
			m.items[i].Joined = true
			return
		}
	}
}

// UpdateLastVisited stamps LastVisited and re-filters if visible.
func (m *Model) UpdateLastVisited(id string, ts int64) {
	for i := range m.items {
		if m.items[i].ID == id {
			m.items[i].LastVisited = ts
			if m.visible {
				m.filter()
			}
			return
		}
	}
}

// SetMessageResults replaces the Message section with `items`, in the
// order provided (server-side ranking is preserved). The call is
// dropped if `forQuery` no longer matches the current query — that
// guard, plus the App-level request-generation guard, protects the
// overlay from stale remote results landing after the user typed
// further.
func (m *Model) SetMessageResults(forQuery string, items []Item) {
	if forQuery != m.query {
		return
	}
	m.replaceCategory(CategoryMessage, items, "message")
	if m.visible {
		m.filter()
	}
}

// SetFileResults mirrors SetMessageResults for the Files section.
func (m *Model) SetFileResults(forQuery string, items []Item) {
	if forQuery != m.query {
		return
	}
	m.replaceCategory(CategoryFile, items, "file")
	if m.visible {
		m.filter()
	}
}

// clearRemote drops Message + File rows. Used on Open/Close and on
// every query mutation to avoid stale rows leaking between queries.
func (m *Model) clearRemote() {
	if len(m.items) == 0 {
		return
	}
	keep := m.items[:0]
	for _, it := range m.items {
		if c := it.Category(); c == CategoryMessage || c == CategoryFile {
			continue
		}
		keep = append(keep, it)
	}
	m.items = keep
}

// replaceCategory swaps out all items currently in `cat` for the
// given new items, forcing each new item's Type so Category() returns
// the right value.
func (m *Model) replaceCategory(cat string, items []Item, forceType string) {
	keep := m.items[:0]
	for _, it := range m.items {
		if it.Category() == cat {
			continue
		}
		keep = append(keep, it)
	}
	m.items = keep
	for _, it := range items {
		if forceType != "" {
			it.Type = forceType
		}
		m.items = append(m.items, it)
	}
}

func (m *Model) extractSynthetic() []Item {
	var synth []Item
	for _, it := range m.items {
		if it.Synthetic {
			synth = append(synth, it)
		}
	}
	return synth
}

func (m *Model) extractRemote() []Item {
	var remote []Item
	for _, it := range m.items {
		if c := it.Category(); c == CategoryMessage || c == CategoryFile {
			remote = append(remote, it)
		}
	}
	return remote
}

// Open shows the overlay and resets state.
func (m *Model) Open() {
	m.visible = true
	m.query = ""
	m.RefreshStyles()
	m.input.SetValue("")
	m.input.Placeholder = m.placeholder
	m.input.Focus()
	m.selected = 0
	m.clearRemote()
	m.filter()
}

// Close hides the overlay.
func (m *Model) Close() {
	m.visible = false
	m.input.Blur()
	// Drop remote results so a debounced Cmd that lands after the
	// overlay closed can't seed stale rows into the next Open().
	m.clearRemote()
}

// IsVisible returns whether the overlay is showing.
func (m Model) IsVisible() bool { return m.visible }

// Query returns the current query text.
func (m Model) Query() string { return m.query }

// SectionLen returns the number of rows currently rendered in the
// given category. Useful for app-level tests that assert remote
// results landed in the right bucket without poking at internals.
func (m Model) SectionLen(cat string) int { return len(m.sectionItems[cat]) }

// HandleKeyMsg is the input entrypoint for real Bubble Tea key events.
// Returns a Result when the user confirms a selection, otherwise nil.
func (m *Model) HandleKeyMsg(msg tea.KeyMsg) (*Result, tea.Cmd) {
	switch msg.Key().Code {
	case tea.KeyEnter:
		if len(m.flat) > 0 && m.selected >= 0 && m.selected < len(m.flat) {
			idx := m.flat[m.selected]
			it := m.items[idx]
			return &Result{
				ID:          it.ID,
				Name:        it.Name,
				Type:        it.Type,
				Joined:      it.Joined,
				ChannelID:   it.ChannelID,
				ChannelName: it.ChannelName,
				ChannelType: it.ChannelType,
				MessageTS:   it.MessageTS,
				Permalink:   it.Permalink,
			}, nil
		}
		return nil, nil
	case tea.KeyEscape:
		m.Close()
		return nil, nil
	case tea.KeyDown:
		if m.selected < len(m.flat)-1 {
			m.selected++
		}
		return nil, nil
	case tea.KeyUp:
		if m.selected > 0 {
			m.selected--
		}
		return nil, nil
	}
	if msg.Key().Mod == tea.ModCtrl {
		switch msg.Key().Code {
		case 'n':
			if m.selected < len(m.flat)-1 {
				m.selected++
			}
			return nil, nil
		case 'p':
			if m.selected > 0 {
				m.selected--
			}
			return nil, nil
		}
	}
	var cmd tea.Cmd
	m.input, cmd = m.input.Update(msg)
	m.syncQueryFromInput()
	return nil, cmd
}

// HandleKey is a small test-friendly wrapper around HandleKeyMsg.
func (m *Model) HandleKey(keyStr string) *Result {
	switch keyStr {
	case "enter":
		res, _ := m.HandleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEnter})
		return res
	case "esc":
		res, _ := m.HandleKeyMsg(tea.KeyPressMsg{Code: tea.KeyEscape})
		return res
	case "down":
		res, _ := m.HandleKeyMsg(tea.KeyPressMsg{Code: tea.KeyDown})
		return res
	case "up":
		res, _ := m.HandleKeyMsg(tea.KeyPressMsg{Code: tea.KeyUp})
		return res
	case "backspace":
		res, _ := m.HandleKeyMsg(tea.KeyPressMsg{Code: tea.KeyBackspace})
		return res
	case "ctrl+n":
		res, _ := m.HandleKeyMsg(tea.KeyPressMsg{Code: 'n', Mod: tea.ModCtrl, Text: "n"})
		return res
	case "ctrl+p":
		res, _ := m.HandleKeyMsg(tea.KeyPressMsg{Code: 'p', Mod: tea.ModCtrl, Text: "p"})
		return res
	}
	if strings.Contains(keyStr, "+") {
		return nil
	}
	if r, sz := utf8.DecodeRuneInString(keyStr); sz == len(keyStr) && r != utf8.RuneError {
		res, _ := m.HandleKeyMsg(tea.KeyPressMsg{Code: r, Text: keyStr})
		return res
	}
	return nil
}

func (m *Model) syncQueryFromInput() {
	q := m.input.Value()
	if q == m.query {
		return
	}
	m.query = q
	m.selected = 0
	m.clearRemote()
	m.filter()
}

type match struct {
	tier  int // 0 prefix, 1 substring, 2 subsequence
	score int // subsequence score; 0 for prefix/substring
}

type candidate struct {
	match
	idx int
}

// filter rebuilds sectioned results from items + query.
//
// Ranking inside each section follows the channel-finder shape:
//  1. Joined first (channel section only — other sections don't carry
//     a meaningful Joined bit)
//  2. Match tier: prefix > substring > subsequence
//  3. LastVisited DESC (recency)
//  4. Subsequence score DESC
//  5. typeRank ASC (group_dm demoted)
//  6. Name ASC (case-insensitive)
func (m *Model) filter() {
	m.sectionItems = map[string][]int{}
	m.sectionOrder = nil
	m.flat = nil

	q := text.Fold(m.query)
	cands := map[string][]candidate{}
	for i, it := range m.items {
		cat := it.Category()
		// Remote rows (Message/File) are pre-ranked by the server and
		// reflect the query they were fetched for. We do not re-run
		// the local fuzzy ranker on them: it would discard hits whose
		// names don't share runes with the query and reorder them in
		// ways the user didn't ask for.
		if cat == CategoryMessage || cat == CategoryFile {
			cands[cat] = append(cands[cat], candidate{idx: i})
			continue
		}
		c, ok := rank(it, q)
		if !ok {
			continue
		}
		cands[cat] = append(cands[cat], candidate{match: c, idx: i})
	}

	for _, cat := range CategoryOrder {
		slice, ok := cands[cat]
		if !ok || len(slice) == 0 {
			continue
		}
		m.sortCandidates(cat, slice)
		if cat != CategorySynthetic && len(slice) > maxPerSection {
			slice = slice[:maxPerSection]
		}
		idxs := make([]int, 0, len(slice))
		for _, c := range slice {
			idxs = append(idxs, c.idx)
		}
		m.sectionItems[cat] = idxs
		m.sectionOrder = append(m.sectionOrder, cat)
		m.flat = append(m.flat, idxs...)
	}

	// Clamp the selection so external mutations (SetItems on
	// workspace switch, SetBrowseable after browseable load,
	// UpdateLastVisited, etc.) can't leave m.selected pointing past
	// the end of the regenerated flat list — pressing Enter in that
	// state would index out of bounds. HandleKey "enter" also has its
	// own guard for belt-and-suspenders safety.
	if m.selected < 0 {
		m.selected = 0
	}
	if m.selected >= len(m.flat) {
		if len(m.flat) == 0 {
			m.selected = 0
		} else {
			m.selected = len(m.flat) - 1
		}
	}
}

func rank(item Item, q string) (match, bool) {
	if q == "" {
		return match{}, true
	}
	name := text.Fold(item.Name)
	switch {
	case strings.HasPrefix(name, q):
		return match{tier: 0}, true
	case strings.Contains(name, q):
		return match{tier: 1}, true
	}
	if score, ok := subsequenceScore(name, q); ok {
		return match{tier: 2, score: score}, true
	}
	return match{}, false
}

func (m *Model) sortCandidates(cat string, slice []candidate) {
	// Remote sections trust the server-side ranking and the order of
	// items passed to SetMessageResults / SetFileResults. The model's
	// own ranker doesn't have signal the server has (recency,
	// relevance, channel context), so we preserve order as-is.
	if cat == CategoryMessage || cat == CategoryFile {
		return
	}
	// Synthetic destinations rank within match tier (so a prefix
	// match — e.g. "thr" → Threads — outranks a subsequence match)
	// and within a tier we preserve registration order via idx so
	// the App can rely on the seeded order ("Threads" before
	// "Activity"). This is intentional: query-aware order is a
	// better UX than strict registration order, and inside the
	// dominant tier the registration order is still honored.
	if cat == CategorySynthetic {
		sort.SliceStable(slice, func(i, j int) bool {
			if slice[i].tier != slice[j].tier {
				return slice[i].tier < slice[j].tier
			}
			return slice[i].idx < slice[j].idx
		})
		return
	}
	sort.SliceStable(slice, func(i, j int) bool {
		a, b := m.items[slice[i].idx], m.items[slice[j].idx]
		if cat == CategoryChannel && a.Joined != b.Joined {
			return a.Joined
		}
		if slice[i].tier != slice[j].tier {
			return slice[i].tier < slice[j].tier
		}
		if a.LastVisited != b.LastVisited {
			return a.LastVisited > b.LastVisited
		}
		if slice[i].score != slice[j].score {
			return slice[i].score > slice[j].score
		}
		if ar, br := typeRank(a), typeRank(b); ar != br {
			return ar < br
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
}

func typeRank(it Item) int {
	if it.Type == "group_dm" {
		return 1
	}
	return 0
}

// subsequenceScore mirrors the channel finder's scorer.
func subsequenceScore(name, q string) (int, bool) {
	if q == "" {
		return 0, true
	}
	score := 0
	qi := 0
	qrunes := []rune(q)
	first, last := -1, -1
	prevWasSep := true
	for i, r := range name {
		if qi >= len(qrunes) {
			break
		}
		if r == qrunes[qi] {
			if first < 0 {
				first = i
			}
			last = i
			score += 10
			if prevWasSep {
				score += 25
			}
			qi++
		}
		prevWasSep = isSeparator(r)
	}
	if qi < len(qrunes) {
		return 0, false
	}
	span := last - first + 1
	if span > 0 {
		score += 50 * len(qrunes) / span
	}
	return score, true
}

func isSeparator(r rune) bool {
	switch r {
	case '-', '_', '.', ' ', '/', ':':
		return true
	}
	return false
}

// View returns the overlay box only.
func (m *Model) View(termWidth int) string {
	return m.renderBox(termWidth)
}

// ViewOverlay composites the centered modal over a dimmed backdrop.
func (m *Model) ViewOverlay(termWidth, termHeight int, background string) string {
	if !m.visible {
		return background
	}
	box := m.renderBox(termWidth)
	if box == "" {
		return background
	}
	return overlay.DimmedOverlay(termWidth, termHeight, background, box, 0.5)
}

// Cursor returns the absolute terminal position where the input
// cursor should be drawn while the overlay is visible. macOS / Linux
// IMEs anchor their pre-edit ("composition") rectangle to that
// terminal cursor, so without this method Korean / CJK input was
// either invisible (cursor pointed elsewhere on screen) or rendered
// behind the modal. Returns nil when the overlay is hidden.
func (m *Model) Cursor(termWidth, termHeight int) *tea.Cursor {
	if !m.visible {
		return nil
	}
	box := m.renderBox(termWidth)
	if box == "" {
		return nil
	}
	c := m.input.Cursor()
	if c == nil {
		return nil
	}
	modalW := lipgloss.Width(box)
	modalH := lipgloss.Height(box)
	startX := (termWidth - modalW) / 2
	startY := (termHeight - modalH) / 2
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}
	inputRow := 1
	if m.scopeLabel != "" {
		inputRow++
	}
	c.Position.X += startX + 4
	c.Position.Y += startY + 2 + inputRow
	return c
}

func sectionLabel(cat string) string {
	switch cat {
	case CategorySynthetic:
		return "Views"
	case CategoryChannel:
		return "Channels"
	case CategoryPerson:
		return "People"
	case CategoryMessage:
		return "Messages"
	case CategoryFile:
		return "Files"
	}
	return cat
}

func (m *Model) renderBox(termWidth int) string {
	if !m.visible {
		return ""
	}

	overlayWidth := termWidth / 2
	if overlayWidth < 36 {
		overlayWidth = 36
	}
	if overlayWidth > 90 {
		overlayWidth = 90
	}
	innerWidth := overlayWidth - 4
	inputRenderWidth := innerWidth - 1
	inputContentWidth := inputRenderWidth - 1
	if inputContentWidth < 8 {
		inputContentWidth = 8
	}
	m.input.SetWidth(inputContentWidth)

	bg := styles.Background
	inputBG := inputBackground()

	title := lipgloss.NewStyle().
		Bold(true).
		Background(bg).
		Foreground(styles.Primary).
		Render(m.title)

	var header []string
	header = append(header, title)
	if m.scopeLabel != "" {
		chip := lipgloss.NewStyle().
			Background(styles.SurfaceDark).
			Foreground(styles.TextMuted).
			Padding(0, 1).
			Render(m.scopeLabel)
		header = append(header, chip)
	}
	inputAttrs := ansiAttrs(inputBG, styles.TextPrimary)
	inputView := messages.ReapplyBgAfterResets(m.input.View(), inputAttrs)
	inputBody := lipgloss.NewStyle().
		Background(inputBG).
		Foreground(styles.TextPrimary).
		Width(inputContentWidth).
		Render(inputView)
	input := lipgloss.NewStyle().
		BorderStyle(lipgloss.Border{Left: "▌"}).
		BorderLeft(true).
		BorderForeground(styles.Primary).
		BorderBackground(inputBG).
		PaddingLeft(1).
		Background(inputBG).
		Foreground(styles.TextPrimary).
		Width(inputRenderWidth).
		Render(inputBody)
	header = append(header, input)

	contentWidth := innerWidth - 1 // leading indicator column

	var rows []string
	flatPos := 0 // mirror m.flat index for selection highlighting
	for _, cat := range m.sectionOrder {
		idxs := m.sectionItems[cat]
		if len(idxs) == 0 {
			continue
		}
		header := lipgloss.NewStyle().
			Background(bg).
			Foreground(styles.TextMuted).
			Bold(true).
			Render(sectionLabel(cat))
		rows = append(rows, " "+header)
		for _, idx := range idxs {
			item := m.items[idx]
			isSelected := flatPos == m.selected

			var prefix, name string
			if item.Joined || cat == CategorySynthetic || cat == CategoryPerson {
				prefix = itemPrefix(item)
				nameStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)
				if isSelected {
					nameStyle = nameStyle.Background(bg).Foreground(styles.Primary).Bold(true)
				}
				name = nameStyle.Render(item.Name)
			} else {
				dim := lipgloss.NewStyle().Background(bg).Foreground(nonJoinedColor)
				prefix = dim.Render("#")
				name = dim.Render(item.Name)
			}

			line := prefix + " " + name
			if item.Subtitle != "" {
				sub := lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Render("  " + item.Subtitle)
				line += sub
			}
			if lipgloss.Width(line) > contentWidth {
				line = truncate.StringWithTail(line, uint(contentWidth), "…")
			}
			if pad := contentWidth - lipgloss.Width(line); pad > 0 {
				line += strings.Repeat(" ", pad)
			}

			var row string
			if isSelected {
				indicator := lipgloss.NewStyle().Background(bg).Foreground(styles.Accent).Render("▌")
				row = indicator + line
			} else {
				row = " " + line
			}
			rows = append(rows, row)
			flatPos++
		}
	}

	if len(m.flat) == 0 {
		var label string
		if m.query == "" {
			label = "Type to search…"
		} else {
			label = "No results"
		}
		rows = append(rows, lipgloss.NewStyle().
			Background(bg).
			Foreground(styles.TextMuted).
			Italic(true).
			Render(label))
	}

	content := strings.Join(header, "\n") + "\n\n" + strings.Join(rows, "\n")
	content = messages.ReapplyBgAfterResets(content, messages.BgANSI()+messages.FgANSI())

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(styles.Primary).
		BorderBackground(bg).
		Background(bg).
		Padding(1, 1).
		Width(overlayWidth).
		Render(content)
}

func itemPrefix(item Item) string {
	switch item.Type {
	case "threads":
		return lipgloss.NewStyle().Foreground(styles.Accent).Render("⚑")
	case "activity":
		return lipgloss.NewStyle().Foreground(styles.Accent).Render("◆")
	case "private":
		return lipgloss.NewStyle().Foreground(styles.Warning).Render("◆")
	case "dm":
		if item.Presence == "active" {
			return lipgloss.NewStyle().Foreground(styles.Accent).Render("●")
		}
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("○")
	case "group_dm":
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("●")
	case "app":
		return lipgloss.NewStyle().Foreground(styles.Accent).Render("⌬")
	case "message":
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("✉")
	case "file":
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("📄")
	default:
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("#")
	}
}

// Package newconvopicker implements the "new conversation" overlay
// (triggered by `n` in normal mode). It is a sibling of
// channelfinder: both render the same kind of modal list + query box,
// but newconvopicker mixes workspace users into the result set and
// supports building a group DM by chipping multiple users before
// submission. See ../channelfinder/model.go for the simpler
// channel-switch flow.
//
// Drift risk note: the ranking algorithm (prefix > substring >
// subsequence, with word-boundary bonus) is duplicated from
// channelfinder. If channelfinder's filter behavior changes, this
// package's filter must be updated in lockstep.
package newconvopicker

import (
	"image/color"
	"sort"
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/text"
	"github.com/gammons/slk/internal/ui/channelfinder"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/overlay"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/muesli/reflow/truncate"
)

// Kind tags an Item as a channel/dm row or as a workspace-user row.
type Kind int

const (
	KindChannel Kind = iota
	KindUser
)

// Item is a single picker row. Channels and users live in the same
// slice; Kind disambiguates the union. Field ownership:
//   - KindChannel: ID = channelID, Type ∈ {channel, private, dm,
//     group_dm}, Presence (for dm), Joined.
//   - KindUser:    ID = userID, Username (handle), IsExternal, Presence
//     (active/away/""), DMChannelID (set when an existing 1:1 DM with
//     this user is known; lets Enter route through Result.Channel
//     instead of the conversations.open path).
type Item struct {
	ID          string
	Kind        Kind
	Name        string
	Username    string // user-only (handle without @)
	Type        string // channel-only: channel | private | dm | group_dm
	Presence    string // dm/user only
	Joined      bool   // channel-only
	IsExternal  bool   // user-only
	DMChannelID string // user-only: existing 1:1 DM channel ID if known
	LastVisited int64
}

// SelectedUser is a chip in the input row.
type SelectedUser struct {
	ID   string
	Name string
}

// Result is the outcome of a single Enter / Ctrl+Enter press that
// reaches a decision. Exactly one of Channel / Users is populated.
//
// Channel reuses channelfinder.ChannelResult so the App can route this
// branch through the same code path as the ctrl+t finder (join /
// switch). DMUserID is set non-empty only when the user picked a
// known-user row that short-circuits to an existing 1:1 DM; the App
// uses it to re-upsert the sidebar row with the proper DMUserID
// mapping in case the original bootstrap missed it.
type Result struct {
	Channel   *channelfinder.ChannelResult
	DMUserID  string
	Users     []SelectedUser
}

// Model holds the picker's state. Construct with New().
type Model struct {
	channels []Item
	users    []Item

	chips []SelectedUser

	filtered []int // indices into a freshly merged slice; see filter()
	merged   []Item

	query    string
	selected int // index into filtered; -1 when query is empty (no auto-highlight)
	visible  bool
}

// New constructs an empty picker.
func New() Model {
	return Model{selected: -1}
}

// SetChannels replaces the channel rows.
func (m *Model) SetChannels(items []Item) {
	m.channels = items
	if m.visible {
		m.filter()
	}
}

// SetUsers replaces the user rows. Items with Kind != KindUser are
// silently coerced.
func (m *Model) SetUsers(items []Item) {
	out := make([]Item, len(items))
	for i, it := range items {
		it.Kind = KindUser
		out[i] = it
	}
	m.users = out
	if m.visible {
		m.filter()
	}
}

// PatchUserName updates the display Name of a user row when a late
// users.info resolution arrives. No-op for unknown userIDs.
func (m *Model) PatchUserName(userID, name string) {
	for i := range m.users {
		if m.users[i].ID == userID {
			m.users[i].Name = name
			if m.visible {
				m.filter()
			}
			return
		}
	}
}

// PatchUserDMChannelID wires (or updates) an existing 1:1 DM channel
// ID onto the matching user row. Used after a fresh conversations.open
// completes so future Enter on this user short-circuits to a channel
// switch instead of re-calling the API. No-op for unknown userIDs.
func (m *Model) PatchUserDMChannelID(userID, channelID string) {
	for i := range m.users {
		if m.users[i].ID == userID {
			m.users[i].DMChannelID = channelID
			if m.visible {
				m.filter()
			}
			return
		}
	}
}

// UpsertChannel inserts-or-replaces a channel row by ID.
func (m *Model) UpsertChannel(it Item) {
	it.Kind = KindChannel
	for i := range m.channels {
		if m.channels[i].ID == it.ID {
			m.channels[i] = it
			if m.visible {
				m.filter()
			}
			return
		}
	}
	m.channels = append(m.channels, it)
	if m.visible {
		m.filter()
	}
}

// Open shows the overlay and clears transient state.
func (m *Model) Open() {
	m.visible = true
	m.query = ""
	m.chips = nil
	m.selected = -1
	m.filter()
}

// Close hides the overlay.
func (m *Model) Close() {
	m.visible = false
}

// IsVisible reports whether the overlay is showing.
func (m Model) IsVisible() bool { return m.visible }

// Chips returns a defensive copy of the current chip list (for tests).
func (m Model) Chips() []SelectedUser {
	out := make([]SelectedUser, len(m.chips))
	copy(out, m.chips)
	return out
}

// Query returns the current query string (for tests).
func (m Model) Query() string { return m.query }

// HandleKey processes a key press. Returns a non-nil Result only when
// the user committed to a destination (channel switch or DM/mpim open).
//
// Key semantics (see plan for rationale):
//   - enter, query != "": pick the highlighted row. User → chip, clear
//     query. Channel → return ChannelResult, but only if no chips
//     (silent ignore otherwise so a half-built group DM doesn't get
//     hijacked by a stray channel match).
//   - enter, query == "":
//     submit chips if any; otherwise no-op. Empty query has NO
//     auto-highlight, so this can't accidentally pick a row.
//   - ctrl+enter: always submit chips (bonus shortcut; some terminals
//     can't distinguish it from plain enter, hence the empty-query
//     submit fallback).
//   - backspace: rune-aware. Query has chars → drop one rune. Query
//     empty + chips non-empty → drop last chip.
//   - esc: close.
//   - printable rune: append to query, reset selected to 0, re-filter.
func (m *Model) HandleKey(keyStr string) *Result {
	switch keyStr {
	case "enter":
		return m.commitEnter()
	case "ctrl+enter":
		if len(m.chips) > 0 {
			out := append([]SelectedUser(nil), m.chips...)
			return &Result{Users: out}
		}
		return nil
	case "esc":
		m.Close()
		return nil
	case "down", "ctrl+n":
		if m.selected < 0 && len(m.filtered) > 0 {
			m.selected = 0
		} else if m.selected < len(m.filtered)-1 {
			m.selected++
		}
		return nil
	case "up", "ctrl+p":
		if m.selected > 0 {
			m.selected--
		}
		return nil
	case "backspace":
		runes := []rune(m.query)
		if len(runes) > 0 {
			m.query = string(runes[:len(runes)-1])
			if m.query == "" {
				m.selected = -1
			} else {
				m.selected = 0
			}
			m.filter()
			return nil
		}
		if len(m.chips) > 0 {
			m.chips = m.chips[:len(m.chips)-1]
			m.filter()
		}
		return nil
	}

	// Printable runes go into the query.
	if isPrintableKey(keyStr) {
		m.query += keyStr
		m.selected = 0
		m.filter()
	}
	return nil
}

func isPrintableKey(s string) bool {
	// HandleKey only forwards single-keystroke strings here. We accept
	// runes that occupy a single rune in s. Bubbletea key strings for
	// printable text are exactly the typed rune (e.g. "a", "한").
	if s == "" {
		return false
	}
	runes := []rune(s)
	if len(runes) != 1 {
		return false
	}
	r := runes[0]
	if r < 0x20 || r == 0x7f { // control chars
		return false
	}
	return true
}

// commitEnter implements the enter-key state machine. Split out so the
// branching reads top-to-bottom rather than nested-switch.
func (m *Model) commitEnter() *Result {
	if m.query == "" {
		if len(m.chips) > 0 {
			// Submission. If exactly one chip and we already know its
			// 1:1 DM channel, short-circuit to a channel switch
			// (saves a conversations.open round-trip; Slack would
			// return the same channel anyway, but skipping the network
			// call gives instant feedback).
			if len(m.chips) == 1 {
				if dm := m.dmChannelForUser(m.chips[0].ID); dm != "" {
					name := m.chips[0].Name
					id := m.chips[0].ID
					m.chips = nil
					return &Result{
						Channel: &channelfinder.ChannelResult{
							ID:     dm,
							Name:   name,
							Type:   "dm",
							Joined: true,
						},
						DMUserID: id,
					}
				}
			}
			out := append([]SelectedUser(nil), m.chips...)
			return &Result{Users: out}
		}
		return nil
	}
	if m.selected < 0 || m.selected >= len(m.filtered) {
		return nil
	}
	it := m.merged[m.filtered[m.selected]]
	switch it.Kind {
	case KindUser:
		// Always chip on user Enter — even if a 1:1 DM already exists
		// for this user. Building a group DM out of two users who each
		// have an existing 1:1 DM requires the first chip to NOT
		// short-circuit. Submission (empty-query Enter) handles the
		// existing-DM optimization centrally.
		if !m.isChipped(it.ID) {
			m.chips = append(m.chips, SelectedUser{ID: it.ID, Name: it.Name})
		}
		m.query = ""
		m.selected = -1
		m.filter()
		return nil
	case KindChannel:
		if len(m.chips) > 0 {
			// Silent ignore — building a group DM, channels not allowed.
			return nil
		}
		return &Result{Channel: &channelfinder.ChannelResult{
			ID:     it.ID,
			Name:   it.Name,
			Type:   it.Type,
			Joined: it.Joined,
		}}
	}
	return nil
}

func (m *Model) isChipped(userID string) bool {
	for _, c := range m.chips {
		if c.ID == userID {
			return true
		}
	}
	return false
}

// dmChannelForUser returns the cached 1:1 DM channel ID for the given
// userID, or "" when none is known. Used by the submission path to
// short-circuit single-user submissions when we already have the DM.
func (m *Model) dmChannelForUser(userID string) string {
	for _, u := range m.users {
		if u.ID == userID {
			return u.DMChannelID
		}
	}
	return ""
}

// filter rebuilds merged + filtered for the current query.
//
// merged is the union of m.channels and m.users with two suppressions:
//   - users whose ID is already in chips are dropped.
//   - dm channel rows are dropped when a matching user row carries the
//     same DMChannelID (we render a single "person row" instead of a
//     row pair).
//
// filtered then applies the prefix > substring > subsequence ranking
// from channelfinder (q == "" → recency order, no row gets highlighted).
func (m *Model) filter() {
	merged := make([]Item, 0, len(m.channels)+len(m.users))

	// Index DMChannelIDs claimed by user rows so we can suppress the
	// matching dm channel rows.
	suppressedDMs := make(map[string]struct{})
	for _, u := range m.users {
		if u.DMChannelID != "" {
			suppressedDMs[u.DMChannelID] = struct{}{}
		}
	}

	for _, c := range m.channels {
		c.Kind = KindChannel
		if c.Type == "dm" {
			if _, dup := suppressedDMs[c.ID]; dup {
				continue
			}
		}
		merged = append(merged, c)
	}
	for _, u := range m.users {
		u.Kind = KindUser
		if m.isChipped(u.ID) {
			continue
		}
		merged = append(merged, u)
	}
	m.merged = merged

	q := text.Fold(m.query)
	if q == "" {
		idxs := make([]int, len(merged))
		for i := range merged {
			idxs[i] = i
		}
		sort.SliceStable(idxs, func(i, j int) bool {
			return lessNoQuery(merged[idxs[i]], merged[idxs[j]])
		})
		m.filtered = idxs
		// Clamp selected: empty query stays at -1 (no auto-highlight).
		if m.query == "" {
			m.selected = -1
		}
		return
	}

	type match struct {
		idx   int
		tier  int
		score int
	}
	var matches []match
	for i, it := range merged {
		hay := text.Fold(it.Name)
		hay2 := text.Fold(it.Username)
		switch {
		case strings.HasPrefix(hay, q) || (hay2 != "" && strings.HasPrefix(hay2, q)):
			matches = append(matches, match{idx: i, tier: 0})
		case strings.Contains(hay, q) || (hay2 != "" && strings.Contains(hay2, q)):
			matches = append(matches, match{idx: i, tier: 1})
		default:
			if score, ok := subsequenceScore(hay, q); ok {
				matches = append(matches, match{idx: i, tier: 2, score: score})
			}
		}
	}
	sort.SliceStable(matches, func(i, j int) bool {
		a := merged[matches[i].idx]
		b := merged[matches[j].idx]
		if matches[i].tier != matches[j].tier {
			return matches[i].tier < matches[j].tier
		}
		// Joined channels and users-with-existing-DM rank above non-joined / strangers.
		ar, br := readiness(a), readiness(b)
		if ar != br {
			return ar > br
		}
		if a.LastVisited != b.LastVisited {
			return a.LastVisited > b.LastVisited
		}
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return strings.ToLower(a.Name) < strings.ToLower(b.Name)
	})
	m.filtered = make([]int, len(matches))
	for i, mm := range matches {
		m.filtered[i] = mm.idx
	}
	if len(m.filtered) == 0 {
		m.selected = -1
	} else if m.selected < 0 || m.selected >= len(m.filtered) {
		m.selected = 0
	}
}

// readiness boosts items that the user is already plugged into: joined
// channels and users with an existing DM. Used as a secondary sort key
// inside each match tier.
func readiness(it Item) int {
	if it.Kind == KindChannel && it.Joined {
		return 1
	}
	if it.Kind == KindUser && it.DMChannelID != "" {
		return 1
	}
	return 0
}

func lessNoQuery(a, b Item) bool {
	ar, br := readiness(a), readiness(b)
	if ar != br {
		return ar > br
	}
	if a.LastVisited != b.LastVisited {
		return a.LastVisited > b.LastVisited
	}
	return strings.ToLower(a.Name) < strings.ToLower(b.Name)
}

// subsequenceScore is duplicated from channelfinder (see drift note at
// the top of this file). Returns score+true when every rune of q
// appears in name in order; rewards word-boundary hits and tightness.
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

// ViewOverlay renders the picker as a centered modal with a dimmed
// backdrop. Width/height follow the channelfinder modal so the two
// look consistent.
func (m Model) ViewOverlay(termWidth, termHeight int, background string) string {
	if !m.visible {
		return background
	}
	box := m.renderBox(termWidth)
	if box == "" {
		return background
	}
	return overlay.DimmedOverlay(termWidth, termHeight, background, box, 0.5)
}

func (m Model) renderBox(termWidth int) string {
	if !m.visible {
		return ""
	}
	overlayWidth := termWidth / 2
	if overlayWidth < 40 {
		overlayWidth = 40
	}
	if overlayWidth > 80 {
		overlayWidth = 80
	}
	innerWidth := overlayWidth - 4
	bg := styles.Background

	title := lipgloss.NewStyle().
		Bold(true).
		Background(bg).
		Foreground(styles.Primary).
		Render("New message")

	// Build the input line: chips + query.
	inputContent := renderChipsAndQuery(m.chips, m.query, bg)
	input := lipgloss.NewStyle().
		BorderStyle(lipgloss.Border{Left: "▌"}).
		BorderLeft(true).
		BorderForeground(styles.Primary).
		BorderBackground(bg).
		PaddingLeft(1).
		Background(bg).
		Foreground(styles.TextPrimary).
		Render(inputContent)

	// Result rows.
	maxVisible := 10
	total := len(m.filtered)
	if maxVisible > total {
		maxVisible = total
	}
	startIdx := 0
	if m.selected >= maxVisible {
		startIdx = m.selected - maxVisible + 1
	}
	endIdx := startIdx + maxVisible
	if endIdx > total {
		endIdx = total
		startIdx = endIdx - maxVisible
		if startIdx < 0 {
			startIdx = 0
		}
	}
	showScrollbar := total > maxVisible
	contentWidth := innerWidth - 1
	if showScrollbar {
		contentWidth--
	}

	var thumbStart, thumbEnd int
	if showScrollbar {
		thumbHeight := maxVisible * maxVisible / total
		if thumbHeight < 1 {
			thumbHeight = 1
		}
		denom := total - maxVisible
		if denom < 1 {
			denom = 1
		}
		thumbStart = startIdx * (maxVisible - thumbHeight) / denom
		if thumbStart < 0 {
			thumbStart = 0
		}
		if thumbStart > maxVisible-thumbHeight {
			thumbStart = maxVisible - thumbHeight
		}
		thumbEnd = thumbStart + thumbHeight
	}
	thumbStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.Primary)
	trackStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.Border)

	var rows []string
	for i := startIdx; i < endIdx; i++ {
		idx := m.filtered[i]
		it := m.merged[idx]
		isSel := i == m.selected
		row := renderRow(it, isSel, contentWidth, bg)
		if showScrollbar {
			rel := i - startIdx
			if rel >= thumbStart && rel < thumbEnd {
				row += thumbStyle.Render("█")
			} else {
				row += trackStyle.Render("│")
			}
		}
		rows = append(rows, row)
	}
	if len(m.filtered) == 0 && m.query != "" {
		noResults := lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Italic(true).
			Render("No matches")
		rows = append(rows, noResults)
	}

	content := title + "\n" + input + "\n\n" + strings.Join(rows, "\n")
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

func renderChipsAndQuery(chips []SelectedUser, query string, bg color.Color) string {
	if len(chips) == 0 && query == "" {
		placeholder := lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).
			Render("#a-channel, @somebody")
		return "█ " + placeholder
	}
	var b strings.Builder
	chipStyle := lipgloss.NewStyle().Background(styles.Primary).Foreground(styles.Background).Padding(0, 1)
	sep := lipgloss.NewStyle().Background(bg).Render(" ")
	for i, c := range chips {
		if i > 0 {
			b.WriteString(sep)
		}
		b.WriteString(chipStyle.Render("@" + c.Name))
	}
	if len(chips) > 0 && query != "" {
		b.WriteString(sep)
	}
	if query != "" {
		b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary).Render(query))
	}
	// Cursor at the end.
	b.WriteString(lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary).Render("█"))
	return b.String()
}

func renderRow(it Item, selected bool, contentWidth int, bg color.Color) string {
	prefix := rowPrefix(it)

	nameStyle := lipgloss.NewStyle().Background(bg).Foreground(styles.TextPrimary)
	if selected {
		nameStyle = nameStyle.Background(bg).Foreground(styles.Primary).Bold(true)
	}
	name := nameStyle.Render(it.Name)

	suffix := ""
	if it.Kind == KindUser && it.IsExternal {
		suffix = lipgloss.NewStyle().Background(bg).Foreground(styles.TextMuted).Render(" (ext)")
	}

	line := prefix + " " + name + suffix
	if lipgloss.Width(line) > contentWidth {
		line = truncate.StringWithTail(line, uint(contentWidth), "…")
	}
	if pad := contentWidth - lipgloss.Width(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}

	if selected {
		indicator := lipgloss.NewStyle().Background(bg).Foreground(styles.Accent).Render("▌")
		return indicator + line
	}
	return " " + line
}

func rowPrefix(it Item) string {
	if it.Kind == KindUser {
		switch it.Presence {
		case "active":
			return lipgloss.NewStyle().Foreground(styles.Accent).Render("●")
		default:
			return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("○")
		}
	}
	switch it.Type {
	case "private":
		return lipgloss.NewStyle().Foreground(styles.Warning).Render("◆")
	case "dm":
		if it.Presence == "active" {
			return lipgloss.NewStyle().Foreground(styles.Accent).Render("●")
		}
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("○")
	case "group_dm":
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("●")
	default:
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("#")
	}
}

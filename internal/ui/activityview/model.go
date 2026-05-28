package activityview

import (
	"strconv"
	"strings"
	"time"

	"charm.land/lipgloss/v2"
	"github.com/gammons/slk/internal/cache"
	"github.com/gammons/slk/internal/ui/messages"
	"github.com/gammons/slk/internal/ui/styles"
	"github.com/muesli/reflow/truncate"
)

const (
	cardContentLines = 3
	cardStride       = cardContentLines + 1
)

func mutedStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.TextMuted)
}

func unreadDotStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
}

func unreadBadgeStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Background).Background(styles.Primary).Bold(true)
}

func channelNameStyle() lipgloss.Style {
	return lipgloss.NewStyle().Foreground(styles.Primary).Bold(true)
}

func actorNameStyle() lipgloss.Style {
	return lipgloss.NewStyle().Bold(true)
}

var thickLeftBorder = lipgloss.Border{Left: "▌"}

func borderInvisStyle() lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(thickLeftBorder).BorderLeft(true).
		BorderForeground(styles.Background).
		BorderBackground(styles.Background)
}

func borderSelectStyle(focused bool) lipgloss.Style {
	return lipgloss.NewStyle().
		BorderStyle(thickLeftBorder).BorderLeft(true).
		BorderForeground(styles.SelectionBorderColor(focused)).
		BorderBackground(styles.SelectionTintColor(focused)).
		Background(styles.SelectionTintColor(focused))
}

func borderFillStyle() lipgloss.Style {
	return lipgloss.NewStyle().Background(styles.Background)
}

type Model struct {
	items            []cache.ActivityItem
	userNames        map[string]string
	channelNames     map[string]string
	selfUserID       string
	selected         int
	focused          bool
	yOffset          int
	snappedSelection int
	hasSnapped       bool
	// loading is true between construction (or workspace switch) and
	// the first SetItems call. While true the empty-state placeholder
	// is replaced with a "Loading…" indicator so the panel never
	// flashes "no activity" before the activity-list fetcher has had
	// a chance to populate it. Default true so a freshly-constructed
	// Model defaults to the loading state.
	loading bool
	version int64
}

func New(userNames map[string]string, selfUserID string) Model {
	if userNames == nil {
		userNames = map[string]string{}
	}
	return Model{
		userNames:    userNames,
		channelNames: map[string]string{},
		selfUserID:   selfUserID,
	}
}

func (m *Model) Version() int64 { return m.version }

func (m *Model) dirty() { m.version++ }

func (m *Model) SetItems(items []cache.ActivityItem) {
	prevCh, prevTS, hadSel := m.selectedKey()
	m.items = items
	newSel := 0
	if hadSel {
		for i, item := range items {
			if item.ChannelID == prevCh && item.TS == prevTS {
				newSel = i
				break
			}
		}
	}
	m.selected = newSel
	m.clampSelection()
	m.hasSnapped = false
	// Any SetItems call ends the bootstrap loading state: empty result
	// now legitimately means "no activity" and we want the user-facing
	// empty placeholder, not a forever-spinner.
	m.loading = false
	m.dirty()
}

// SetLoading marks the activity panel as still waiting for its
// first data load. Setting true is only meaningful before the first
// SetItems call (or after a workspace switch that needs to suppress
// the previous workspace's empty/non-empty state until fresh data
// arrives). Setting false is identical to SetItems(nil) except it
// doesn't reset the selection.
func (m *Model) SetLoading(loading bool) {
	if m.loading == loading {
		return
	}
	m.loading = loading
	m.dirty()
}

func (m *Model) SetUserNames(names map[string]string) {
	if names == nil {
		names = map[string]string{}
	}
	if stringMapsEqual(m.userNames, names) {
		return
	}
	m.userNames = names
	m.dirty()
}

func (m *Model) SetChannelNames(names map[string]string) {
	if names == nil {
		names = map[string]string{}
	}
	if stringMapsEqual(m.channelNames, names) {
		return
	}
	m.channelNames = names
	m.dirty()
}

func (m *Model) SetSelfUserID(id string) {
	if m.selfUserID == id {
		return
	}
	m.selfUserID = id
	m.dirty()
}

func (m *Model) SetFocused(f bool) {
	if m.focused == f {
		return
	}
	m.focused = f
	m.dirty()
}

func (m *Model) SelectedItem() (cache.ActivityItem, bool) {
	if len(m.items) == 0 || m.selected < 0 || m.selected >= len(m.items) {
		return cache.ActivityItem{}, false
	}
	return m.items[m.selected], true
}

func (m *Model) selectedKey() (string, string, bool) {
	item, ok := m.SelectedItem()
	if !ok {
		return "", "", false
	}
	return item.ChannelID, item.TS, true
}

func (m *Model) SelectedIndex() int { return m.selected }

func (m *Model) MoveDown() {
	if m.selected < len(m.items)-1 {
		m.selected++
		m.dirty()
	}
}

func (m *Model) MoveUp() {
	if m.selected > 0 {
		m.selected--
		m.dirty()
	}
}

func (m *Model) GoToTop() {
	if m.selected != 0 {
		m.selected = 0
		m.dirty()
	}
}

func (m *Model) GoToBottom() {
	if n := len(m.items); n > 0 && m.selected != n-1 {
		m.selected = n - 1
		m.dirty()
	}
}

func (m *Model) ScrollUp(n int) {
	if n <= 0 {
		return
	}
	m.yOffset -= n
	if m.yOffset < 0 {
		m.yOffset = 0
	}
	m.hasSnapped = false
	m.dirty()
}

func (m *Model) ScrollDown(n int) {
	if n <= 0 {
		return
	}
	m.yOffset += n
	m.hasSnapped = false
	m.dirty()
}

func (m *Model) ClickAt(rowY int) bool {
	if rowY < 0 {
		return false
	}
	absLine := m.yOffset + rowY
	if absLine < 0 {
		return false
	}
	if absLine%cardStride >= cardContentLines {
		return false
	}
	idx := absLine / cardStride
	if idx < 0 || idx >= len(m.items) {
		return false
	}
	if m.selected != idx {
		m.selected = idx
		m.dirty()
	}
	return true
}

func (m *Model) UnreadCount() int {
	n := 0
	for _, item := range m.items {
		if item.Unread {
			n++
		}
	}
	return n
}

func (m *Model) clampSelection() {
	if m.selected < 0 {
		m.selected = 0
	}
	if n := len(m.items); n == 0 {
		m.selected = 0
	} else if m.selected >= n {
		m.selected = n - 1
	}
}

func (m *Model) View(height, width int) string {
	if width < 1 {
		width = 1
	}
	if height < 1 {
		height = 1
	}
	if len(m.items) == 0 {
		text := "no activity"
		if m.loading {
			text = "⏳  Loading activity…"
		}
		empty := mutedStyle().Render(text)
		return lipgloss.Place(width, height, lipgloss.Center, lipgloss.Center, empty)
	}
	lines := m.renderRows(width)
	if !m.hasSnapped || m.snappedSelection != m.selected {
		m.snapToSelected(height, len(lines))
		m.snappedSelection = m.selected
		m.hasSnapped = true
	}
	maxOffset := len(lines) - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.yOffset > maxOffset {
		m.yOffset = maxOffset
	}
	if m.yOffset < 0 {
		m.yOffset = 0
	}
	end := m.yOffset + height
	if end > len(lines) {
		end = len(lines)
	}
	visible := lines[m.yOffset:end]
	if pad := height - len(visible); pad > 0 {
		filler := blankLine(width)
		out := make([]string, 0, height)
		out = append(out, visible...)
		for i := 0; i < pad; i++ {
			out = append(out, filler)
		}
		visible = out
	}
	return strings.Join(visible, "\n")
}

func (m *Model) snapToSelected(height, totalLines int) {
	start := m.selected * cardStride
	end := start + cardContentLines
	if end > m.yOffset+height {
		m.yOffset = end - height
	}
	if start < m.yOffset {
		m.yOffset = start
	}
	if m.yOffset < 0 {
		m.yOffset = 0
	}
	maxOffset := totalLines - height
	if maxOffset < 0 {
		maxOffset = 0
	}
	if m.yOffset > maxOffset {
		m.yOffset = maxOffset
	}
}

func (m *Model) renderRows(width int) []string {
	separator := blankLine(width)
	var lines []string
	for i, item := range m.items {
		if i > 0 {
			lines = append(lines, separator)
		}
		lines = append(lines, m.renderCard(item, width, i == m.selected)...)
	}
	return lines
}

func blankLine(width int) string {
	return lipgloss.NewStyle().Width(width).Render("")
}

func (m *Model) renderCard(item cache.ActivityItem, width int, selected bool) []string {
	contentWidth := width - 1
	if contentWidth < 1 {
		contentWidth = 1
	}

	header := m.renderHeader(item, contentWidth)
	context := m.renderContext(item, contentWidth)
	preview := m.renderPreview(item, contentWidth)

	borderStyle := borderInvisStyle()
	fill := borderFillStyle().Width(contentWidth)
	if selected {
		borderStyle = borderSelectStyle(m.focused)
		fill = lipgloss.NewStyle().Background(styles.SelectionTintColor(m.focused)).Width(contentWidth)
	}

	headerOut := borderStyle.Render(fill.Render(header))
	contextOut := borderStyle.Render(fill.Render(context))
	previewOut := borderStyle.Render(fill.Foreground(styles.TextMuted).Render(preview))
	return []string{headerOut, contextOut, previewOut}
}

func (m *Model) renderHeader(item cache.ActivityItem, width int) string {
	actor := m.resolveUser(item.UserID)
	if actor == "" {
		if item.ChannelType == "app" && item.ChannelName != "" {
			actor = item.ChannelName
		} else {
			actor = kindLabel(item.Kind)
		}
	}
	left := actorNameStyle().Render(actor)
	right := mutedStyle().Render(formatActivityTime(item.TS))
	if item.Unread {
		right = right + "  " + unreadBadgeStyle().Render(" 1 ")
	}
	return joinLeftRight(left, right, width)
}

func (m *Model) renderContext(item cache.ActivityItem, width int) string {
	label := contextLabel(item)
	if label == "" {
		return ""
	}
	return clipToWidth(mutedStyle().Render(label), width)
}

func (m *Model) renderPreview(item cache.ActivityItem, width int) string {
	preview := messages.RenderSlackMarkdown(item.Text, m.userNames, m.channelNames)
	preview = strings.ReplaceAll(preview, "\n", " ")
	previewMax := width
	if previewMax < 0 {
		previewMax = 0
	}
	if strings.TrimSpace(preview) == "" {
		preview = mutedStyle().Render("No message preview")
	}
	return clipToWidth(truncate.StringWithTail(preview, uint(previewMax), "…"), width)
}

func (m *Model) renderChannelRef(item cache.ActivityItem) string {
	name := item.ChannelName
	if name == "" {
		name = item.ChannelID
	}
	return channelGlyph(item.ChannelType) + channelNameStyle().Render(name)
}

func channelGlyph(channelType string) string {
	switch channelType {
	case "private":
		return lipgloss.NewStyle().Foreground(styles.Warning).Render("◆ ")
	case "dm", "group_dm":
		return lipgloss.NewStyle().Foreground(styles.TextMuted).Render("● ")
	case "app":
		return ""
	default:
		return "# "
	}
}

func kindLabel(kind string) string {
	switch kind {
	case "mention":
		return "Mention"
	case "dm":
		return "Direct message"
	case "thread_reply":
		return "Thread"
	case "app":
		return "App"
	case "reminder":
		return "Reminder"
	case "invitation":
		return "Invitation"
	case "notification":
		return "Notification"
	default:
		return "Activity"
	}
}

// contextLabel mirrors Slack's second-line context: it omits the
// channel reference for DMs/group DMs (the header already names the
// counterpart) and otherwise renders "<kind> in #channel".
func contextLabel(item cache.ActivityItem) string {
	switch item.ChannelType {
	case "dm":
		return "Direct message"
	case "group_dm":
		return "Group message"
	case "app":
		if strings.Contains(strings.ToLower(item.Text), "user group") {
			return "User group"
		}
		return "App"
	}
	if item.Kind == "notification" && strings.Contains(item.Text, "<!subteam^") {
		return "User group"
	}
	name := item.ChannelName
	if name == "" {
		name = item.ChannelID
	}
	if name == "" {
		return kindLabel(item.Kind)
	}
	return kindLabel(item.Kind) + " in " + channelGlyph(item.ChannelType) + name
}

func (m *Model) resolveUser(uid string) string {
	if uid == "" {
		return ""
	}
	if uid == m.selfUserID {
		return "me"
	}
	if name, ok := m.userNames[uid]; ok && name != "" {
		return name
	}
	return uid
}

func formatRelTime(ts string) string {
	return formatActivityTime(ts)
}

// formatActivityTime mirrors Slack's Activity-list time format:
//   - today: "h:MM AM/PM"
//   - yesterday: "Yesterday"
//   - within the last 6 days: weekday name (e.g. "Saturday")
//   - older: "Mon DD"
//   - older than a year: "Mon DD, YYYY"
//
// Returns "" if ts is empty or unparsable so callers can fall back.
func formatActivityTime(ts string) string {
	if ts == "" {
		return ""
	}
	secStr := ts
	if dot := strings.IndexByte(ts, '.'); dot >= 0 {
		secStr = ts[:dot]
	}
	sec, err := strconv.ParseInt(secStr, 10, 64)
	if err != nil {
		return ""
	}
	t := time.Unix(sec, 0)
	now := time.Now()
	today := startOfDay(now)
	then := startOfDay(t)
	days := int(today.Sub(then) / (24 * time.Hour))
	switch {
	case days <= 0:
		return t.Format("3:04 PM")
	case days == 1:
		return "Yesterday"
	case days < 7:
		return t.Format("Monday")
	case t.Year() == now.Year():
		return t.Format("Jan 2")
	default:
		return t.Format("Jan 2, 2006")
	}
}

func startOfDay(t time.Time) time.Time {
	y, m, d := t.Date()
	return time.Date(y, m, d, 0, 0, 0, 0, t.Location())
}

func joinLeftRight(left, right string, width int) string {
	if width <= 0 {
		return ""
	}
	if right == "" {
		return clipToWidth(left, width)
	}
	rightWidth := lipgloss.Width(right)
	if rightWidth >= width {
		return clipToWidth(right, width)
	}
	leftWidth := width - rightWidth - 1
	if leftWidth < 1 {
		leftWidth = 1
	}
	left = clipToWidth(left, leftWidth)
	gap := width - lipgloss.Width(left) - rightWidth
	if gap < 1 {
		gap = 1
	}
	return left + strings.Repeat(" ", gap) + right
}

func clipToWidth(s string, width int) string {
	if width <= 0 {
		return ""
	}
	if lipgloss.Width(s) <= width {
		return s
	}
	return truncate.StringWithTail(s, uint(width), "…")
}

func stringMapsEqual(a, b map[string]string) bool {
	if len(a) != len(b) {
		return false
	}
	for k, va := range a {
		if vb, ok := b[k]; !ok || vb != va {
			return false
		}
	}
	return true
}

package overlay

import (
	"strings"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/image"
)

// kittyPlaceholderPrefix is the leading byte sequence of any cell whose
// content begins with image.PlaceholderRune (U+10EEEE encoded as UTF-8:
// 0xF4 0x8E 0xBB 0xAE). We match against the Cell.Content string rather
// than the rune to handle the diacritic-suffixed graphemes kitty's
// renderer emits without taking a dependency on cluster segmentation.
var kittyPlaceholderPrefix = string(image.PlaceholderRune)

// DimmedOverlay composites a modal box on top of a dimmed background.
//
// This intentionally stays in string/cell-width space instead of using
// lipgloss.Canvas. The Canvas/Layer path in lipgloss v2.0.3 can drop or
// corrupt Korean/CJK wide cells while parsing styled strings, which makes
// search queries and background text appear as black blocks under modals.
//
// Cells whose content carries the kitty unicode-placeholder rune are
// blanked: their FG is a 24-bit encoding of an image ID (not a visual
// color), so darkening it would mutate the ID and surface a different
// image — see internal/image/kitty.go. Replacing the placeholder rune
// with a space drops the kitty placement for the duration of the
// overlay; the image data stays uploaded and the next non-overlay
// frame re-emits the placeholder cells, re-creating the placement
// without any image-state plumbing. Issue #18.
func DimmedOverlay(width, height int, background string, box string, dimPercent float64) string {
	_ = dimPercent // kept for the public contract; text-safe dimming uses faint SGR.

	bgLines := strings.Split(background, "\n")
	out := make([]string, height)
	dimStyle := lipgloss.NewStyle().Faint(true)
	for y := 0; y < height; y++ {
		line := ""
		if y < len(bgLines) {
			line = sanitizeBackgroundLine(bgLines[y])
		}
		out[y] = dimStyle.Render(fitLine(line, width))
	}

	modalW := lipgloss.Width(box)
	modalH := lipgloss.Height(box)
	startX := (width - modalW) / 2
	startY := (height - modalH) / 2
	if startX < 0 {
		startX = 0
	}
	if startY < 0 {
		startY = 0
	}

	boxLines := strings.Split(box, "\n")
	for my := 0; my < modalH; my++ {
		y := startY + my
		if y < 0 || y >= height {
			continue
		}
		boxLine := ""
		if my < len(boxLines) {
			boxLine = boxLines[my]
		}
		visibleBoxWidth := modalW
		if startX+visibleBoxWidth > width {
			visibleBoxWidth = width - startX
		}
		if visibleBoxWidth <= 0 {
			continue
		}
		boxLine = fitLine(ansi.Cut(boxLine, 0, visibleBoxWidth), visibleBoxWidth)
		left := fitLine(ansi.Cut(out[y], 0, startX), startX)
		rightStart := startX + visibleBoxWidth
		rightWidth := width - rightStart
		right := ""
		if rightWidth > 0 {
			right = fitLine(ansi.Cut(out[y], rightStart, width), rightWidth)
		}
		out[y] = left + boxLine + right
	}

	return strings.Join(out, "\n")
}

func sanitizeBackgroundLine(line string) string {
	line = ansi.Strip(line)
	if strings.Contains(line, kittyPlaceholderPrefix) {
		line = strings.ReplaceAll(line, kittyPlaceholderPrefix, " ")
	}
	return line
}

func fitLine(line string, width int) string {
	if width <= 0 {
		return ""
	}
	line = ansi.Truncate(line, width, "")
	if pad := width - ansi.StringWidth(line); pad > 0 {
		line += strings.Repeat(" ", pad)
	}
	return line
}

package overlay_test

import (
	"fmt"
	"strings"
	"testing"

	"charm.land/lipgloss/v2"
	"github.com/charmbracelet/x/ansi"

	"github.com/gammons/slk/internal/image"
	"github.com/gammons/slk/internal/ui/overlay"
)

// TestDimmedOverlayDropsKittyPlaceholders guards issue #18.
//
// Kitty's unicode-placeholder protocol encodes an image's ID in each
// placeholder cell's SGR foreground color (R = byte 2, G = byte 1,
// B = byte 0 of the 24-bit ID). The original bug: DimmedOverlay's
// per-cell FG darken mutated those IDs, surfacing whatever image
// happened to own the darkened ID — users saw avatars and image
// previews "go wonky" while any modal was open.
//
// The chosen remediation is to drop the kitty placement entirely for
// the duration of the overlay by blanking placeholder cells. The
// image data stays uploaded in the terminal; the next non-overlay
// frame re-emits the placeholder cells and the placement re-appears
// with no image-state plumbing. This test asserts that contract:
// after DimmedOverlay, the rendered output contains zero placeholder
// runes and zero "image ID-as-RGB" SGR escapes from the synthetic
// background we provided.
func TestDimmedOverlayDropsKittyPlaceholders(t *testing.T) {
	const id uint32 = 42
	r := byte((id >> 16) & 0xFF)
	g := byte((id >> 8) & 0xFF)
	b := byte(id & 0xFF)
	idFG := fmt.Sprintf("\x1b[38;2;%d;%d;%dm", r, g, b)

	placeholders := idFG +
		string(image.PlaceholderRune) +
		string(image.PlaceholderRune) +
		string(image.PlaceholderRune) +
		"\x1b[39m"

	const width, height = 12, 3
	bg := strings.Join([]string{
		strings.Repeat("a", width),
		placeholders + strings.Repeat(" ", width-3),
		strings.Repeat("b", width),
	}, "\n")

	// Narrow box so part of the placeholder row survives to the right
	// of the modal — that's the region the bug used to corrupt.
	box := lipgloss.NewStyle().
		Border(lipgloss.NormalBorder()).
		Width(2).
		Height(1).
		Render("X")

	out := overlay.DimmedOverlay(width, height, bg, box, 0.5)

	if strings.ContainsRune(out, image.PlaceholderRune) {
		t.Fatalf("dim left kitty placeholder rune in output (image would render through dim):\n%q", out)
	}
	if strings.Contains(out, idFG) {
		t.Fatalf("dim left raw image-ID FG escape %q in output:\n%q", idFG, out)
	}
}

func TestDimmedOverlayPreservesKoreanWideCells(t *testing.T) {
	const width, height = 48, 6
	bg := strings.Join([]string{
		"한글 검색 결과가 배경에 남아야 합니다" + strings.Repeat(" ", 12),
		"korean wide cells should not become holes" + strings.Repeat(" ", 8),
		"검색어 테스트" + strings.Repeat(" ", 30),
		strings.Repeat(" ", width),
		strings.Repeat(" ", width),
		strings.Repeat(" ", width),
	}, "\n")
	box := lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		Width(12).
		Height(1).
		Render("Search")

	out := overlay.DimmedOverlay(width, height, bg, box, 0.5)
	plain := ansi.Strip(out)

	for _, want := range []string{"한글 검색", "검색어 테스트"} {
		if !strings.Contains(plain, want) {
			t.Fatalf("dimmed overlay lost Korean text %q; rendered plain output:\n%s", want, plain)
		}
	}
}

package image

import (
	"image"
	"strconv"

	"github.com/gammons/slk/internal/debuglog"
)

var renderCellPixels = image.Pt(8, 16)

// NormalizeCellPixels returns a sane terminal cell size. Some terminals /
// multiplexers report bogus pixel metrics (for example, cells wider than they
// are tall), which makes both inline images and fullscreen preview render
// comically small. In those cases we fall back to the conservative default.
func NormalizeCellPixels(px image.Point) image.Point {
	if px.X <= 0 || px.Y <= 0 {
		return image.Pt(8, 16)
	}
	ratio := float64(px.X) / float64(px.Y)
	if ratio < 0.30 || ratio > 1.00 {
		return image.Pt(8, 16)
	}
	return px
}

// SetRenderCellPixels records the actual terminal cell size used by renderers
// that emit pixel-addressed protocols like kitty and sixel.
func SetRenderCellPixels(px image.Point) {
	renderCellPixels = NormalizeCellPixels(px)
}

func currentRenderCellPixels() image.Point {
	return renderCellPixels
}

// CellPixels returns the (width, height) of a terminal cell in pixels.
// It honors $COLORTERM_CELL_WIDTH/$COLORTERM_CELL_HEIGHT, then attempts
// TIOCGWINSZ on the given fd (unix only), then falls back to (8, 16).
//
// fd is typically int(os.Stdout.Fd()). Pass -1 to skip the ioctl path.
// On Windows the ioctl path is unavailable; only the env override and
// fallback apply.
func CellPixels(fd int) (pxW, pxH int) {
	if w, ok := atoi(getenv("COLORTERM_CELL_WIDTH")); ok {
		if h, ok := atoi(getenv("COLORTERM_CELL_HEIGHT")); ok {
			px := NormalizeCellPixels(image.Pt(w, h))
			debuglog.ImgRender("CellPixels: cell_w=%d cell_h=%d source=env_override normalized=(%d,%d)", w, h, px.X, px.Y)
			return px.X, px.Y
		}
	}
	if fd >= 0 {
		if w, h, ok := winsizePixels(fd); ok {
			px := NormalizeCellPixels(image.Pt(w, h))
			debuglog.ImgRender("CellPixels: cell_w=%d cell_h=%d source=ioctl fd=%d normalized=(%d,%d)", w, h, fd, px.X, px.Y)
			return px.X, px.Y
		}
	}
	debuglog.ImgRender("CellPixels: cell_w=8 cell_h=16 source=fallback (no env, no ioctl)")
	return 8, 16
}

func atoi(s string) (int, bool) {
	if s == "" {
		return 0, false
	}
	n, err := strconv.Atoi(s)
	if err != nil || n <= 0 {
		return 0, false
	}
	return n, true
}

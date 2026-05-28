// internal/ui/messages/link_hit_test.go
package messages

import "testing"

// TestHTTPLinkSpansFromLines_SingleLine confirms the baseline path
// works: a labeled link on one rendered line produces one span.
func TestHTTPLinkSpansFromLines_SingleLine(t *testing.T) {
	in := RenderSlackMarkdown("see <https://example.com|the doc>", nil, nil)
	spans := HTTPLinkSpansFromLines([]string{in})
	if len(spans) == 0 {
		t.Fatalf("expected at least one link span, got 0 (rendered: %q)", in)
	}
	got := spans[0]
	if got.URL != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", got.URL)
	}
}

// TestHTTPLinkSpansFromLines_WrappedLink is the regression that
// motivated this file: a hyperlink whose label wraps across multiple
// lines used to only register a click hit on the first wrapped line.
// We now persist the active OSC 8 URL across line boundaries so every
// wrapped continuation row remains clickable.
func TestHTTPLinkSpansFromLines_WrappedLink(t *testing.T) {
	rendered := RenderSlackMarkdown(
		"see <https://example.com|alpha beta gamma delta epsilon zeta eta theta>", nil, nil)
	wrapped := WordWrap(rendered, 20)
	lines := splitLines(wrapped)
	if len(lines) < 2 {
		t.Fatalf("expected wrap to produce >=2 lines, got %d (rendered=%q)", len(lines), rendered)
	}
	spans := HTTPLinkSpansFromLines(lines)
	if len(spans) == 0 {
		t.Fatalf("expected wrapped link spans, got 0 (lines=%q)", lines)
	}
	rows := map[int]bool{}
	for _, s := range spans {
		for r := s.RowStart; r < s.RowEnd; r++ {
			rows[r] = true
		}
	}
	if len(rows) < 2 {
		t.Errorf("link spans cover %d distinct rows, want >=2; spans=%+v", len(rows), spans)
	}
}

// TestHTTPLinkSpansFromLines_PlainTextURL captures the second failure
// mode: a bare http(s) URL emitted by some app posts as plain text
// (not the Slack `<url>` wire form) is not detectable today because
// RenderSlackMarkdown only OSC 8-wraps the angle-bracketed forms.
// Fix: detect raw URLs during render and wrap them too.
func TestHTTPLinkSpansFromLines_PlainTextURL(t *testing.T) {
	rendered := RenderSlackMarkdown("check https://example.com please", nil, nil)
	spans := HTTPLinkSpansFromLines([]string{rendered})
	if len(spans) == 0 {
		t.Fatalf("plain-text URL should be detected as a clickable span (rendered=%q)", rendered)
	}
}

func splitLines(s string) []string {
	var out []string
	start := 0
	for i := 0; i < len(s); i++ {
		if s[i] == '\n' {
			out = append(out, s[start:i])
			start = i + 1
		}
	}
	out = append(out, s[start:])
	return out
}

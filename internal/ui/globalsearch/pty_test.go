package globalsearch

import (
	"bytes"
	"errors"
	"io"
	"strconv"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/charmbracelet/colorprofile"
	"github.com/creack/pty"

	"github.com/gammons/slk/internal/config"
	"github.com/gammons/slk/internal/ui/styles"
)

type overlayPTYQuitMsg struct{}

type overlayPTYModel struct {
	search *Model
	width  int
	height int
}

func (m overlayPTYModel) Init() tea.Cmd {
	return tea.Tick(10*time.Millisecond, func(time.Time) tea.Msg {
		return overlayPTYQuitMsg{}
	})
}

func (m overlayPTYModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg.(type) {
	case overlayPTYQuitMsg, tea.QuitMsg:
		return m, tea.Quit
	}
	return m, nil
}

func (m overlayPTYModel) View() tea.View {
	v := tea.NewView(m.search.View(m.width))
	v.AltScreen = true
	v.Cursor = m.search.Cursor(m.width, m.height)
	return v
}

func TestPTYOverlayRendersHangulInputBackgroundAndCursor(t *testing.T) {
	styles.Apply("dark", config.Theme{})
	t.Cleanup(func() { styles.Apply("dark", config.Theme{}) })

	ptmx, tty, err := pty.Open()
	if err != nil {
		t.Fatalf("open pty: %v", err)
	}
	defer ptmx.Close()
	defer tty.Close()

	const width = 80
	const height = 24
	if err := pty.Setsize(tty, &pty.Winsize{Cols: width, Rows: height}); err != nil {
		t.Fatalf("set pty size: %v", err)
	}

	var out bytes.Buffer
	readDone := make(chan error, 1)
	go func() {
		_, err := io.Copy(&out, ptmx)
		readDone <- err
	}()

	mv := New()
	search := &mv
	search.Open()
	search.HandleKey("한")

	model := overlayPTYModel{search: search, width: width, height: height}
	program := tea.NewProgram(
		model,
		tea.WithInput(nil),
		tea.WithOutput(tty),
		tea.WithWindowSize(width, height),
		tea.WithColorProfile(colorprofile.TrueColor),
		tea.WithEnvironment([]string{"TERM=xterm-256color", "COLORTERM=truecolor"}),
		tea.WithoutSignals(),
	)
	if _, err := program.Run(); err != nil {
		t.Fatalf("run bubble tea program: %v", err)
	}
	if err := tty.Close(); err != nil {
		t.Fatalf("close pty tty: %v", err)
	}
	if err := ptmx.Close(); err != nil {
		t.Fatalf("close pty master: %v", err)
	}
	if err := <-readDone; err != nil && !errors.Is(err, io.ErrClosedPipe) {
		t.Fatalf("read pty output: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "한") {
		t.Fatalf("PTY output must contain Hangul query; output %q", rendered)
	}
	inputAttrs := ansiAttrs(inputBackground(), styles.TextPrimary)
	if !strings.Contains(rendered, inputAttrs) {
		bgAttrs, fgAttrs, ok := strings.Cut(inputAttrs, "m")
		if !ok || !strings.Contains(rendered, strings.TrimPrefix(bgAttrs, "\x1b[")) || !strings.Contains(rendered, strings.TrimPrefix(fgAttrs, "\x1b[")) {
			t.Fatalf("PTY output must contain input background attrs %q; output %q", inputAttrs, rendered)
		}
	}
	cursor := search.Cursor(width, height)
	if cursor == nil {
		t.Fatal("overlay search must expose cursor")
	}
	cursorSeq := "\x1b[" + strconv.Itoa(cursor.Position.Y+1) + ";" + strconv.Itoa(cursor.Position.X+1) + "H"
	if !strings.Contains(rendered, cursorSeq) {
		t.Fatalf("PTY output must move cursor to overlay input row with %q; output %q", cursorSeq, rendered)
	}
}

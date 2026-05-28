//go:build e2e

package e2e

import (
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/creack/pty"
)

// ptySession wraps a child process attached to a pseudo-terminal so a
// test can send keystrokes and grep accumulated screen output. The
// reader goroutine drains the PTY into an internal buffer continuously
// (otherwise the kernel buffer fills and the child blocks on writes).
type ptySession struct {
	cmd   *exec.Cmd
	ptmx  *os.File
	mu    sync.Mutex
	buf   bytes.Buffer
	readD chan struct{} // closed when reader goroutine returns
}

// startSession compiles cmd/slk (if needed) into a per-test tmpdir,
// seeds the workspace token under that same tmpdir's XDG_DATA_HOME,
// and spawns the binary attached to a freshly opened PTY. The caller
// is responsible for invoking session.stop in t.Cleanup.
func startSession(t *testing.T, token, cookie, teamID, teamName string) *ptySession {
	t.Helper()
	tmp := t.TempDir()

	// Build the binary into the tmpdir so each test has its own copy
	// and a stale -race or -count run can't see a previous build's
	// artifact. Build with the current module's go version.
	repoRoot := findRepoRoot(t)
	binPath := filepath.Join(tmp, "slk")
	build := exec.Command("go", "build", "-o", binPath, "./cmd/slk")
	build.Dir = repoRoot
	if out, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build slk: %v\n%s", err, out)
	}

	// Seed the token under tmp/slk/tokens/<team_id>.json so the binary
	// skips onboarding.
	tokenDir := filepath.Join(tmp, "slk", "tokens")
	if err := os.MkdirAll(tokenDir, 0o700); err != nil {
		t.Fatalf("mkdir tokens: %v", err)
	}
	tokenJSON := fmt.Sprintf(
		`{"access_token":%q,"cookie":%q,"team_id":%q,"team_name":%q}`,
		token, cookie, teamID, teamName,
	)
	tokenPath := filepath.Join(tokenDir, teamID+".json")
	if err := os.WriteFile(tokenPath, []byte(tokenJSON), 0o600); err != nil {
		t.Fatalf("write token: %v", err)
	}

	cmd := exec.Command(binPath)
	cmd.Env = append(
		os.Environ(),
		"XDG_DATA_HOME="+tmp,
		// Force a deterministic terminal that bubbletea fully supports.
		"TERM=xterm-256color",
		// Disable any color-detection sidechannels that probe a real TTY.
		"NO_COLOR=",
		// Avoid the locale picker degrading widths under some setups.
		"LANG=en_US.UTF-8",
	)

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 160})
	if err != nil {
		t.Fatalf("pty.Start: %v", err)
	}

	s := &ptySession{cmd: cmd, ptmx: ptmx, readD: make(chan struct{})}
	go s.drain()
	t.Cleanup(s.stop)
	return s
}

// drain copies the PTY's output into the session's buffer until the
// child exits or the PTY is closed. Runs in its own goroutine because
// a blocked reader would otherwise stall the child on the first full
// kernel buffer (~64 KiB) which is well under one full TUI repaint.
func (s *ptySession) drain() {
	defer close(s.readD)
	chunk := make([]byte, 4096)
	for {
		n, err := s.ptmx.Read(chunk)
		if n > 0 {
			s.mu.Lock()
			s.buf.Write(chunk[:n])
			s.mu.Unlock()
		}
		if err != nil {
			return
		}
	}
}

// send writes raw bytes to the PTY (interpreted as keyboard input).
// Use control characters directly, e.g. "\x1b" for Esc, "\r" for Enter.
func (s *ptySession) send(b string) error {
	_, err := s.ptmx.Write([]byte(b))
	return err
}

// screen returns a snapshot of everything the child has written since
// the session started. The TUI repaints the full screen on most events
// so the latest paint is at the end of the buffer; callers that only
// care about the current frame can search backwards.
func (s *ptySession) screen() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.buf.String()
}

// waitFor polls the screen buffer until needle appears, the timeout
// elapses, or the child exits. The poll cadence is short (50 ms) so
// snappy events don't drag the test out.
func (s *ptySession) waitFor(t *testing.T, needle string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if strings.Contains(s.screen(), needle) {
			return
		}
		select {
		case <-s.readD:
			t.Fatalf("child exited before %q appeared. screen tail:\n%s", needle, tailScreen(s.screen(), 800))
		case <-time.After(50 * time.Millisecond):
		}
	}
	t.Fatalf("timed out waiting for %q after %s. screen tail:\n%s", needle, timeout, tailScreen(s.screen(), 800))
}

// stop terminates the child and closes the PTY. Idempotent. Waits on
// the child after kill so the test process doesn't leave zombies — on
// macOS in particular, unreaped children show up in `ps` until the
// parent exits, which is noisy when running the test repeatedly.
func (s *ptySession) stop() {
	if s.cmd != nil && s.cmd.Process != nil {
		_ = s.cmd.Process.Kill()
	}
	if s.ptmx != nil {
		_ = s.ptmx.Close()
	}
	<-s.readD
	if s.cmd != nil {
		_ = s.cmd.Wait()
	}
}

// findRepoRoot walks up from the test file's working directory until
// it sees go.mod, so `go build ./cmd/slk` resolves against the actual
// module regardless of where the test is invoked from.
func findRepoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate go.mod from test working directory")
		}
		dir = parent
	}
}

// tailScreen returns the last n bytes of s, useful for failure
// messages where dumping the whole accumulated output would drown the
// signal in ANSI escape noise.
func tailScreen(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return "...(truncated)...\n" + s[len(s)-n:]
}

// requireEnv pulls a value from the environment and skips the test if
// it's empty. The skip message names every required var so the user
// knows what to set without grepping the test source.
func requireEnv(t *testing.T) (token, cookie, teamID, teamName string) {
	t.Helper()
	token = os.Getenv("SLK_TEST_TOKEN")
	cookie = os.Getenv("SLK_TEST_COOKIE")
	teamID = os.Getenv("SLK_TEST_TEAM_ID")
	teamName = os.Getenv("SLK_TEST_TEAM_NAME")
	if token == "" || cookie == "" || teamID == "" || teamName == "" {
		t.Skip("e2e test requires SLK_TEST_TOKEN, SLK_TEST_COOKIE, SLK_TEST_TEAM_ID, SLK_TEST_TEAM_NAME")
	}
	return
}

// asciiOnly strips ANSI escape sequences so substring searches in
// assertions don't have to match color codes. Crude but sufficient for
// presence-of-text checks (it doesn't preserve cell positions).
func asciiOnly(s string) string {
	var out strings.Builder
	out.Grow(len(s))
	inEsc := false
	for _, r := range s {
		if inEsc {
			if (r >= 'A' && r <= 'Z') || (r >= 'a' && r <= 'z') {
				inEsc = false
			}
			continue
		}
		if r == 0x1b {
			inEsc = true
			continue
		}
		out.WriteRune(r)
	}
	return out.String()
}

// rightHalfHasText reports whether the most-recent visible frame has
// any non-whitespace content past the column where the activity list
// ends. The activity list occupies roughly the left 40% of msgArea —
// anything substantive past column ~60 is the preview pane. Used as a
// fuzzy "preview rendered something" check that doesn't depend on
// specific message text (which is live workspace state).
func rightHalfHasText(s string, splitCol int) bool {
	plain := asciiOnly(s)
	lines := strings.Split(plain, "\n")
	// Walk the last N lines so we look at the current frame, not stale
	// loading screens accumulated earlier in the buffer.
	tail := lines
	if len(tail) > 80 {
		tail = tail[len(tail)-80:]
	}
	for _, line := range tail {
		if len(line) <= splitCol {
			continue
		}
		right := strings.TrimSpace(line[splitCol:])
		// Skip border-only fragments (┃, │, ─, etc.) and empty cells.
		stripped := strings.Map(func(r rune) rune {
			switch r {
			case ' ', '│', '┃', '─', '━', '┌', '┐', '└', '┘', '├', '┤', '┬', '┴', '┼':
				return -1
			}
			return r
		}, right)
		if stripped != "" {
			return true
		}
	}
	return false
}

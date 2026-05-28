// Package e2e holds end-to-end PTY-driven tests that spawn the real slk
// binary, attach a pseudo-terminal, and assert on rendered output. They
// are gated behind the "e2e" build tag because:
//
//  1. They require real Slack credentials (no mock server exists yet);
//     the API/WebSocket URLs are hardcoded to slack.com in
//     internal/slack/client.go.
//  2. They depend on live workspace data so assertions are necessarily
//     fuzzy (presence of *some* preview content rather than specific
//     text). Running them on every push would generate flaky failures
//     when the workspace state shifts.
//
// To run locally:
//
//	export SLK_TEST_TOKEN="xoxc-..."
//	export SLK_TEST_COOKIE="<d cookie value>"
//	export SLK_TEST_TEAM_ID="T..."
//	export SLK_TEST_TEAM_NAME="MyWorkspace"
//	go test -tags=e2e ./internal/e2e/...
//
// The harness sets XDG_DATA_HOME to a per-test tmpdir, writes a seeded
// token file there, and spawns the binary built from cmd/slk.
package e2e

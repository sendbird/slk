package cache

import (
	"fmt"
)

// RecordChannelVisit upserts the (workspace_id, channel_id) row with the
// caller-supplied timestamp. The timestamp must be captured at the moment
// the visit happened so persistence order matches navigation order even
// when writes are dispatched asynchronously. The DB UPDATE is monotonic
// (MAX of stored vs incoming) so an older write that lands after a
// newer one can't roll the row back -- important for rapid repeated
// selection of the same channel where goroutines race into SQLite. Used
// by the App when the user navigates to a channel so the Ctrl+T finder
// can order entries by recency and the next restart can restore the
// last viewed channel.
func (db *DB) RecordChannelVisit(workspaceID, channelID string, visitedAt int64) error {
	_, err := db.conn.Exec(`
		INSERT INTO channel_visits (workspace_id, channel_id, last_visited)
		VALUES (?, ?, ?)
		ON CONFLICT(workspace_id, channel_id)
		DO UPDATE SET last_visited = MAX(channel_visits.last_visited, excluded.last_visited)`,
		workspaceID, channelID, visitedAt,
	)
	if err != nil {
		return fmt.Errorf("recording channel visit: %w", err)
	}
	return nil
}

// GetChannelVisits returns a map of channel_id -> last_visited for the
// given workspace. The timestamp scale matches whatever RecordChannelVisit
// callers stored; current callers use milliseconds. Used at workspace-
// connect time to seed the in-memory map that the channel finder
// consults for sorting and to restore the most recently viewed channel.
func (db *DB) GetChannelVisits(workspaceID string) (map[string]int64, error) {
	rows, err := db.conn.Query(`
		SELECT channel_id, last_visited
		FROM channel_visits
		WHERE workspace_id = ?`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying channel visits: %w", err)
	}
	defer rows.Close()

	out := make(map[string]int64)
	for rows.Next() {
		var channelID string
		var lastVisited int64
		if err := rows.Scan(&channelID, &lastVisited); err != nil {
			return nil, fmt.Errorf("scanning channel visit: %w", err)
		}
		out[channelID] = lastVisited
	}
	return out, rows.Err()
}

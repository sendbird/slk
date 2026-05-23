package cache

import (
	"database/sql"
	"fmt"
)

// ReadState captures the per-channel read-state values that drive the
// unread dot and "new messages" line. It is the canonical type for
// passing read state across package boundaries.
type ReadState struct {
	LastReadTS string
	HasUnread  bool
	// LatestTS is the Slack timestamp of the most recent message we've
	// observed in the channel (advanced by realtime WS handlers; sourced
	// from channels.latest_synced_ts, with MAX(messages.ts) as fallback
	// in GetChannelWatermark). The sidebar uses this to sort DMs by
	// recency. Empty when we've never observed a message in the channel.
	LatestTS string
	// MentionCount is the number of unread messages in this channel that
	// directly @-mention the current user. Populated from client.counts
	// at bootstrap/reconnect and reset to 0 when the channel is marked
	// read. The sidebar uses this to lift mention-bearing channels to the
	// top of the Channels section and renders it as a badge next to the
	// channel name.
	MentionCount int
}

// ChannelReadStateUpdate is one entry in a batched read-state write.
// LastReadTS == "" means "preserve the existing last_read_ts" (used by
// events that update has_unread only, e.g. new-message arrivals).
type ChannelReadStateUpdate struct {
	ChannelID  string
	LastReadTS string
	HasUnread  bool
	// MentionCount writes channels.mention_count alongside has_unread.
	// Negative values are clamped to 0 by BatchUpdateChannelReadState so
	// callers (bootstrap, reconnect catch-up) can pass server-provided
	// counters directly without sanitizing.
	MentionCount int
}

// UpdateChannelReadState atomically updates the per-channel read state.
// If lastReadTS == "", the existing last_read_ts is preserved. This is
// the ONLY function permitted to modify read state after bootstrap.
//
// When hasUnread transitions to false, mention_count is also reset to 0
// so the sidebar's mention badge clears at the same moment the unread
// dot does. Callers that need to set mention_count to a specific value
// (e.g., bootstrap from client.counts) should use
// BatchUpdateChannelReadState or SetChannelMentionCount instead.
func (db *DB) UpdateChannelReadState(channelID, lastReadTS string, hasUnread bool) error {
	var q string
	var args []any
	switch {
	case lastReadTS == "" && !hasUnread:
		q = `UPDATE channels SET has_unread = 0, mention_count = 0 WHERE id = ?`
		args = []any{channelID}
	case lastReadTS == "":
		q = `UPDATE channels SET has_unread = ? WHERE id = ?`
		args = []any{boolToInt(hasUnread), channelID}
	case !hasUnread:
		q = `UPDATE channels SET last_read_ts = ?, has_unread = 0, mention_count = 0 WHERE id = ?`
		args = []any{lastReadTS, channelID}
	default:
		q = `UPDATE channels SET last_read_ts = ?, has_unread = ? WHERE id = ?`
		args = []any{lastReadTS, boolToInt(hasUnread), channelID}
	}
	if _, err := db.conn.Exec(q, args...); err != nil {
		return fmt.Errorf("updating channel read state: %w", err)
	}
	return nil
}

// BatchUpdateChannelReadState writes multiple updates in a single
// transaction. Used by bootstrap and reconnect catch-up paths.
func (db *DB) BatchUpdateChannelReadState(updates []ChannelReadStateUpdate) error {
	if len(updates) == 0 {
		return nil
	}
	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("begin batch read-state tx: %w", err)
	}
	stmtBoth, err := tx.Prepare(`UPDATE channels SET last_read_ts = ?, has_unread = ?, mention_count = ? WHERE id = ?`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare both: %w", err)
	}
	defer stmtBoth.Close()
	stmtFlag, err := tx.Prepare(`UPDATE channels SET has_unread = ?, mention_count = ? WHERE id = ?`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("prepare flag: %w", err)
	}
	defer stmtFlag.Close()

	for _, u := range updates {
		mc := u.MentionCount
		if mc < 0 {
			mc = 0
		}
		if u.LastReadTS == "" {
			if _, err := stmtFlag.Exec(boolToInt(u.HasUnread), mc, u.ChannelID); err != nil {
				tx.Rollback()
				return fmt.Errorf("batch flag for %s: %w", u.ChannelID, err)
			}
		} else {
			if _, err := stmtBoth.Exec(u.LastReadTS, boolToInt(u.HasUnread), mc, u.ChannelID); err != nil {
				tx.Rollback()
				return fmt.Errorf("batch both for %s: %w", u.ChannelID, err)
			}
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("commit batch read-state: %w", err)
	}
	return nil
}

// GetChannelReadState returns the read state for a single channel.
// A missing row yields a zero-valued ReadState and a nil error.
func (db *DB) GetChannelReadState(channelID string) (ReadState, error) {
	var lastReadTS, latestTS string
	var hasUnread, mentionCount int
	err := db.conn.QueryRow(
		`SELECT last_read_ts, has_unread, COALESCE(latest_synced_ts, ''), COALESCE(mention_count, 0) FROM channels WHERE id = ?`,
		channelID,
	).Scan(&lastReadTS, &hasUnread, &latestTS, &mentionCount)
	if err == sql.ErrNoRows {
		return ReadState{}, nil
	}
	if err != nil {
		return ReadState{}, fmt.Errorf("getting channel read state: %w", err)
	}
	return ReadState{
		LastReadTS:   lastReadTS,
		HasUnread:    hasUnread == 1,
		LatestTS:     latestTS,
		MentionCount: mentionCount,
	}, nil
}

// GetWorkspaceReadState returns channelID -> ReadState for every
// channel in the workspace. Single batched query. Called by the
// sidebar View() at render time.
func (db *DB) GetWorkspaceReadState(workspaceID string) (map[string]ReadState, error) {
	rows, err := db.conn.Query(
		`SELECT id, last_read_ts, has_unread, COALESCE(latest_synced_ts, ''), COALESCE(mention_count, 0) FROM channels WHERE workspace_id = ?`,
		workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("query workspace read state: %w", err)
	}
	defer rows.Close()
	out := make(map[string]ReadState)
	for rows.Next() {
		var id, lastRead, latestTS string
		var hasUnread, mentionCount int
		if err := rows.Scan(&id, &lastRead, &hasUnread, &latestTS, &mentionCount); err != nil {
			return nil, fmt.Errorf("scan workspace read state: %w", err)
		}
		out[id] = ReadState{
			LastReadTS:   lastRead,
			HasUnread:    hasUnread == 1,
			LatestTS:     latestTS,
			MentionCount: mentionCount,
		}
	}
	return out, rows.Err()
}

// WorkspacesWithUnreads returns the set of workspace IDs with at least
// one has_unread=true channel. Used by the workspace rail.
func (db *DB) WorkspacesWithUnreads() ([]string, error) {
	rows, err := db.conn.Query(
		`SELECT DISTINCT workspace_id FROM channels WHERE has_unread = 1`,
	)
	if err != nil {
		return nil, fmt.Errorf("query workspaces with unreads: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, fmt.Errorf("scan workspace id: %w", err)
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

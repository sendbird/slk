package cache

import (
	"database/sql"
	"fmt"
	"sort"
)

// ThreadSummary is one row in the Threads view: a thread the user is
// involved in (authored, replied to, or @-mentioned in). Computed from
// the local cache; v1 has no Slack-side authoritative data.
type ThreadSummary struct {
	ChannelID    string
	ChannelName  string
	ChannelType  string // "channel" | "private" | "dm" | "group_dm"
	ThreadTS     string
	ParentUserID string
	ParentText   string
	ParentTS     string
	ReplyCount   int // number of replies (does not count the parent)
	LastReplyTS  string
	LastReplyBy  string
	Unread       bool
}

// ListSubscribedThreads returns the workspace's Threads view rows. It
// starts with Slack's authoritative active thread_subscriptions, then
// unions in cached threads the current user is involved in (authored,
// replied to, or @-mentioned). The cached-involved fallback is important
// because Slack's subscription view can be much narrower than the list
// users expect from the Threads screen; relying on active subscriptions
// alone leaves the panel visibly truncated even when the local message
// cache has many relevant threads.
//
// Threads with no cached messages still appear when they come from an
// active subscription; their parent text/user fall back to "" and
// LastReplyTS falls back to the subscription's LastRead so sort still
// produces a sensible order. Cached-only rows use the channel's
// last_read_ts as the read boundary when no per-thread subscription row
// exists.
//
// Ordering: newest LastReplyTS first.
//
// Unread is computed from the best available read boundary: per-thread
// LastRead when present, otherwise the channel's last_read_ts.
func (db *DB) ListSubscribedThreads(workspaceID, selfUserID string) ([]ThreadSummary, error) {
	mention := "%<@" + selfUserID + ">%"
	const q = `
WITH involved AS (
    SELECT channel_id, thread_ts
    FROM messages
    WHERE workspace_id = ?
      AND is_deleted = 0
      AND thread_ts != ''
      AND (user_id = ? OR text LIKE ?)
    GROUP BY channel_id, thread_ts

    UNION

    SELECT channel_id, ts AS thread_ts
    FROM messages
    WHERE workspace_id = ?
      AND is_deleted = 0
      AND reply_count > 0
      AND (user_id = ? OR text LIKE ?)
    GROUP BY channel_id, ts
),
keys AS (
    SELECT channel_id, thread_ts
    FROM thread_subscriptions
    WHERE workspace_id = ? AND active = 1

    UNION

    SELECT channel_id, thread_ts
    FROM involved
),
ranked AS (
    SELECT
        k.channel_id,
        k.thread_ts,
        COALESCE(c.name, ''),
        COALESCE(c.type, ''),
        COALESCE(NULLIF(s.last_read, ''), c.last_read_ts, '') AS last_read,
        COALESCE((SELECT user_id FROM messages
                  WHERE workspace_id = ? AND channel_id = k.channel_id
                    AND ts = k.thread_ts AND is_deleted = 0), ''),
        COALESCE((SELECT text FROM messages
                  WHERE workspace_id = ? AND channel_id = k.channel_id
                    AND ts = k.thread_ts AND is_deleted = 0), ''),
        (SELECT COUNT(*) FROM messages
         WHERE workspace_id = ? AND channel_id = k.channel_id
           AND thread_ts = k.thread_ts AND ts != k.thread_ts
           AND is_deleted = 0) AS reply_count,
        COALESCE(
            (SELECT MAX(ts) FROM messages
             WHERE workspace_id = ? AND channel_id = k.channel_id
               AND (thread_ts = k.thread_ts OR ts = k.thread_ts)
               AND is_deleted = 0),
            s.last_read,
            c.last_read_ts,
            k.thread_ts
        ) AS last_reply_ts,
        COALESCE(
            (SELECT user_id FROM messages
             WHERE workspace_id = ? AND channel_id = k.channel_id
               AND (thread_ts = k.thread_ts OR ts = k.thread_ts)
               AND is_deleted = 0
             ORDER BY ts DESC LIMIT 1),
            ''
        ) AS last_reply_by
    FROM keys k
    LEFT JOIN thread_subscriptions s
      ON s.workspace_id = ?
     AND s.channel_id = k.channel_id
     AND s.thread_ts = k.thread_ts
    LEFT JOIN channels c
      ON c.workspace_id = ?
     AND c.id = k.channel_id
)
SELECT * FROM ranked
ORDER BY last_reply_ts DESC
LIMIT 1000
`
	rows, err := db.conn.Query(q,
		workspaceID, selfUserID, mention,
		workspaceID, selfUserID, mention,
		workspaceID,
		workspaceID, workspaceID, workspaceID, workspaceID, workspaceID,
		workspaceID, workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("listing subscribed threads: %w", err)
	}
	defer rows.Close()

	var out []ThreadSummary
	for rows.Next() {
		var s ThreadSummary
		var lastRead string
		if err := rows.Scan(
			&s.ChannelID,
			&s.ThreadTS,
			&s.ChannelName,
			&s.ChannelType,
			&lastRead,
			&s.ParentUserID,
			&s.ParentText,
			&s.ReplyCount,
			&s.LastReplyTS,
			&s.LastReplyBy,
		); err != nil {
			return nil, fmt.Errorf("scanning subscribed thread row: %w", err)
		}
		s.ParentTS = s.ThreadTS
		s.Unread = s.LastReplyTS > lastRead && s.LastReplyBy != selfUserID && s.LastReplyBy != ""
		out = append(out, s)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	sort.SliceStable(out, func(i, j int) bool {
		return out[i].LastReplyTS > out[j].LastReplyTS
	})
	return out, nil
}

// ThreadInvolvesUser reports whether the given thread (identified by
// workspaceID, channelID, threadTS) has any cached message authored
// by selfUserID or containing the angle-bracketed mention "<@selfUserID>".
// Used by the reconnect backfill to filter which threads warrant a
// conversations.replies catch-up call.
func (db *DB) ThreadInvolvesUser(workspaceID, channelID, threadTS, selfUserID string) (bool, error) {
	mention := "%<@" + selfUserID + ">%"
	const q = `
SELECT 1 FROM messages
WHERE workspace_id = ? AND channel_id = ? AND thread_ts = ?
  AND is_deleted = 0
  AND (user_id = ? OR text LIKE ?)
LIMIT 1
`
	var one int
	err := db.conn.QueryRow(q, workspaceID, channelID, threadTS, selfUserID, mention).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking thread involvement: %w", err)
	}
	return true, nil
}

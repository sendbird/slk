package cache

import (
	"fmt"
	"sort"
	"strings"
)

// ActivityItem is one row in the Activity view. It is derived entirely from
// the local cache and represents a user-relevant event in a channel.
type ActivityItem struct {
	Kind        string // mention | thread_reply | dm | app | reminder | invitation | notification
	ChannelID   string
	ChannelName string
	ChannelType string
	TS          string
	ThreadTS    string
	UserID      string
	Text        string
	Subtype     string
	Unread      bool
}

// ListActivityItems returns a cache-backed approximation of Slack's Activity
// view for the current user. It models Slack's filter taxonomy more closely
// than a generic unread feed: direct mentions, active thread replies, recent
// DMs, app messages, reminders, invitations, and notification-style rows.
func (db *DB) ListActivityItems(workspaceID, selfUserID string, limit int) ([]ActivityItem, error) {
	if limit <= 0 {
		limit = 100
	}

	const q = `
SELECT
    m.ts,
    COALESCE(m.thread_ts, ''),
    m.channel_id,
	    COALESCE(c.name, ''),
	    COALESCE(c.type, ''),
	    COALESCE(m.user_id, ''),
	    COALESCE(m.text, ''),
	    COALESCE(m.subtype, ''),
	    COALESCE(c.last_read_ts, ''),
	    COALESCE(c.has_unread, 0),
	    COALESCE(ts.last_read, ''),
	    COALESCE(ts.active, 0)
FROM messages m
LEFT JOIN channels c
  ON c.id = m.channel_id
LEFT JOIN thread_subscriptions ts
  ON ts.workspace_id = m.workspace_id
 AND ts.channel_id = m.channel_id
 AND ts.thread_ts = m.thread_ts
	WHERE m.workspace_id = ?
		  AND m.is_deleted = 0
		  AND (
		    m.text LIKE ?
		    OR c.type IN ('dm', 'group_dm', 'app')
		    OR m.subtype IN ('channel_join', 'group_join', 'reminder_add')
		    OR (
		      c.type = 'private'
		      AND c.has_unread = 1
		      AND (
		        COALESCE(m.subtype, '') = 'bot_message'
		        OR m.text LIKE '%Manage reminder%'
		        OR m.text LIKE '%<!subteam^%'
		      )
		    )
		    OR (ts.active = 1 AND m.thread_ts != '' AND m.ts != m.thread_ts)
		  )
	ORDER BY m.ts DESC
	LIMIT ?
	`

	mention := "%<@" + selfUserID + ">%"
	rows, err := db.conn.Query(q, workspaceID, mention, limit*10)
	if err != nil {
		return nil, fmt.Errorf("listing activity items: %w", err)
	}
	defer rows.Close()

	byKey := map[string]ActivityItem{}
	priority := map[string]int{
		"mention":      7,
		"dm":           6,
		"thread_reply": 5,
		"app":          4,
		"reminder":     3,
		"invitation":   2,
		"notification": 1,
	}

	for rows.Next() {
		var item ActivityItem
		var channelLastRead string
		var channelHasUnread int
		var threadLastRead string
		var threadActive int
		if err := rows.Scan(
			&item.TS,
			&item.ThreadTS,
			&item.ChannelID,
			&item.ChannelName,
			&item.ChannelType,
			&item.UserID,
			&item.Text,
			&item.Subtype,
			&channelLastRead,
			&channelHasUnread,
			&threadLastRead,
			&threadActive,
		); err != nil {
			return nil, fmt.Errorf("scanning activity row: %w", err)
		}

		if item.UserID == selfUserID {
			continue
		}

		kind := ""
		unread := false
		isDirectMention := item.Text != "" && selfUserID != "" && containsMention(item.Text, selfUserID)
		isUserGroupMention := strings.Contains(item.Text, "<!subteam^")
		isThreadReply := item.ThreadTS != "" && item.ThreadTS != item.TS
		isTopLevel := item.ThreadTS == "" || item.ThreadTS == item.TS
		isUnreadThreadReply := threadActive == 1 && isThreadReply && item.TS > threadLastRead
		isActiveThreadReply := threadActive == 1 && isThreadReply
		isDM := (item.ChannelType == "dm" || item.ChannelType == "group_dm") && isTopLevel
		isApp := item.ChannelType == "app" && isTopLevel
		isReminder := item.Subtype == "reminder_add" || strings.Contains(item.Text, "Manage reminder")
		isInvitation := item.Subtype == "channel_join" || item.Subtype == "group_join"
		isNotification := item.ChannelType == "private" && channelHasUnread == 1 && isTopLevel &&
			(item.Subtype == "bot_message" || isReminder || isUserGroupMention)
		isMention := isDirectMention || isUserGroupMention

		switch {
		case isMention:
			kind = "mention"
			unread = channelHasUnread == 1 || isUnreadThreadReply || item.TS > channelLastRead
		case isDM:
			kind = "dm"
			unread = channelHasUnread == 1 && item.TS > channelLastRead
		case isActiveThreadReply:
			kind = "thread_reply"
			unread = isUnreadThreadReply
		case isApp:
			kind = "app"
			unread = channelHasUnread == 1 && item.TS > channelLastRead
		case isReminder:
			kind = "reminder"
			unread = channelHasUnread == 1 && item.TS > channelLastRead
		case isInvitation:
			kind = "invitation"
			unread = channelHasUnread == 1 && item.TS > channelLastRead
		case isNotification:
			kind = "notification"
			unread = true
		default:
			continue
		}

		item.Kind = kind
		item.Unread = unread
		if item.ThreadTS == "" {
			item.ThreadTS = item.TS
		}

		// Dedup key intentionally varies per kind to mirror the Slack
		// Activity feed's grouping:
		//   - thread replies collapse into one row per (channel, thread)
		//     keyed on the latest reply (highest TS wins);
		//   - everything else stays per-message so mentions and individual
		//     unread channel messages get their own row.
		var key string
		if item.Kind == "thread_reply" {
			key = "thread:" + item.ChannelID + ":" + item.ThreadTS
		} else if item.Kind == "dm" || item.Kind == "app" || item.Kind == "reminder" || item.Kind == "invitation" || item.Kind == "notification" {
			key = item.Kind + ":" + item.ChannelID
		} else {
			key = item.Kind + ":" + item.ChannelID + ":" + item.TS
		}
		if existing, ok := byKey[key]; ok {
			if item.Kind == "thread_reply" || item.Kind == "dm" || item.Kind == "app" || item.Kind == "reminder" || item.Kind == "invitation" || item.Kind == "notification" {
				// Keep the freshest row per Slack-like grouped category.
				if item.TS > existing.TS {
					byKey[key] = item
				}
			} else if priority[item.Kind] > priority[existing.Kind] {
				byKey[key] = item
			}
			continue
		}
		byKey[key] = item
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}

	out := make([]ActivityItem, 0, len(byKey))
	for _, item := range byKey {
		out = append(out, item)
	}

	sort.SliceStable(out, func(i, j int) bool {
		if priority[out[i].Kind] != priority[out[j].Kind] {
			return priority[out[i].Kind] > priority[out[j].Kind]
		}
		if out[i].Unread != out[j].Unread {
			return out[i].Unread
		}
		return out[i].TS > out[j].TS
	})

	if len(out) > limit {
		out = out[:limit]
	}
	return out, nil
}

func containsMention(text, userID string) bool {
	if text == "" || userID == "" {
		return false
	}
	return contains(text, "<@"+userID+">")
}

func contains(s, sub string) bool {
	return len(sub) > 0 && len(s) >= len(sub) && func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}()
}

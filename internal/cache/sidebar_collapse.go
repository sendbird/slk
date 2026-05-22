package cache

import (
	"fmt"
	"time"
)

// SetSidebarSectionCollapsed records whether the sidebar's section
// identified by sectionKey is collapsed in the given workspace. The
// sectionKey is whatever the sidebar uses for ToggleCollapse — a
// section name ("Channels", "Direct Messages", a custom section
// name) in config mode, or a Slack-native section ID in Slack
// mode. Rows are upserted with a monotonically advancing updated_at
// so a future "most-recently-toggled" caller can sort if needed.
//
// We persist BOTH collapsed=true and collapsed=false rows rather
// than treating absence as expanded: callers that mix persisted
// state with section-name defaults (e.g. "Channels collapsed by
// default") need to distinguish "user expanded this" from "never
// touched."
func (db *DB) SetSidebarSectionCollapsed(workspaceID, sectionKey string, collapsed bool) error {
	if workspaceID == "" || sectionKey == "" {
		return fmt.Errorf("SetSidebarSectionCollapsed: workspace_id and section_key required")
	}
	_, err := db.conn.Exec(`
		INSERT INTO sidebar_section_collapsed (workspace_id, section_key, collapsed, updated_at)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(workspace_id, section_key) DO UPDATE SET
			collapsed = excluded.collapsed,
			updated_at = excluded.updated_at`,
		workspaceID, sectionKey, boolToInt(collapsed), time.Now().Unix(),
	)
	if err != nil {
		return fmt.Errorf("upserting sidebar_section_collapsed: %w", err)
	}
	return nil
}

// GetSidebarSectionCollapsed returns the persisted collapse state for
// every section the user has toggled in this workspace. The returned
// map's presence semantics are "we have a recorded value" — callers
// merge it with their per-section defaults so untouched sections
// keep their first-launch behavior (Channels + Apps collapsed in
// config mode).
func (db *DB) GetSidebarSectionCollapsed(workspaceID string) (map[string]bool, error) {
	rows, err := db.conn.Query(`
		SELECT section_key, collapsed
		FROM sidebar_section_collapsed
		WHERE workspace_id = ?`, workspaceID,
	)
	if err != nil {
		return nil, fmt.Errorf("querying sidebar_section_collapsed: %w", err)
	}
	defer rows.Close()
	out := make(map[string]bool)
	for rows.Next() {
		var key string
		var collapsed int
		if err := rows.Scan(&key, &collapsed); err != nil {
			return nil, fmt.Errorf("scanning sidebar_section_collapsed: %w", err)
		}
		out[key] = collapsed == 1
	}
	return out, rows.Err()
}

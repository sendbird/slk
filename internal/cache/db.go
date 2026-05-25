package cache

import (
	"database/sql"
	"encoding/json"
	"fmt"
	"strings"

	_ "modernc.org/sqlite"
)

type DB struct {
	conn *sql.DB
}

// dsnPragmas are appended to every DSN passed to New(). They are
// applied per-connection by modernc.org/sqlite as the pool opens new
// connections, which is the only reliable way to set per-connection
// pragmas (a one-off conn.Exec only sets the pragma on the single
// connection that ran it). See issue #9: without busy_timeout, two
// goroutines that write at the same time fail the second with
// SQLITE_BUSY immediately instead of waiting, which the reconnect
// backfill silently swallowed.
//
//   - busy_timeout(5000): writers wait up to 5s for a competing
//     writer to finish before returning SQLITE_BUSY. Five seconds
//     comfortably covers the bursty fan-out in runChannelPhase.
//   - journal_mode(WAL): concurrent readers don't block the writer.
//     WAL persists in the file header once set, but applying it
//     per-connection is harmless and keeps it visible in the DSN
//     alongside busy_timeout.
//   - foreign_keys(ON): mirrors the previous one-shot PRAGMA exec.
const dsnPragmas = "_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(ON)"

// appendPragmas returns dsn with the per-connection pragmas spliced
// into its query string. Handles plain paths, ":memory:", and
// already-URI-formatted DSNs (file:..., or path?key=value).
func appendPragmas(dsn string) string {
	sep := "?"
	if strings.Contains(dsn, "?") {
		sep = "&"
	}
	return dsn + sep + dsnPragmas
}

func New(dsn string) (*DB, error) {
	conn, err := sql.Open("sqlite", appendPragmas(dsn))
	if err != nil {
		return nil, fmt.Errorf("opening database: %w", err)
	}

	// SQLite ":memory:" databases are per-connection: each new
	// connection in the pool gets its own empty database. Pin the
	// pool to a single connection so concurrent writers all see the
	// same in-memory schema. Disk-backed DSNs are unaffected.
	if strings.HasPrefix(dsn, ":memory:") {
		conn.SetMaxOpenConns(1)
	}

	db := &DB{conn: conn}
	if err := db.migrate(); err != nil {
		conn.Close()
		return nil, fmt.Errorf("running migrations: %w", err)
	}

	return db, nil
}

func (db *DB) Close() error {
	return db.conn.Close()
}

func (db *DB) migrate() error {
	schema := `
	CREATE TABLE IF NOT EXISTS workspaces (
		id TEXT PRIMARY KEY,
		name TEXT NOT NULL,
		domain TEXT NOT NULL DEFAULT '',
		icon_url TEXT NOT NULL DEFAULT '',
		last_synced_at INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS users (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		display_name TEXT NOT NULL DEFAULT '',
		avatar_url TEXT NOT NULL DEFAULT '',
		presence TEXT NOT NULL DEFAULT 'away',
		is_bot INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id)
	);

	CREATE TABLE IF NOT EXISTS channels (
		id TEXT PRIMARY KEY,
		workspace_id TEXT NOT NULL,
		name TEXT NOT NULL,
		type TEXT NOT NULL DEFAULT 'channel',
		topic TEXT NOT NULL DEFAULT '',
		is_member INTEGER NOT NULL DEFAULT 0,
		is_starred INTEGER NOT NULL DEFAULT 0,
		last_read_ts TEXT NOT NULL DEFAULT '',
		unread_count INTEGER NOT NULL DEFAULT 0,
		has_unread INTEGER NOT NULL DEFAULT 0,
		updated_at INTEGER NOT NULL DEFAULT 0,
		FOREIGN KEY (workspace_id) REFERENCES workspaces(id)
	);

	CREATE TABLE IF NOT EXISTS messages (
		ts TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		workspace_id TEXT NOT NULL,
		user_id TEXT NOT NULL DEFAULT '',
		text TEXT NOT NULL DEFAULT '',
		thread_ts TEXT NOT NULL DEFAULT '',
		reply_count INTEGER NOT NULL DEFAULT 0,
		edited_at TEXT NOT NULL DEFAULT '',
		is_deleted INTEGER NOT NULL DEFAULT 0,
			raw_json TEXT NOT NULL DEFAULT '',
			created_at INTEGER NOT NULL DEFAULT 0,
			subtype TEXT NOT NULL DEFAULT '',
			latest_reply TEXT NOT NULL DEFAULT '',
			PRIMARY KEY (ts, channel_id)
		);

	CREATE TABLE IF NOT EXISTS reactions (
		message_ts TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		emoji TEXT NOT NULL,
		user_ids TEXT NOT NULL DEFAULT '[]',
		count INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (message_ts, channel_id, emoji)
	);

	CREATE TABLE IF NOT EXISTS files (
		id TEXT PRIMARY KEY,
		message_ts TEXT NOT NULL DEFAULT '',
		channel_id TEXT NOT NULL DEFAULT '',
		name TEXT NOT NULL DEFAULT '',
		mimetype TEXT NOT NULL DEFAULT '',
		size INTEGER NOT NULL DEFAULT 0,
		url_private TEXT NOT NULL DEFAULT '',
		local_path TEXT NOT NULL DEFAULT '',
		thumbnail_path TEXT NOT NULL DEFAULT ''
	);

	CREATE TABLE IF NOT EXISTS frecent_emoji (
		emoji TEXT PRIMARY KEY,
		use_count INTEGER NOT NULL DEFAULT 0,
		last_used INTEGER NOT NULL DEFAULT 0
	);

	CREATE TABLE IF NOT EXISTS channel_visits (
		workspace_id TEXT NOT NULL,
		channel_id TEXT NOT NULL,
		last_visited INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (workspace_id, channel_id)
	);

	CREATE TABLE IF NOT EXISTS thread_subscriptions (
		workspace_id TEXT NOT NULL,
		channel_id   TEXT NOT NULL,
		thread_ts    TEXT NOT NULL,
		last_read    TEXT NOT NULL DEFAULT '',
		active       INTEGER NOT NULL DEFAULT 1,
		updated_at   INTEGER NOT NULL DEFAULT 0,
		PRIMARY KEY (workspace_id, channel_id, thread_ts)
	);

	CREATE INDEX IF NOT EXISTS idx_messages_channel ON messages(channel_id, ts);
	CREATE INDEX IF NOT EXISTS idx_messages_thread ON messages(thread_ts, channel_id);
	CREATE INDEX IF NOT EXISTS idx_channels_workspace ON channels(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_users_workspace ON users(workspace_id);
	CREATE INDEX IF NOT EXISTS idx_channel_visits_recent ON channel_visits(workspace_id, last_visited DESC);
	CREATE INDEX IF NOT EXISTS idx_thread_subs_workspace
		ON thread_subscriptions(workspace_id, active);

	CREATE TABLE IF NOT EXISTS channel_members (
		workspace_id TEXT NOT NULL,
		channel_id   TEXT NOT NULL,
		user_id      TEXT NOT NULL,
		updated_at   INTEGER NOT NULL,
		PRIMARY KEY (workspace_id, channel_id, user_id)
	);

	CREATE TABLE IF NOT EXISTS channel_membership_meta (
		workspace_id        TEXT NOT NULL,
		channel_id          TEXT NOT NULL,
		last_full_fetch_at  INTEGER NOT NULL,
		PRIMARY KEY (workspace_id, channel_id)
	);

	CREATE INDEX IF NOT EXISTS idx_channel_members_channel
		ON channel_members(workspace_id, channel_id);

	CREATE TABLE IF NOT EXISTS cache_migrations (
		name TEXT PRIMARY KEY
	);

	CREATE TABLE IF NOT EXISTS sidebar_section_collapsed (
		workspace_id TEXT NOT NULL,
		section_key  TEXT NOT NULL,
		collapsed    INTEGER NOT NULL,
		updated_at   INTEGER NOT NULL,
		PRIMARY KEY (workspace_id, section_key)
	);
	`

	if _, err := db.conn.Exec(schema); err != nil {
		return err
	}

	// Idempotent column-level migrations for existing databases.
	// SQLite's ADD COLUMN has no IF NOT EXISTS, so we probe first.
	if err := db.addColumnIfMissing("messages", "subtype",
		"ALTER TABLE messages ADD COLUMN subtype TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := db.addColumnIfMissing("messages", "latest_reply",
		"ALTER TABLE messages ADD COLUMN latest_reply TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := db.backfillMessageLatestReplyFromRawJSONOnce(); err != nil {
		return err
	}
	if err := db.addColumnIfMissing("users", "is_bot",
		"ALTER TABLE users ADD COLUMN is_bot INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := db.addColumnIfMissing("channels", "synced_at",
		"ALTER TABLE channels ADD COLUMN synced_at INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := db.addColumnIfMissing("channels", "latest_synced_ts",
		"ALTER TABLE channels ADD COLUMN latest_synced_ts TEXT NOT NULL DEFAULT ''"); err != nil {
		return err
	}
	if err := db.addColumnIfMissing("channels", "has_unread",
		"ALTER TABLE channels ADD COLUMN has_unread INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := db.addColumnIfMissing("users", "is_external",
		"ALTER TABLE users ADD COLUMN is_external INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}
	if err := db.addColumnIfMissing("channels", "mention_count",
		"ALTER TABLE channels ADD COLUMN mention_count INTEGER NOT NULL DEFAULT 0"); err != nil {
		return err
	}

	return nil
}

const messageLatestReplyBackfillMigration = "messages.latest_reply.raw_json_backfill"

func (db *DB) backfillMessageLatestReplyFromRawJSONOnce() error {
	applied, err := db.cacheMigrationApplied(messageLatestReplyBackfillMigration)
	if err != nil {
		return err
	}
	if applied {
		return nil
	}
	if err := db.backfillMessageLatestReplyFromRawJSON(); err != nil {
		return err
	}
	return db.markCacheMigrationApplied(messageLatestReplyBackfillMigration)
}

func (db *DB) backfillMessageLatestReplyFromRawJSON() error {
	rows, err := db.conn.Query(`
		SELECT ts, channel_id, raw_json
		FROM messages
		WHERE latest_reply = ''
		  AND raw_json LIKE '%"latest_reply"%'
	`)
	if err != nil {
		return fmt.Errorf("querying messages for latest_reply backfill: %w", err)
	}
	defer rows.Close()

	type latestReplyPayload struct {
		LatestReply string `json:"latest_reply"`
	}
	type latestReplyUpdate struct {
		ts          string
		channelID   string
		latestReply string
	}

	var updates []latestReplyUpdate
	for rows.Next() {
		var ts, channelID, rawJSON string
		if err := rows.Scan(&ts, &channelID, &rawJSON); err != nil {
			return fmt.Errorf("scanning latest_reply backfill row: %w", err)
		}
		var payload latestReplyPayload
		if err := json.Unmarshal([]byte(rawJSON), &payload); err != nil {
			continue
		}
		if payload.LatestReply == "" {
			continue
		}
		updates = append(updates, latestReplyUpdate{
			ts:          ts,
			channelID:   channelID,
			latestReply: payload.LatestReply,
		})
	}
	if err := rows.Err(); err != nil {
		return fmt.Errorf("iterating latest_reply backfill rows: %w", err)
	}
	if len(updates) == 0 {
		return nil
	}

	tx, err := db.conn.Begin()
	if err != nil {
		return fmt.Errorf("beginning latest_reply backfill: %w", err)
	}
	stmt, err := tx.Prepare(`
		UPDATE messages
		SET latest_reply = ?
		WHERE channel_id = ?
		  AND ts = ?
		  AND latest_reply = ''
	`)
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("preparing latest_reply backfill: %w", err)
	}
	defer stmt.Close()
	for _, update := range updates {
		if _, err := stmt.Exec(update.latestReply, update.channelID, update.ts); err != nil {
			tx.Rollback()
			return fmt.Errorf("backfilling latest_reply for %s/%s: %w", update.channelID, update.ts, err)
		}
	}
	if err := tx.Commit(); err != nil {
		return fmt.Errorf("committing latest_reply backfill: %w", err)
	}
	return nil
}

func (db *DB) cacheMigrationApplied(name string) (bool, error) {
	var one int
	err := db.conn.QueryRow(`SELECT 1 FROM cache_migrations WHERE name = ?`, name).Scan(&one)
	if err == sql.ErrNoRows {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("checking cache migration %s: %w", name, err)
	}
	return true, nil
}

func (db *DB) markCacheMigrationApplied(name string) error {
	if _, err := db.conn.Exec(`INSERT OR IGNORE INTO cache_migrations (name) VALUES (?)`, name); err != nil {
		return fmt.Errorf("marking cache migration %s: %w", name, err)
	}
	return nil
}

// addColumnIfMissing runs the given DDL only if the column isn't
// already present on the table. Used for additive schema migrations on
// pre-existing databases.
func (db *DB) addColumnIfMissing(table, column, ddl string) error {
	exists, err := db.columnExists(table, column)
	if err != nil {
		return err
	}
	if exists {
		return nil
	}
	if _, err := db.conn.Exec(ddl); err != nil {
		return fmt.Errorf("adding %s.%s: %w", table, column, err)
	}
	return nil
}

func (db *DB) columnExists(table, column string) (bool, error) {
	rows, err := db.conn.Query(fmt.Sprintf("PRAGMA table_info(%s)", table))
	if err != nil {
		return false, fmt.Errorf("inspecting %s columns: %w", table, err)
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, ctype string
		var notnull, pk int
		var dflt sql.NullString
		if err := rows.Scan(&cid, &name, &ctype, &notnull, &dflt, &pk); err != nil {
			return false, fmt.Errorf("scanning %s columns: %w", table, err)
		}
		if name == column {
			return true, nil
		}
	}
	if err := rows.Err(); err != nil {
		return false, err
	}
	return false, nil
}

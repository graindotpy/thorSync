package store

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/apgul/thorsync/internal/adapter"
	"github.com/apgul/thorsync/internal/config"
	"github.com/apgul/thorsync/internal/ids"
	_ "modernc.org/sqlite"
)

const schemaVersion = 1

const (
	revisionPayloadsMigrationKey = "migration_revision_payloads_v1"
	windowsGBAProfileSetting     = "windows_gba_profile"
	windowsGBAConfiguredSetting  = "windows_gba_configured"
)

type Store struct {
	db *sql.DB
}

func Open(ctx context.Context, cfg config.Config) (*Store, error) {
	if err := os.MkdirAll(cfg.DataDir, 0o750); err != nil {
		return nil, fmt.Errorf("create data directory: %w", err)
	}
	db, err := sql.Open("sqlite", "file:"+filepath.ToSlash(cfg.DatabasePath()))
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	for _, pragma := range []string{
		"PRAGMA journal_mode=WAL",
		"PRAGMA synchronous=FULL",
		"PRAGMA foreign_keys=ON",
		"PRAGMA busy_timeout=5000",
	} {
		if _, err := db.ExecContext(ctx, pragma); err != nil {
			db.Close()
			return nil, fmt.Errorf("%s: %w", pragma, err)
		}
	}
	store := &Store{db: db}
	if err := store.migrate(ctx, cfg.DataDir); err != nil {
		db.Close()
		return nil, err
	}
	if err := store.seed(ctx, cfg); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) Ready(ctx context.Context) error {
	if err := s.db.PingContext(ctx); err != nil {
		return err
	}
	var version int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != schemaVersion {
		return fmt.Errorf("schema version %d, expected %d", version, schemaVersion)
	}
	return nil
}

func (s *Store) DB() *sql.DB { return s.db }

func (s *Store) migrate(ctx context.Context, dataDir string) error {
	var current int
	if err := s.db.QueryRowContext(ctx, "PRAGMA user_version").Scan(&current); err != nil {
		return err
	}
	if current > schemaVersion {
		return fmt.Errorf("database schema %d is newer than this application supports (%d)", current, schemaVersion)
	}
	if current > 0 && current < schemaVersion {
		if err := s.backupBeforeMigration(ctx, dataDir, schemaVersion); err != nil {
			return fmt.Errorf("pre-migration backup: %w", err)
		}
	}
	if current == 0 {
		if _, err := s.db.ExecContext(ctx, schemaV1); err != nil {
			return fmt.Errorf("apply schema v1: %w", err)
		}
	}
	// revision_payloads is an additive feature migration rather than a schema
	// version bump. Keeping user_version at 1 allows upgraded databases to stay
	// compatible with the original v1 release while a durable setting records
	// that the one-time backup and backfill completed.
	if err := s.migrateRevisionPayloads(ctx, dataDir, current != 0); err != nil {
		return fmt.Errorf("apply revision payload migration: %w", err)
	}
	return nil
}

func (s *Store) migrateRevisionPayloads(ctx context.Context, dataDir string, existing bool) error {
	var marker string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", revisionPayloadsMigrationKey).Scan(&marker)
	if err != nil && err != sql.ErrNoRows {
		return err
	}
	firstRun := marker != "complete"
	if existing && firstRun {
		if err := s.backupBeforeFeatureMigration(ctx, dataDir, "revision-payloads"); err != nil {
			return fmt.Errorf("pre-migration backup: %w", err)
		}
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.ExecContext(ctx, `CREATE TABLE IF NOT EXISTS revision_payloads (
		revision_id TEXT PRIMARY KEY REFERENCES revisions(id) ON DELETE CASCADE,
		battery_blob_hash TEXT NOT NULL REFERENCES blobs(hash),
		battery_size INTEGER NOT NULL,
		rtc_blob_hash TEXT REFERENCES blobs(hash),
		rtc_size INTEGER NOT NULL DEFAULT 0,
		content_hash TEXT NOT NULL
	)`); err != nil {
		return err
	}
	if _, err = tx.ExecContext(ctx, `CREATE INDEX IF NOT EXISTS revision_payloads_content ON revision_payloads(content_hash)`); err != nil {
		return err
	}
	profileColumn, err := tableHasColumn(ctx, tx, "broker_operations", "profile_id")
	if err != nil {
		return err
	}
	if !profileColumn {
		if _, err = tx.ExecContext(ctx, `ALTER TABLE broker_operations ADD COLUMN profile_id TEXT NOT NULL DEFAULT ''`); err != nil {
			return err
		}
	}
	// A legacy revision was a single raw battery-save blob. Its logical content
	// identity intentionally remains that blob hash, rather than being rehashed.
	if _, err = tx.ExecContext(ctx, `INSERT OR IGNORE INTO revision_payloads
		(revision_id,battery_blob_hash,battery_size,rtc_blob_hash,rtc_size,content_hash)
		SELECT id,blob_hash,size,NULL,0,blob_hash FROM revisions`); err != nil {
		return err
	}
	if firstRun {
		if _, err = tx.ExecContext(ctx, `INSERT INTO settings (key,value,updated_at) VALUES (?, 'complete', ?)
			ON CONFLICT(key) DO UPDATE SET value=excluded.value,updated_at=excluded.updated_at`, revisionPayloadsMigrationKey, time.Now().UTC()); err != nil {
			return err
		}
	}
	return tx.Commit()
}

type pragmaQueryer interface {
	QueryContext(context.Context, string, ...any) (*sql.Rows, error)
}

func tableHasColumn(ctx context.Context, queryer pragmaQueryer, table, column string) (bool, error) {
	rows, err := queryer.QueryContext(ctx, "PRAGMA table_info("+table+")")
	if err != nil {
		return false, err
	}
	defer rows.Close()
	for rows.Next() {
		var cid int
		var name, dataType string
		var notNull, primaryKey int
		var defaultValue any
		if err := rows.Scan(&cid, &name, &dataType, &notNull, &defaultValue, &primaryKey); err != nil {
			return false, err
		}
		if name == column {
			return true, nil
		}
	}
	return false, rows.Err()
}

func (s *Store) backupBeforeMigration(ctx context.Context, dataDir string, target int) error {
	return s.backupDatabase(ctx, dataDir, fmt.Sprintf("v%d", target))
}

func (s *Store) backupBeforeFeatureMigration(ctx context.Context, dataDir, feature string) error {
	return s.backupDatabase(ctx, dataDir, feature)
}

func (s *Store) backupDatabase(ctx context.Context, dataDir, label string) error {
	dir := filepath.Join(dataDir, "migration-backups")
	if err := os.MkdirAll(dir, 0o750); err != nil {
		return err
	}
	name := fmt.Sprintf("thorsync-before-%s-%s.db", label, time.Now().UTC().Format("20060102T150405.000000000Z"))
	path := filepath.Join(dir, name)
	escaped := strings.ReplaceAll(filepath.ToSlash(path), "'", "''")
	if _, err := s.db.ExecContext(ctx, "VACUUM INTO '"+escaped+"'"); err != nil {
		return err
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return err
	}
	var backups []os.DirEntry
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasPrefix(entry.Name(), "thorsync-before-") && strings.HasSuffix(entry.Name(), ".db") {
			backups = append(backups, entry)
		}
	}
	sort.Slice(backups, func(i, j int) bool { return backups[i].Name() > backups[j].Name() })
	if len(backups) > 10 {
		for _, entry := range backups[10:] {
			if err := os.Remove(filepath.Join(dir, entry.Name())); err != nil {
				return err
			}
		}
	}
	return nil
}

func (s *Store) seed(ctx context.Context, cfg config.Config) error {
	now := time.Now().UTC()
	endpoints := []struct {
		id, name, kind, folderID, rootPath string
	}{
		{"thor", "AYN Thor", "android", cfg.SyncthingThorFolderID, cfg.ThorDir},
		{"windows", "Windows PC", "windows", cfg.SyncthingWindowsFolderID, cfg.WindowsDir},
	}
	for _, endpoint := range endpoints {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO endpoints (id, name, kind, folder_id, root_path, state, created_at, updated_at)
			VALUES (?, ?, ?, ?, ?, 'unknown', ?, ?)
			ON CONFLICT(id) DO UPDATE SET folder_id=excluded.folder_id, root_path=excluded.root_path, updated_at=excluded.updated_at`,
			endpoint.id, endpoint.name, endpoint.kind, endpoint.folderID, endpoint.rootPath, now, now)
		if err != nil {
			return err
		}
	}
	for _, profile := range adapter.Profiles() {
		_, err := s.db.ExecContext(ctx, `
			INSERT INTO emulator_profiles (id, name, endpoint_id, platform, extension, format)
			VALUES (?, ?, ?, ?, ?, ?)
			ON CONFLICT(id) DO UPDATE SET name=excluded.name, extension=excluded.extension, format=excluded.format`,
			profile.ID, profile.Name, profile.EndpointID, profile.Platform, profile.Extension, profile.Format)
		if err != nil {
			return err
		}
	}
	defaults := map[string]string{
		"propagation_enabled":       "false",
		"onboarding_complete":       "false",
		"syncthing_event_cursor":    "0",
		windowsGBAProfileSetting:    "windows-mgba",
		windowsGBAConfiguredSetting: "false",
	}
	for key, value := range defaults {
		_, err := s.db.ExecContext(ctx, "INSERT OR IGNORE INTO settings (key, value, updated_at) VALUES (?, ?, ?)", key, value, now)
		if err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) Setting(ctx context.Context, key string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, "SELECT value FROM settings WHERE key=?", key).Scan(&value)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

func (s *Store) SetSetting(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO settings (key, value, updated_at) VALUES (?, ?, ?)
		ON CONFLICT(key) DO UPDATE SET value=excluded.value, updated_at=excluded.updated_at`, key, value, time.Now().UTC())
	return err
}

func (s *Store) PropagationEnabled(ctx context.Context) bool {
	value, err := s.Setting(ctx, "propagation_enabled")
	return err == nil && value == "true"
}

func (s *Store) RecordActivity(ctx context.Context, gameID, kind, summary, endpointID string) error {
	_, err := s.db.ExecContext(ctx, "INSERT INTO activity (id, game_id, kind, summary, endpoint_id, created_at) VALUES (?, ?, ?, ?, ?, ?)",
		ids.New(), nullString(gameID), kind, summary, nullString(endpointID), time.Now().UTC())
	return err
}

func nullString(value string) any {
	if value == "" {
		return nil
	}
	return value
}

const schemaV1 = `
CREATE TABLE IF NOT EXISTS settings (
  key TEXT PRIMARY KEY,
  value TEXT NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS endpoints (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  kind TEXT NOT NULL,
  device_id TEXT NOT NULL DEFAULT '',
  folder_id TEXT NOT NULL,
  root_path TEXT NOT NULL,
  online INTEGER NOT NULL DEFAULT 0,
  last_seen_at DATETIME,
  state TEXT NOT NULL DEFAULT 'unknown',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS emulator_profiles (
  id TEXT PRIMARY KEY,
  name TEXT NOT NULL,
  endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
  platform TEXT NOT NULL CHECK(platform IN ('gba','nds')),
  extension TEXT NOT NULL,
  format TEXT NOT NULL
);
CREATE TABLE IF NOT EXISTS games (
  id TEXT PRIMARY KEY,
  title TEXT NOT NULL,
  platform TEXT NOT NULL CHECK(platform IN ('gba','nds')),
  crc32 TEXT NOT NULL DEFAULT '',
  sha1 TEXT NOT NULL DEFAULT '',
  artwork_path TEXT NOT NULL DEFAULT '',
  current_revision_id TEXT,
  status TEXT NOT NULL DEFAULT 'unconfigured',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE UNIQUE INDEX IF NOT EXISTS games_identity_crc ON games(platform, crc32) WHERE crc32 <> '';
CREATE UNIQUE INDEX IF NOT EXISTS games_identity_sha1 ON games(platform, sha1) WHERE sha1 <> '';
CREATE TABLE IF NOT EXISTS save_bindings (
  id TEXT PRIMARY KEY,
  game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
  profile_id TEXT NOT NULL REFERENCES emulator_profiles(id),
  relative_path TEXT NOT NULL,
  last_deployed_revision_id TEXT,
  enabled INTEGER NOT NULL DEFAULT 1,
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL,
  UNIQUE(endpoint_id, relative_path),
  UNIQUE(game_id, endpoint_id)
);
CREATE TABLE IF NOT EXISTS blobs (
  hash TEXT PRIMARY KEY,
  size INTEGER NOT NULL,
  created_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS revisions (
  id TEXT PRIMARY KEY,
  game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  parent_revision_id TEXT REFERENCES revisions(id),
  restored_from_id TEXT REFERENCES revisions(id),
  promoted_from_id TEXT REFERENCES revisions(id),
  blob_hash TEXT NOT NULL REFERENCES blobs(hash),
  size INTEGER NOT NULL,
  source_endpoint_id TEXT REFERENCES endpoints(id),
  source_modified_at DATETIME,
  observed_at DATETIME NOT NULL,
  provenance TEXT NOT NULL CHECK(provenance IN ('confirmed','inferred','unknown')),
  kind TEXT NOT NULL,
  state TEXT NOT NULL,
  actor TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS revisions_game_observed ON revisions(game_id, observed_at DESC);
CREATE TABLE IF NOT EXISTS revision_payloads (
  revision_id TEXT PRIMARY KEY REFERENCES revisions(id) ON DELETE CASCADE,
  battery_blob_hash TEXT NOT NULL REFERENCES blobs(hash),
  battery_size INTEGER NOT NULL,
  rtc_blob_hash TEXT REFERENCES blobs(hash),
  rtc_size INTEGER NOT NULL DEFAULT 0,
  content_hash TEXT NOT NULL
);
CREATE INDEX IF NOT EXISTS revision_payloads_content ON revision_payloads(content_hash);
CREATE TABLE IF NOT EXISTS observations (
  id TEXT PRIMARY KEY,
  game_id TEXT REFERENCES games(id) ON DELETE CASCADE,
  revision_id TEXT REFERENCES revisions(id),
  endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
  relative_path TEXT NOT NULL,
  blob_hash TEXT REFERENCES blobs(hash),
  action TEXT NOT NULL,
  source_modified_at DATETIME,
  observed_at DATETIME NOT NULL,
  provenance TEXT NOT NULL,
  detail TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS observations_game_time ON observations(game_id, observed_at DESC);
CREATE TABLE IF NOT EXISTS conflicts (
  id TEXT PRIMARY KEY,
  game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  head_revision_id TEXT REFERENCES revisions(id),
  branch_revision_id TEXT NOT NULL REFERENCES revisions(id),
  reason TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'open',
  created_at DATETIME NOT NULL,
  resolved_at DATETIME
);
CREATE INDEX IF NOT EXISTS conflicts_game_state ON conflicts(game_id, state);
CREATE TABLE IF NOT EXISTS broker_operations (
  id TEXT PRIMARY KEY,
  idempotency_key TEXT UNIQUE,
  game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  revision_id TEXT NOT NULL REFERENCES revisions(id),
  target_endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
  relative_path TEXT NOT NULL,
  profile_id TEXT NOT NULL DEFAULT '',
  blob_hash TEXT NOT NULL REFERENCES blobs(hash),
  state TEXT NOT NULL,
  error TEXT NOT NULL DEFAULT '',
  created_at DATETIME NOT NULL,
  updated_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS operations_echo ON broker_operations(target_endpoint_id, relative_path, blob_hash, state);
CREATE TABLE IF NOT EXISTS mutation_requests (
  idempotency_key TEXT PRIMARY KEY,
  game_id TEXT NOT NULL REFERENCES games(id) ON DELETE CASCADE,
  revision_id TEXT NOT NULL REFERENCES revisions(id),
  created_at DATETIME NOT NULL
);
CREATE TABLE IF NOT EXISTS unassigned_files (
  id TEXT PRIMARY KEY,
  endpoint_id TEXT NOT NULL REFERENCES endpoints(id),
  relative_path TEXT NOT NULL,
  blob_hash TEXT REFERENCES blobs(hash),
  size INTEGER NOT NULL DEFAULT 0,
  source_modified_at DATETIME,
  observed_at DATETIME NOT NULL,
  provenance TEXT NOT NULL,
  state TEXT NOT NULL DEFAULT 'unassigned',
  detail TEXT NOT NULL DEFAULT '',
  UNIQUE(endpoint_id, relative_path)
);
CREATE TABLE IF NOT EXISTS activity (
  id TEXT PRIMARY KEY,
  game_id TEXT REFERENCES games(id) ON DELETE CASCADE,
  kind TEXT NOT NULL,
  summary TEXT NOT NULL,
  endpoint_id TEXT REFERENCES endpoints(id),
  created_at DATETIME NOT NULL
);
CREATE INDEX IF NOT EXISTS activity_time ON activity(created_at DESC);
PRAGMA user_version=1;
`

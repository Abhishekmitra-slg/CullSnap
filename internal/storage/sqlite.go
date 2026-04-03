package storage

import (
	"database/sql"
	"fmt"
	"sync"
	"time"

	_ "modernc.org/sqlite"
)

type SQLiteStore struct {
	db *sql.DB
	mu sync.RWMutex
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	// Performance pragmas — applied once per connection.
	pragmas := []string{
		"PRAGMA journal_mode = WAL",
		"PRAGMA synchronous = NORMAL",
		"PRAGMA cache_size = -64000",
		"PRAGMA busy_timeout = 5000",
		"PRAGMA foreign_keys = ON",
		"PRAGMA temp_store = MEMORY",
	}
	for _, p := range pragmas {
		if _, err := db.Exec(p); err != nil {
			_ = db.Close()
			return nil, fmt.Errorf("failed to set pragma %q: %w", p, err)
		}
	}

	// Connection pool configuration for SQLite.
	// SQLite supports one writer at a time; keep pool small to avoid contention.
	db.SetMaxOpenConns(4)
	db.SetMaxIdleConns(2)

	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		_ = db.Close()
		return nil, err
	}

	return store, nil
}

func (s *SQLiteStore) initSchema() error {
	queries := []string{
		`CREATE TABLE IF NOT EXISTS selections (
			path TEXT PRIMARY KEY,
			session_id TEXT,
			selected_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS recents (
			path TEXT PRIMARY KEY,
			accessed_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS exported (
			path TEXT PRIMARY KEY,
			exported_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS ratings (
			path TEXT PRIMARY KEY,
			rating INTEGER DEFAULT 0,
			updated_at DATETIME
		);`,
		`CREATE TABLE IF NOT EXISTS app_config (
    key   TEXT PRIMARY KEY,
    value TEXT NOT NULL
);`,
		`CREATE TABLE IF NOT EXISTS cloud_mirrors (
    provider_id  TEXT NOT NULL,
    album_id     TEXT NOT NULL,
    album_title  TEXT NOT NULL,
    local_path   TEXT NOT NULL,
    synced_at    DATETIME,
    PRIMARY KEY (provider_id, album_id)
);`,
		`CREATE TABLE IF NOT EXISTS cloud_media_meta (
    local_path        TEXT PRIMARY KEY,
    remote_id         TEXT NOT NULL,
    provider_id       TEXT NOT NULL,
    remote_updated_at DATETIME
);`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) SaveSelection(path string, sessionID string, selected bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if selected {
		query := `INSERT OR REPLACE INTO selections (path, session_id, selected_at) VALUES (?, ?, ?)`
		_, err := s.db.Exec(query, path, sessionID, time.Now())
		if err != nil {
			return fmt.Errorf("failed to save selection: %w", err)
		}
	} else {
		query := `DELETE FROM selections WHERE path = ?`
		_, err := s.db.Exec(query, path)
		if err != nil {
			return fmt.Errorf("failed to remove selection: %w", err)
		}
	}
	return nil
}

func (s *SQLiteStore) GetSelections(sessionID string) (map[string]bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT path FROM selections WHERE session_id = ?", sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query selections: %w", err)
	}
	defer func() { _ = rows.Close() }()

	selections := make(map[string]bool)
	for rows.Next() {
		var path string
		if err := rows.Scan(&path); err != nil {
			return nil, err
		}
		selections[path] = true
	}
	return selections, nil
}

func (s *SQLiteStore) AddRecent(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Add or update timestamp
	query := `INSERT OR REPLACE INTO recents (path, accessed_at) VALUES (?, ?)`
	_, err := s.db.Exec(query, path, time.Now())
	if err != nil {
		return err
	}

	// Keep only top 30
	cleanup := `DELETE FROM recents WHERE path NOT IN (
		SELECT path FROM recents ORDER BY accessed_at DESC LIMIT 30
	)`
	_, err = s.db.Exec(cleanup)
	return err
}

func (s *SQLiteStore) GetRecents() ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query("SELECT path FROM recents ORDER BY accessed_at DESC")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	var paths []string
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		paths = append(paths, p)
	}
	return paths, nil
}

func (s *SQLiteStore) MarkExported(path string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT OR REPLACE INTO exported (path, exported_at) VALUES (?, ?)`
	_, err := s.db.Exec(query, path, time.Now())
	return err
}

func (s *SQLiteStore) GetExportedInDirectory(dirPath string) (map[string]bool, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Append slash if missing to ensure prefix match
	// Actually, just LIKE 'dir/%'
	query := "SELECT path FROM exported WHERE path LIKE ?"
	rows, err := s.db.Query(query, dirPath+"/%")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	exported := make(map[string]bool)
	for rows.Next() {
		var p string
		if err := rows.Scan(&p); err != nil {
			return nil, err
		}
		exported[p] = true
	}
	return exported, nil
}

func (s *SQLiteStore) Close() error {
	return s.db.Close()
}

// SaveRating persists a star rating (0-5) for a photo path.
func (s *SQLiteStore) SaveRating(path string, rating int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if rating <= 0 {
		_, err := s.db.Exec("DELETE FROM ratings WHERE path = ?", path)
		return err
	}
	query := `INSERT OR REPLACE INTO ratings (path, rating, updated_at) VALUES (?, ?, ?)`
	_, err := s.db.Exec(query, path, rating, time.Now())
	return err
}

// GetRatingsInDirectory returns all ratings for photos in a directory.
func (s *SQLiteStore) GetRatingsInDirectory(dirPath string) (map[string]int, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	query := "SELECT path, rating FROM ratings WHERE path LIKE ?"
	rows, err := s.db.Query(query, dirPath+"/%")
	if err != nil {
		return nil, err
	}
	defer func() { _ = rows.Close() }()

	ratings := make(map[string]int)
	for rows.Next() {
		var p string
		var r int
		if err := rows.Scan(&p, &r); err != nil {
			return nil, err
		}
		ratings[p] = r
	}
	return ratings, nil
}

// GetSQLiteVersion returns the SQLite library version.
func (s *SQLiteStore) GetSQLiteVersion() (string, error) {
	var ver string
	err := s.db.QueryRow("SELECT sqlite_version()").Scan(&ver)
	return ver, err
}

// CloudMirror represents a synced cloud album mapped to a local directory.
type CloudMirror struct {
	ProviderID string
	AlbumID    string
	AlbumTitle string
	LocalPath  string
	SyncedAt   time.Time
}

// CloudMediaMeta holds remote metadata for a locally known media file.
type CloudMediaMeta struct {
	LocalPath       string
	RemoteID        string
	ProviderID      string
	RemoteUpdatedAt time.Time
}

// SaveCloudMirror upserts a cloud mirror record.
func (s *SQLiteStore) SaveCloudMirror(providerID, albumID, albumTitle, localPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT OR REPLACE INTO cloud_mirrors (provider_id, album_id, album_title, local_path, synced_at)
	          VALUES (?, ?, ?, ?, ?)`
	_, err := s.db.Exec(query, providerID, albumID, albumTitle, localPath, time.Now())
	if err != nil {
		return fmt.Errorf("failed to save cloud mirror: %w", err)
	}
	return nil
}

// GetCloudMirror retrieves a cloud mirror by provider and album ID.
func (s *SQLiteStore) GetCloudMirror(providerID, albumID string) (CloudMirror, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var m CloudMirror
	query := `SELECT provider_id, album_id, album_title, local_path, synced_at
	          FROM cloud_mirrors WHERE provider_id = ? AND album_id = ?`
	err := s.db.QueryRow(query, providerID, albumID).Scan(
		&m.ProviderID, &m.AlbumID, &m.AlbumTitle, &m.LocalPath, &m.SyncedAt,
	)
	if err != nil {
		return CloudMirror{}, fmt.Errorf("failed to get cloud mirror: %w", err)
	}
	return m, nil
}

// ListCloudMirrors returns all stored cloud mirrors.
func (s *SQLiteStore) ListCloudMirrors() ([]CloudMirror, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(`SELECT provider_id, album_id, album_title, local_path, synced_at FROM cloud_mirrors`)
	if err != nil {
		return nil, fmt.Errorf("failed to list cloud mirrors: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var mirrors []CloudMirror
	for rows.Next() {
		var m CloudMirror
		if err := rows.Scan(&m.ProviderID, &m.AlbumID, &m.AlbumTitle, &m.LocalPath, &m.SyncedAt); err != nil {
			return nil, fmt.Errorf("failed to scan cloud mirror row: %w", err)
		}
		mirrors = append(mirrors, m)
	}
	return mirrors, nil
}

// DeleteCloudMirror removes a cloud mirror record.
func (s *SQLiteStore) DeleteCloudMirror(providerID, albumID string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM cloud_mirrors WHERE provider_id = ? AND album_id = ?`, providerID, albumID)
	if err != nil {
		return fmt.Errorf("failed to delete cloud mirror: %w", err)
	}
	return nil
}

// SaveCloudMediaMeta upserts remote metadata for a local media file.
func (s *SQLiteStore) SaveCloudMediaMeta(localPath, remoteID, providerID string, remoteUpdatedAt time.Time) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	query := `INSERT OR REPLACE INTO cloud_media_meta (local_path, remote_id, provider_id, remote_updated_at)
	          VALUES (?, ?, ?, ?)`
	_, err := s.db.Exec(query, localPath, remoteID, providerID, remoteUpdatedAt)
	if err != nil {
		return fmt.Errorf("failed to save cloud media meta: %w", err)
	}
	return nil
}

// DeleteCloudMediaMetaByPrefix removes all cloud media metadata entries whose
// local_path starts with the given prefix. Used when deleting an album's cache.
func (s *SQLiteStore) DeleteCloudMediaMetaByPrefix(pathPrefix string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(`DELETE FROM cloud_media_meta WHERE local_path LIKE ?`, pathPrefix+"%")
	if err != nil {
		return fmt.Errorf("failed to delete cloud media meta by prefix: %w", err)
	}
	return nil
}

// GetCloudMediaMeta retrieves remote metadata for a local media file.
func (s *SQLiteStore) GetCloudMediaMeta(localPath string) (CloudMediaMeta, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var m CloudMediaMeta
	query := `SELECT local_path, remote_id, provider_id, remote_updated_at
	          FROM cloud_media_meta WHERE local_path = ?`
	err := s.db.QueryRow(query, localPath).Scan(
		&m.LocalPath, &m.RemoteID, &m.ProviderID, &m.RemoteUpdatedAt,
	)
	if err != nil {
		return CloudMediaMeta{}, fmt.Errorf("failed to get cloud media meta: %w", err)
	}
	return m, nil
}

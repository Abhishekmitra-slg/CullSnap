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
	mu sync.Mutex
}

func NewSQLiteStore(dbPath string) (*SQLiteStore, error) {
	db, err := sql.Open("sqlite", dbPath)
	if err != nil {
		return nil, fmt.Errorf("failed to open database: %w", err)
	}

	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("failed to ping database: %w", err)
	}

	store := &SQLiteStore{db: db}
	if err := store.initSchema(); err != nil {
		db.Close()
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
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT path FROM selections WHERE session_id = ?", sessionID)
	if err != nil {
		return nil, fmt.Errorf("failed to query selections: %w", err)
	}
	defer rows.Close()

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
	s.mu.Lock()
	defer s.mu.Unlock()

	rows, err := s.db.Query("SELECT path FROM recents ORDER BY accessed_at DESC")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
	s.mu.Lock()
	defer s.mu.Unlock()

	// Append slash if missing to ensure prefix match
	// Actually, just LIKE 'dir/%'
	query := "SELECT path FROM exported WHERE path LIKE ?"
	rows, err := s.db.Query(query, dirPath+"/%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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
	s.mu.Lock()
	defer s.mu.Unlock()

	query := "SELECT path, rating FROM ratings WHERE path LIKE ?"
	rows, err := s.db.Query(query, dirPath+"/%")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

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

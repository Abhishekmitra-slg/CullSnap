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
		`CREATE TABLE IF NOT EXISTS ai_scores (
    photo_path TEXT PRIMARY KEY,
    overall_score REAL NOT NULL,
    face_count INTEGER NOT NULL DEFAULT 0,
    provider TEXT NOT NULL,
    scored_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP
);`,
		`CREATE TABLE IF NOT EXISTS face_clusters (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    folder_path TEXT NOT NULL,
    label TEXT NOT NULL,
    representative_path TEXT,
    photo_count INTEGER NOT NULL DEFAULT 0,
    hidden INTEGER NOT NULL DEFAULT 0
);`,
		`CREATE TABLE IF NOT EXISTS face_detections (
    id INTEGER PRIMARY KEY AUTOINCREMENT,
    photo_path TEXT NOT NULL,
    cluster_id INTEGER,
    bbox_x INTEGER NOT NULL,
    bbox_y INTEGER NOT NULL,
    bbox_w INTEGER NOT NULL,
    bbox_h INTEGER NOT NULL,
    eye_sharpness REAL NOT NULL DEFAULT 0,
    eyes_open INTEGER NOT NULL DEFAULT 0,
    expression REAL NOT NULL DEFAULT 0,
    confidence REAL NOT NULL DEFAULT 0,
    embedding BLOB,
    FOREIGN KEY (cluster_id) REFERENCES face_clusters(id) ON DELETE SET NULL
);`,
		`CREATE INDEX IF NOT EXISTS idx_face_detections_photo ON face_detections(photo_path);`,
		`CREATE INDEX IF NOT EXISTS idx_face_detections_cluster ON face_detections(cluster_id);`,
		`CREATE INDEX IF NOT EXISTS idx_face_clusters_folder ON face_clusters(folder_path);`,
		`CREATE INDEX IF NOT EXISTS idx_ai_scores_score ON ai_scores(overall_score);`,
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

// AIScore stores the AI quality analysis result for a single photo.
type AIScore struct {
	PhotoPath    string    `json:"photoPath"`
	OverallScore float64   `json:"overallScore"`
	FaceCount    int       `json:"faceCount"`
	Provider     string    `json:"provider"`
	ScoredAt     time.Time `json:"scoredAt"`
}

// FaceDetection stores a single detected face within a photo.
type FaceDetection struct {
	ID           int64   `json:"id"`
	PhotoPath    string  `json:"photoPath"`
	ClusterID    *int64  `json:"clusterId"`
	BboxX        int     `json:"bboxX"`
	BboxY        int     `json:"bboxY"`
	BboxW        int     `json:"bboxW"`
	BboxH        int     `json:"bboxH"`
	EyeSharpness float64 `json:"eyeSharpness"`
	EyesOpen     bool    `json:"eyesOpen"`
	Expression   float64 `json:"expression"`
	Confidence   float64 `json:"confidence"`
	Embedding    []byte  `json:"embedding,omitempty"`
}

// FaceCluster represents a group of detected faces identified as the same person.
type FaceCluster struct {
	ID                 int64  `json:"id"`
	FolderPath         string `json:"folderPath"`
	Label              string `json:"label"`
	RepresentativePath string `json:"representativePath"`
	PhotoCount         int    `json:"photoCount"`
	Hidden             bool   `json:"hidden"`
}

// SaveAIScore inserts or replaces an AI quality score for a photo.
func (s *SQLiteStore) SaveAIScore(photoPath string, overallScore float64, faceCount int, provider string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO ai_scores (photo_path, overall_score, face_count, provider, scored_at) VALUES (?, ?, ?, ?, ?)`,
		photoPath, overallScore, faceCount, provider, time.Now(),
	)
	if err != nil {
		return fmt.Errorf("failed to save AI score: %w", err)
	}
	return nil
}

// GetAIScore retrieves the AI score for a single photo, or nil if not scored.
func (s *SQLiteStore) GetAIScore(photoPath string) (*AIScore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	row := s.db.QueryRow(`SELECT photo_path, overall_score, face_count, provider, scored_at FROM ai_scores WHERE photo_path = ?`, photoPath)
	var score AIScore
	err := row.Scan(&score.PhotoPath, &score.OverallScore, &score.FaceCount, &score.Provider, &score.ScoredAt)
	if err != nil {
		if err == sql.ErrNoRows {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get AI score: %w", err)
	}
	return &score, nil
}

// GetAIScoresForFolder retrieves all AI scores for photos within a folder prefix.
func (s *SQLiteStore) GetAIScoresForFolder(folderPath string) ([]AIScore, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(
		`SELECT photo_path, overall_score, face_count, provider, scored_at FROM ai_scores WHERE photo_path LIKE ?`,
		folderPath+"/%",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI scores for folder: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var scores []AIScore
	for rows.Next() {
		var score AIScore
		if err := rows.Scan(&score.PhotoPath, &score.OverallScore, &score.FaceCount, &score.Provider, &score.ScoredAt); err != nil {
			return nil, fmt.Errorf("failed to scan AI score: %w", err)
		}
		scores = append(scores, score)
	}
	return scores, nil
}

// SaveFaceDetection inserts a face detection record and returns the new ID.
func (s *SQLiteStore) SaveFaceDetection(det *FaceDetection) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	eyesOpenInt := 0
	if det.EyesOpen {
		eyesOpenInt = 1
	}
	result, err := s.db.Exec(
		`INSERT INTO face_detections (photo_path, cluster_id, bbox_x, bbox_y, bbox_w, bbox_h, eye_sharpness, eyes_open, expression, confidence, embedding)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		det.PhotoPath, det.ClusterID, det.BboxX, det.BboxY, det.BboxW, det.BboxH,
		det.EyeSharpness, eyesOpenInt, det.Expression, det.Confidence, det.Embedding,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to save face detection: %w", err)
	}
	return result.LastInsertId()
}

// GetFaceDetections retrieves all face detections for a photo.
func (s *SQLiteStore) GetFaceDetections(photoPath string) ([]FaceDetection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(
		`SELECT id, photo_path, cluster_id, bbox_x, bbox_y, bbox_w, bbox_h, eye_sharpness, eyes_open, expression, confidence, embedding
		 FROM face_detections WHERE photo_path = ?`, photoPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get face detections: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var dets []FaceDetection
	for rows.Next() {
		var det FaceDetection
		var eyesOpenInt int
		if err := rows.Scan(&det.ID, &det.PhotoPath, &det.ClusterID, &det.BboxX, &det.BboxY, &det.BboxW, &det.BboxH,
			&det.EyeSharpness, &eyesOpenInt, &det.Expression, &det.Confidence, &det.Embedding); err != nil {
			return nil, fmt.Errorf("failed to scan face detection: %w", err)
		}
		det.EyesOpen = eyesOpenInt != 0
		dets = append(dets, det)
	}
	return dets, nil
}

// SaveFaceCluster inserts a face cluster and returns the new ID.
func (s *SQLiteStore) SaveFaceCluster(cluster *FaceCluster) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result, err := s.db.Exec(
		`INSERT INTO face_clusters (folder_path, label, representative_path, photo_count, hidden) VALUES (?, ?, ?, ?, 0)`,
		cluster.FolderPath, cluster.Label, cluster.RepresentativePath, cluster.PhotoCount,
	)
	if err != nil {
		return 0, fmt.Errorf("failed to save face cluster: %w", err)
	}
	return result.LastInsertId()
}

// GetFaceClusters retrieves visible (non-hidden) face clusters for a folder.
func (s *SQLiteStore) GetFaceClusters(folderPath string) ([]FaceCluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(
		`SELECT id, folder_path, label, representative_path, photo_count, hidden FROM face_clusters WHERE folder_path = ? AND hidden = 0`,
		folderPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get face clusters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanFaceClusters(rows)
}

// GetAllFaceClusters retrieves all face clusters for a folder (including hidden).
func (s *SQLiteStore) GetAllFaceClusters(folderPath string) ([]FaceCluster, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(
		`SELECT id, folder_path, label, representative_path, photo_count, hidden FROM face_clusters WHERE folder_path = ?`,
		folderPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get all face clusters: %w", err)
	}
	defer func() { _ = rows.Close() }()
	return scanFaceClusters(rows)
}

func scanFaceClusters(rows *sql.Rows) ([]FaceCluster, error) {
	var clusters []FaceCluster
	for rows.Next() {
		var c FaceCluster
		var hiddenInt int
		if err := rows.Scan(&c.ID, &c.FolderPath, &c.Label, &c.RepresentativePath, &c.PhotoCount, &hiddenInt); err != nil {
			return nil, fmt.Errorf("failed to scan face cluster: %w", err)
		}
		c.Hidden = hiddenInt != 0
		clusters = append(clusters, c)
	}
	return clusters, nil
}

// RenameFaceCluster updates the label for a face cluster.
func (s *SQLiteStore) RenameFaceCluster(clusterID int64, label string) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(`UPDATE face_clusters SET label = ? WHERE id = ?`, label, clusterID)
	if err != nil {
		return fmt.Errorf("failed to rename face cluster: %w", err)
	}
	return nil
}

// MergeFaceClusters merges sourceID into targetID: reassigns detections, sums counts, deletes source.
func (s *SQLiteStore) MergeFaceClusters(sourceID, targetID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Reassign all detections from source to target
	_, err := s.db.Exec(`UPDATE face_detections SET cluster_id = ? WHERE cluster_id = ?`, targetID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to reassign detections: %w", err)
	}

	// Update target photo count
	_, err = s.db.Exec(`UPDATE face_clusters SET photo_count = photo_count + (SELECT photo_count FROM face_clusters WHERE id = ?) WHERE id = ?`, sourceID, targetID)
	if err != nil {
		return fmt.Errorf("failed to update photo count: %w", err)
	}

	// Delete source cluster
	_, err = s.db.Exec(`DELETE FROM face_clusters WHERE id = ?`, sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete source cluster: %w", err)
	}

	return nil
}

// HideFaceCluster sets the hidden flag on a cluster.
func (s *SQLiteStore) HideFaceCluster(clusterID int64, hidden bool) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	hiddenInt := 0
	if hidden {
		hiddenInt = 1
	}
	_, err := s.db.Exec(`UPDATE face_clusters SET hidden = ? WHERE id = ?`, hiddenInt, clusterID)
	if err != nil {
		return fmt.Errorf("failed to hide face cluster: %w", err)
	}
	return nil
}

// DeleteAIDataForFolder removes all AI scores, face detections, and face clusters for a folder.
func (s *SQLiteStore) DeleteAIDataForFolder(folderPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Delete detections for photos in this folder
	_, err := s.db.Exec(`DELETE FROM face_detections WHERE photo_path LIKE ?`, folderPath+"/%")
	if err != nil {
		return fmt.Errorf("failed to delete face detections: %w", err)
	}

	// Delete clusters for this folder
	_, err = s.db.Exec(`DELETE FROM face_clusters WHERE folder_path = ?`, folderPath)
	if err != nil {
		return fmt.Errorf("failed to delete face clusters: %w", err)
	}

	// Delete scores for photos in this folder
	_, err = s.db.Exec(`DELETE FROM ai_scores WHERE photo_path LIKE ?`, folderPath+"/%")
	if err != nil {
		return fmt.Errorf("failed to delete AI scores: %w", err)
	}

	return nil
}

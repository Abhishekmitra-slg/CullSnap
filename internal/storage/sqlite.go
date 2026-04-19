package storage

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
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
		`CREATE TABLE IF NOT EXISTS vlm_scores (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			photo_path TEXT NOT NULL UNIQUE,
			folder_path TEXT NOT NULL,
			aesthetic REAL NOT NULL DEFAULT 0,
			composition REAL NOT NULL DEFAULT 0,
			expression REAL NOT NULL DEFAULT 0,
			technical_quality REAL NOT NULL DEFAULT 0,
			scene_type TEXT NOT NULL DEFAULT '',
			issues TEXT NOT NULL DEFAULT '',
			explanation TEXT NOT NULL DEFAULT '',
			tokens_used INTEGER NOT NULL DEFAULT 0,
			model_name TEXT NOT NULL DEFAULT '',
			model_variant TEXT NOT NULL DEFAULT '',
			backend TEXT NOT NULL DEFAULT '',
			prompt_version INTEGER NOT NULL DEFAULT 0,
			custom_instructions_hash TEXT NOT NULL DEFAULT '',
			scored_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE TABLE IF NOT EXISTS vlm_rankings (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			folder_path TEXT NOT NULL,
			group_label TEXT NOT NULL,
			photo_path TEXT NOT NULL,
			rank INTEGER NOT NULL,
			relative_score REAL NOT NULL DEFAULT 0,
			notes TEXT NOT NULL DEFAULT '',
			tokens_used INTEGER NOT NULL DEFAULT 0,
			model_name TEXT NOT NULL DEFAULT '',
			prompt_version INTEGER NOT NULL DEFAULT 0,
			custom_instructions_hash TEXT NOT NULL DEFAULT '',
			ranked_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(folder_path, group_label, photo_path)
		);`,
		`CREATE TABLE IF NOT EXISTS vlm_ranking_groups (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			folder_path TEXT NOT NULL,
			group_label TEXT NOT NULL,
			photo_count INTEGER NOT NULL DEFAULT 0,
			explanation TEXT NOT NULL DEFAULT '',
			model_name TEXT NOT NULL DEFAULT '',
			prompt_version INTEGER NOT NULL DEFAULT 0,
			custom_instructions_hash TEXT NOT NULL DEFAULT '',
			ranked_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(folder_path, group_label)
		);`,
		`CREATE TABLE IF NOT EXISTS vlm_token_usage (
			id INTEGER PRIMARY KEY AUTOINCREMENT,
			provider TEXT NOT NULL DEFAULT '',
			folder_path TEXT NOT NULL DEFAULT '',
			stage TEXT NOT NULL DEFAULT '',
			tokens_input INTEGER NOT NULL DEFAULT 0,
			tokens_output INTEGER NOT NULL DEFAULT 0,
			photo_count INTEGER NOT NULL DEFAULT 0,
			recorded_at TEXT NOT NULL DEFAULT CURRENT_TIMESTAMP
		);`,
		`CREATE INDEX IF NOT EXISTS idx_vlm_scores_folder ON vlm_scores(folder_path);`,
		`CREATE INDEX IF NOT EXISTS idx_vlm_scores_prompt_ver ON vlm_scores(prompt_version);`,
		`CREATE INDEX IF NOT EXISTS idx_vlm_rankings_folder ON vlm_rankings(folder_path);`,
		`CREATE INDEX IF NOT EXISTS idx_vlm_token_usage_provider ON vlm_token_usage(provider);`,
	}

	for _, query := range queries {
		if _, err := s.db.Exec(query); err != nil {
			return fmt.Errorf("failed to create schema: %w", err)
		}
	}

	// Add sub-score columns to ai_scores (ignore duplicate column errors for existing DBs).
	alterStatements := []string{
		"ALTER TABLE ai_scores ADD COLUMN aesthetic_score REAL NOT NULL DEFAULT 0",
		"ALTER TABLE ai_scores ADD COLUMN sharpness_score REAL NOT NULL DEFAULT 0",
		"ALTER TABLE ai_scores ADD COLUMN best_face_sharpness REAL NOT NULL DEFAULT 0",
		"ALTER TABLE ai_scores ADD COLUMN eye_openness REAL NOT NULL DEFAULT 0",
		"ALTER TABLE face_clusters ADD COLUMN centroid BLOB",
		// custom_instructions_hash columns for VLM cache invalidation (#114).
		// Default '' matches the sentinel returned by vlm.HashCustomInstructions("")
		// so legacy rows remain valid when the user has no custom instructions set.
		"ALTER TABLE vlm_scores ADD COLUMN custom_instructions_hash TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE vlm_rankings ADD COLUMN custom_instructions_hash TEXT NOT NULL DEFAULT ''",
		"ALTER TABLE vlm_ranking_groups ADD COLUMN custom_instructions_hash TEXT NOT NULL DEFAULT ''",
	}
	for _, stmt := range alterStatements {
		if _, err := s.db.Exec(stmt); err != nil {
			if !strings.Contains(err.Error(), "duplicate column") {
				return fmt.Errorf("failed to migrate schema: %w", err)
			}
		}
	}

	// Create scoring_plugins table for per-plugin score provenance.
	_, err := s.db.Exec(`CREATE TABLE IF NOT EXISTS scoring_plugins (
		photo_path TEXT NOT NULL,
		plugin_name TEXT NOT NULL,
		version TEXT,
		scored_at DATETIME NOT NULL DEFAULT CURRENT_TIMESTAMP,
		PRIMARY KEY (photo_path, plugin_name)
	)`)
	if err != nil {
		return fmt.Errorf("failed to create scoring_plugins table: %w", err)
	}

	// Clean up stale NIMA scores — NIMA replaced by VLM.
	s.db.Exec("DELETE FROM scoring_plugins WHERE plugin_name = 'NIMA'") //nolint:errcheck

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
	PhotoPath         string    `json:"photoPath"`
	OverallScore      float64   `json:"overallScore"`
	FaceCount         int       `json:"faceCount"`
	Provider          string    `json:"provider"`
	ScoredAt          time.Time `json:"scoredAt"`
	AestheticScore    float64   `json:"aestheticScore"`
	SharpnessScore    float64   `json:"sharpnessScore"`
	BestFaceSharpness float64   `json:"bestFaceSharpness"`
	EyeOpenness       float64   `json:"eyeOpenness"`
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

// VLMScoreRow stores a single VLM scoring result.
type VLMScoreRow struct {
	PhotoPath              string  `json:"photoPath"`
	FolderPath             string  `json:"folderPath"`
	Aesthetic              float64 `json:"aesthetic"`
	Composition            float64 `json:"composition"`
	Expression             float64 `json:"expression"`
	TechnicalQual          float64 `json:"technicalQuality"`
	SceneType              string  `json:"sceneType"`
	Issues                 string  `json:"issues"` // JSON array string
	Explanation            string  `json:"explanation"`
	TokensUsed             int     `json:"tokensUsed"`
	ModelName              string  `json:"modelName"`
	ModelVariant           string  `json:"modelVariant"`
	Backend                string  `json:"backend"`
	PromptVersion          int     `json:"promptVersion"`
	CustomInstructionsHash string  `json:"customInstructionsHash"`
	ScoredAt               string  `json:"scoredAt"`
}

// VLMRankingRow stores a single photo's rank within a ranking group.
type VLMRankingRow struct {
	PhotoPath     string  `json:"photoPath"`
	Rank          int     `json:"rank"`
	RelativeScore float64 `json:"relativeScore"`
	Notes         string  `json:"notes"`
	TokensUsed    int     `json:"tokensUsed"`
}

// VLMRankingGroupRow stores a ranking group with its ranked photos.
type VLMRankingGroupRow struct {
	FolderPath             string          `json:"folderPath"`
	GroupLabel             string          `json:"groupLabel"`
	PhotoCount             int             `json:"photoCount"`
	Explanation            string          `json:"explanation"`
	ModelName              string          `json:"modelName"`
	PromptVersion          int             `json:"promptVersion"`
	CustomInstructionsHash string          `json:"customInstructionsHash"`
	Rankings               []VLMRankingRow `json:"rankings"`
}

// TokenUsageRow records token consumption for a batch of VLM calls.
type TokenUsageRow struct {
	Provider     string `json:"provider"`
	FolderPath   string `json:"folderPath"`
	Stage        string `json:"stage"`
	TokensInput  int    `json:"tokensInput"`
	TokensOutput int    `json:"tokensOutput"`
	PhotoCount   int    `json:"photoCount"`
}

// TokenUsageSummary aggregates token usage per provider.
type TokenUsageSummary struct {
	Provider     string `json:"provider"`
	TotalInput   int    `json:"totalInput"`
	TotalOutput  int    `json:"totalOutput"`
	TotalPhotos  int    `json:"totalPhotos"`
	SessionCount int    `json:"sessionCount"`
}

// SaveAIScore inserts or replaces an AI quality score for a photo.
func (s *SQLiteStore) SaveAIScore(score *AIScore) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO ai_scores
		 (photo_path, overall_score, face_count, provider, scored_at,
		  aesthetic_score, sharpness_score, best_face_sharpness, eye_openness)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		score.PhotoPath, score.OverallScore, score.FaceCount, score.Provider, time.Now(),
		score.AestheticScore, score.SharpnessScore, score.BestFaceSharpness, score.EyeOpenness,
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
	row := s.db.QueryRow(
		`SELECT photo_path, overall_score, face_count, provider, scored_at,
		        COALESCE(aesthetic_score, 0), COALESCE(sharpness_score, 0),
		        COALESCE(best_face_sharpness, 0), COALESCE(eye_openness, 0)
		 FROM ai_scores WHERE photo_path = ?`, photoPath)
	var score AIScore
	err := row.Scan(
		&score.PhotoPath, &score.OverallScore, &score.FaceCount, &score.Provider, &score.ScoredAt,
		&score.AestheticScore, &score.SharpnessScore, &score.BestFaceSharpness, &score.EyeOpenness,
	)
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
		`SELECT photo_path, overall_score, face_count, provider, scored_at,
		        COALESCE(aesthetic_score, 0), COALESCE(sharpness_score, 0),
		        COALESCE(best_face_sharpness, 0), COALESCE(eye_openness, 0)
		 FROM ai_scores WHERE photo_path LIKE ?`,
		folderPath+"/%",
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get AI scores for folder: %w", err)
	}
	defer func() { _ = rows.Close() }()
	var scores []AIScore
	for rows.Next() {
		var score AIScore
		if err := rows.Scan(
			&score.PhotoPath, &score.OverallScore, &score.FaceCount, &score.Provider, &score.ScoredAt,
			&score.AestheticScore, &score.SharpnessScore, &score.BestFaceSharpness, &score.EyeOpenness,
		); err != nil {
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

// GetFaceDetectionsForCluster retrieves all face detections assigned to a cluster.
func (s *SQLiteStore) GetFaceDetectionsForCluster(clusterID int64) ([]FaceDetection, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	rows, err := s.db.Query(
		`SELECT id, photo_path, cluster_id, bbox_x, bbox_y, bbox_w, bbox_h, eye_sharpness, eyes_open, expression, confidence, embedding
		 FROM face_detections WHERE cluster_id = ?`, clusterID,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get face detections for cluster: %w", err)
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
// All three mutations are wrapped in a single transaction for atomicity.
func (s *SQLiteStore) MergeFaceClusters(sourceID, targetID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after commit

	// Reassign all detections from source to target.
	_, err = tx.Exec(`UPDATE face_detections SET cluster_id = ? WHERE cluster_id = ?`, targetID, sourceID)
	if err != nil {
		return fmt.Errorf("failed to reassign detections: %w", err)
	}

	// Update target photo count.
	_, err = tx.Exec(`UPDATE face_clusters SET photo_count = photo_count + (SELECT photo_count FROM face_clusters WHERE id = ?) WHERE id = ?`, sourceID, targetID)
	if err != nil {
		return fmt.Errorf("failed to update photo count: %w", err)
	}

	// Delete source cluster.
	_, err = tx.Exec(`DELETE FROM face_clusters WHERE id = ?`, sourceID)
	if err != nil {
		return fmt.Errorf("failed to delete source cluster: %w", err)
	}

	return tx.Commit()
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

// DeleteAIDataForFolder removes every trace of AI analysis for a folder —
// ONNX outputs (face detections, face clusters, ai_scores) and VLM outputs
// (vlm_scores, vlm_rankings, vlm_ranking_groups). The six DELETEs run inside
// a single transaction so a failure partway through cannot leave users with
// e.g. cleared face data but stale VLM explanations still showing in the UI.
//
// Calls deleteVLMDataForFolderLocked for the VLM half rather than the public
// DeleteVLMDataForFolder to avoid re-entering s.mu (which would deadlock).
func (s *SQLiteStore) DeleteAIDataForFolder(folderPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin AI data delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after Commit

	if _, err := tx.Exec(`DELETE FROM face_detections WHERE photo_path LIKE ?`, folderPath+"/%"); err != nil {
		return fmt.Errorf("failed to delete face detections: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM face_clusters WHERE folder_path = ?`, folderPath); err != nil {
		return fmt.Errorf("failed to delete face clusters: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM ai_scores WHERE photo_path LIKE ?`, folderPath+"/%"); err != nil {
		return fmt.Errorf("failed to delete AI scores: %w", err)
	}
	if err := deleteVLMDataForFolderTx(tx, folderPath); err != nil {
		return err
	}
	return tx.Commit()
}

// DeleteFaceClustersForFolder removes face clusters for a folder and resets
// cluster_id on associated face_detections. Unlike DeleteAIDataForFolder this
// preserves AI scores and the face detections themselves.
func (s *SQLiteStore) DeleteFaceClustersForFolder(folderPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after commit

	// Reset cluster_id on detections belonging to clusters in this folder.
	_, err = tx.Exec(
		`UPDATE face_detections SET cluster_id = NULL
		 WHERE cluster_id IN (SELECT id FROM face_clusters WHERE folder_path = ?)`,
		folderPath,
	)
	if err != nil {
		return fmt.Errorf("reset cluster_id on detections: %w", err)
	}

	// Delete the clusters themselves.
	_, err = tx.Exec(`DELETE FROM face_clusters WHERE folder_path = ?`, folderPath)
	if err != nil {
		return fmt.Errorf("delete face clusters: %w", err)
	}

	return tx.Commit()
}

// AssignFaceToCluster sets the cluster_id for a face detection record.
func (s *SQLiteStore) AssignFaceToCluster(detectionID, clusterID int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, err := s.db.Exec("UPDATE face_detections SET cluster_id = ? WHERE id = ?", clusterID, detectionID)
	return err
}

// SaveVLMScore inserts or replaces a VLM score for a photo.
func (s *SQLiteStore) SaveVLMScore(score VLMScoreRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT OR REPLACE INTO vlm_scores
		 (photo_path, folder_path, aesthetic, composition, expression, technical_quality,
		  scene_type, issues, explanation, tokens_used, model_name, model_variant,
		  backend, prompt_version, custom_instructions_hash, scored_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		score.PhotoPath, score.FolderPath, score.Aesthetic, score.Composition, score.Expression,
		score.TechnicalQual, score.SceneType, score.Issues, score.Explanation, score.TokensUsed,
		score.ModelName, score.ModelVariant, score.Backend, score.PromptVersion,
		score.CustomInstructionsHash, score.ScoredAt,
	)
	if err != nil {
		return fmt.Errorf("failed to save VLM score: %w", err)
	}
	return nil
}

// GetVLMScore retrieves the VLM score for a single photo, or nil if not scored.
// Callers that need cache-hit semantics must compare the returned row's
// PromptVersion and CustomInstructionsHash against the current values — stale
// rows are returned unchanged so the UI can still display historic scores.
func (s *SQLiteStore) GetVLMScore(photoPath string) (*VLMScoreRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var row VLMScoreRow
	err := s.db.QueryRow(
		`SELECT photo_path, folder_path, aesthetic, composition, expression, technical_quality,
		        scene_type, issues, explanation, tokens_used, model_name, model_variant,
		        backend, prompt_version, custom_instructions_hash, scored_at
		 FROM vlm_scores WHERE photo_path = ?`, photoPath,
	).Scan(
		&row.PhotoPath, &row.FolderPath, &row.Aesthetic, &row.Composition, &row.Expression,
		&row.TechnicalQual, &row.SceneType, &row.Issues, &row.Explanation, &row.TokensUsed,
		&row.ModelName, &row.ModelVariant, &row.Backend, &row.PromptVersion,
		&row.CustomInstructionsHash, &row.ScoredAt,
	)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to get VLM score: %w", err)
	}
	return &row, nil
}

// GetVLMScoresForFolder retrieves all VLM scores for photos within a folder.
func (s *SQLiteStore) GetVLMScoresForFolder(folderPath string) ([]VLMScoreRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT photo_path, folder_path, aesthetic, composition, expression, technical_quality,
		        scene_type, issues, explanation, tokens_used, model_name, model_variant,
		        backend, prompt_version, custom_instructions_hash, scored_at
		 FROM vlm_scores WHERE folder_path = ?`, folderPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get VLM scores for folder: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var scores []VLMScoreRow
	for rows.Next() {
		var row VLMScoreRow
		if err := rows.Scan(
			&row.PhotoPath, &row.FolderPath, &row.Aesthetic, &row.Composition, &row.Expression,
			&row.TechnicalQual, &row.SceneType, &row.Issues, &row.Explanation, &row.TokensUsed,
			&row.ModelName, &row.ModelVariant, &row.Backend, &row.PromptVersion,
			&row.CustomInstructionsHash, &row.ScoredAt,
		); err != nil {
			return nil, fmt.Errorf("failed to scan VLM score: %w", err)
		}
		scores = append(scores, row)
	}
	return scores, nil
}

// DeleteVLMDataForFolder removes all VLM scores, rankings, and ranking groups for a folder.
//
// The three DELETEs share a transaction so a partial failure cannot leave
// the folder with ranking rows whose parent scores were already removed.
func (s *SQLiteStore) DeleteVLMDataForFolder(folderPath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin VLM data delete tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after Commit

	if err := deleteVLMDataForFolderTx(tx, folderPath); err != nil {
		return err
	}
	return tx.Commit()
}

// deleteVLMDataForFolderTx issues the three VLM DELETEs on an existing
// transaction. Callers are responsible for holding s.mu (if applicable) and
// for Commit/Rollback. Shared by DeleteVLMDataForFolder and
// DeleteAIDataForFolder to keep the delete list in one place.
func deleteVLMDataForFolderTx(tx *sql.Tx, folderPath string) error {
	if _, err := tx.Exec(`DELETE FROM vlm_scores WHERE folder_path = ?`, folderPath); err != nil {
		return fmt.Errorf("failed to delete VLM scores: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM vlm_rankings WHERE folder_path = ?`, folderPath); err != nil {
		return fmt.Errorf("failed to delete VLM rankings: %w", err)
	}
	if _, err := tx.Exec(`DELETE FROM vlm_ranking_groups WHERE folder_path = ?`, folderPath); err != nil {
		return fmt.Errorf("failed to delete VLM ranking groups: %w", err)
	}
	return nil
}

// ClearAllVLMData deletes all VLM scores, rankings, and token usage across all folders.
func (s *SQLiteStore) ClearAllVLMData() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	if _, err := s.db.Exec(`DELETE FROM vlm_scores`); err != nil {
		return fmt.Errorf("failed to clear vlm_scores: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM vlm_rankings`); err != nil {
		return fmt.Errorf("failed to clear vlm_rankings: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM vlm_ranking_groups`); err != nil {
		return fmt.Errorf("failed to clear vlm_ranking_groups: %w", err)
	}
	if _, err := s.db.Exec(`DELETE FROM vlm_token_usage`); err != nil {
		return fmt.Errorf("failed to clear vlm_token_usage: %w", err)
	}
	return nil
}

// GetStaleVLMFolders returns distinct folder paths that hold VLM scores which
// are stale against the current prompt version or the current custom-instructions
// hash. A folder is stale when ANY of its cached scores used an older
// prompt_version OR a different custom_instructions_hash. Callers display this
// to prompt a re-run.
func (s *SQLiteStore) GetStaleVLMFolders(currentPromptVersion int, currentHash string) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT DISTINCT folder_path FROM vlm_scores
		  WHERE prompt_version < ? OR custom_instructions_hash != ?`,
		currentPromptVersion, currentHash,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get stale VLM folders: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var folders []string
	for rows.Next() {
		var folder string
		if err := rows.Scan(&folder); err != nil {
			return nil, fmt.Errorf("failed to scan stale VLM folder: %w", err)
		}
		folders = append(folders, folder)
	}
	return folders, nil
}

// SaveVLMRanking saves a ranking group and its individual photo rankings in a transaction.
func (s *SQLiteStore) SaveVLMRanking(group VLMRankingGroupRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	tx, err := s.db.Begin()
	if err != nil {
		return fmt.Errorf("begin tx: %w", err)
	}
	defer func() { _ = tx.Rollback() }() //nolint:errcheck // no-op after commit

	_, err = tx.Exec(
		`INSERT OR REPLACE INTO vlm_ranking_groups
		 (folder_path, group_label, photo_count, explanation, model_name, prompt_version, custom_instructions_hash)
		 VALUES (?, ?, ?, ?, ?, ?, ?)`,
		group.FolderPath, group.GroupLabel, group.PhotoCount,
		group.Explanation, group.ModelName, group.PromptVersion, group.CustomInstructionsHash,
	)
	if err != nil {
		return fmt.Errorf("failed to save VLM ranking group: %w", err)
	}

	for _, r := range group.Rankings {
		_, err = tx.Exec(
			`INSERT OR REPLACE INTO vlm_rankings
			 (folder_path, group_label, photo_path, rank, relative_score, notes, tokens_used, model_name, prompt_version, custom_instructions_hash)
			 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
			group.FolderPath, group.GroupLabel, r.PhotoPath, r.Rank,
			r.RelativeScore, r.Notes, r.TokensUsed, group.ModelName, group.PromptVersion,
			group.CustomInstructionsHash,
		)
		if err != nil {
			return fmt.Errorf("failed to save VLM ranking row: %w", err)
		}
	}

	return tx.Commit()
}

// GetVLMRankingsForFolder loads all ranking groups for a folder including their individual rankings.
func (s *SQLiteStore) GetVLMRankingsForFolder(folderPath string) ([]VLMRankingGroupRow, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Single LEFT JOIN instead of the previous 1+N query pattern. The old
	// shape issued one "list groups" query followed by a "list rankings"
	// query per group, all while holding s.mu.RLock — with one write
	// transaction blocked for the whole fan-out. This version fetches the
	// flattened group×ranking tuple set once and aggregates in-memory.
	//
	// ORDER BY group_label keeps rows for a given group contiguous so the
	// map-less aggregator below can preserve insertion order without a
	// second pass. Within a group, ORDER BY rank ASC matches the previous
	// per-group sort. SQLite orders NULLs first by default, which places
	// the synthetic NULL row for an empty group (zero rankings) at the
	// top of its group block — we skip it via photoPath.Valid.
	rows, err := s.db.Query(
		`SELECT g.folder_path, g.group_label, g.photo_count, g.explanation, g.model_name, g.prompt_version,
		        g.custom_instructions_hash,
		        r.photo_path, r.rank, r.relative_score, r.notes, r.tokens_used
		   FROM vlm_ranking_groups g
		   LEFT JOIN vlm_rankings r
		     ON g.folder_path = r.folder_path AND g.group_label = r.group_label
		  WHERE g.folder_path = ?
		  ORDER BY g.group_label, r.rank ASC`,
		folderPath,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to query VLM rankings: %w", err)
	}
	defer func() { _ = rows.Close() }()

	// groupIndex maps group_label -> index in groups so we can append new
	// ranking rows without a second lookup pass. Contiguous-by-group
	// ordering means the map never grows beyond the distinct group count.
	groupIndex := make(map[string]int)
	var groups []VLMRankingGroupRow

	for rows.Next() {
		var g VLMRankingGroupRow
		var (
			photoPath  sql.NullString
			rank       sql.NullInt64
			relScore   sql.NullFloat64
			notes      sql.NullString
			tokensUsed sql.NullInt64
		)
		if err := rows.Scan(
			&g.FolderPath, &g.GroupLabel, &g.PhotoCount, &g.Explanation, &g.ModelName, &g.PromptVersion,
			&g.CustomInstructionsHash,
			&photoPath, &rank, &relScore, &notes, &tokensUsed,
		); err != nil {
			return nil, fmt.Errorf("failed to scan VLM ranking row: %w", err)
		}

		idx, ok := groupIndex[g.GroupLabel]
		if !ok {
			idx = len(groups)
			groupIndex[g.GroupLabel] = idx
			groups = append(groups, g)
		}
		// A group with zero rankings still produces one row (NULL ranking
		// columns) from the LEFT JOIN. Skip those so empty groups end up
		// with a nil Rankings slice instead of a phantom zero-value row.
		if photoPath.Valid {
			groups[idx].Rankings = append(groups[idx].Rankings, VLMRankingRow{
				PhotoPath:     photoPath.String,
				Rank:          int(rank.Int64),
				RelativeScore: relScore.Float64,
				Notes:         notes.String,
				TokensUsed:    int(tokensUsed.Int64),
			})
		}
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("VLM rankings iteration error: %w", err)
	}

	return groups, nil
}

// RecordTokenUsage inserts a token usage record.
func (s *SQLiteStore) RecordTokenUsage(usage TokenUsageRow) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	_, err := s.db.Exec(
		`INSERT INTO vlm_token_usage (provider, folder_path, stage, tokens_input, tokens_output, photo_count)
		 VALUES (?, ?, ?, ?, ?, ?)`,
		usage.Provider, usage.FolderPath, usage.Stage,
		usage.TokensInput, usage.TokensOutput, usage.PhotoCount,
	)
	if err != nil {
		return fmt.Errorf("failed to record token usage: %w", err)
	}
	return nil
}

// GetTokenUsageSummary returns aggregated token usage grouped by provider.
func (s *SQLiteStore) GetTokenUsageSummary() ([]TokenUsageSummary, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	rows, err := s.db.Query(
		`SELECT provider, SUM(tokens_input), SUM(tokens_output), SUM(photo_count), COUNT(*)
		 FROM vlm_token_usage GROUP BY provider`,
	)
	if err != nil {
		return nil, fmt.Errorf("failed to get token usage summary: %w", err)
	}
	defer func() { _ = rows.Close() }()

	var summaries []TokenUsageSummary
	for rows.Next() {
		var s TokenUsageSummary
		if err := rows.Scan(&s.Provider, &s.TotalInput, &s.TotalOutput, &s.TotalPhotos, &s.SessionCount); err != nil {
			return nil, fmt.Errorf("failed to scan token usage summary: %w", err)
		}
		summaries = append(summaries, s)
	}
	return summaries, nil
}

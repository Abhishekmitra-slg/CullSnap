package storage

// GetConfig retrieves a single config value by key.
// Returns empty string (not an error) if the key does not exist.
func (s *SQLiteStore) GetConfig(key string) (string, error) {
	var value string
	row := s.db.QueryRow("SELECT value FROM app_config WHERE key = ?", key)
	err := row.Scan(&value)
	if err != nil {
		return "", nil
	}
	return value, nil
}

// SetConfig upserts a config key-value pair.
func (s *SQLiteStore) SetConfig(key, value string) error {
	_, err := s.db.Exec(
		"INSERT INTO app_config (key, value) VALUES (?, ?) ON CONFLICT(key) DO UPDATE SET value = excluded.value",
		key, value,
	)
	return err
}

// DeleteAllConfig removes all config entries, triggering a fresh probe on next startup.
func (s *SQLiteStore) DeleteAllConfig() error {
	_, err := s.db.Exec("DELETE FROM app_config")
	return err
}

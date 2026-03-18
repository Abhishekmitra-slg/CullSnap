package storage

import (
	"os"
	"testing"
)

func TestAppConfig_SetAndGet(t *testing.T) {
	dbPath := "test_config.db"
	defer os.Remove(dbPath)
	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()
	if err := store.SetConfig("maxConnections", "25"); err != nil {
		t.Fatalf("SetConfig failed: %v", err)
	}
	val, err := store.GetConfig("maxConnections")
	if err != nil {
		t.Fatalf("GetConfig failed: %v", err)
	}
	if val != "25" {
		t.Errorf("Expected '25', got %q", val)
	}
}

func TestAppConfig_GetMissing(t *testing.T) {
	dbPath := "test_config_missing.db"
	defer os.Remove(dbPath)
	store, _ := NewSQLiteStore(dbPath)
	defer store.Close()
	val, err := store.GetConfig("nonexistent")
	if err != nil {
		t.Fatalf("GetConfig for missing key should not error: %v", err)
	}
	if val != "" {
		t.Errorf("Expected empty string for missing key, got %q", val)
	}
}

func TestAppConfig_Overwrite(t *testing.T) {
	dbPath := "test_config_overwrite.db"
	defer os.Remove(dbPath)
	store, _ := NewSQLiteStore(dbPath)
	defer store.Close()
	store.SetConfig("key", "original")
	store.SetConfig("key", "updated")
	val, _ := store.GetConfig("key")
	if val != "updated" {
		t.Errorf("Expected 'updated', got %q", val)
	}
}

func TestAppConfig_DeleteAll(t *testing.T) {
	dbPath := "test_config_delete.db"
	defer os.Remove(dbPath)
	store, _ := NewSQLiteStore(dbPath)
	defer store.Close()
	store.SetConfig("a", "1")
	store.SetConfig("b", "2")
	if err := store.DeleteAllConfig(); err != nil {
		t.Fatalf("DeleteAllConfig failed: %v", err)
	}
	val, _ := store.GetConfig("a")
	if val != "" {
		t.Errorf("Expected empty after delete, got %q", val)
	}
}

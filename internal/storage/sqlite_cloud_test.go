package storage

import (
	"errors"
	"os"
	"testing"
	"time"
)

func TestCloudMirrors(t *testing.T) {
	dbPath := "test_cloud_mirrors.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	providerID := "google"
	albumID := "album-001"
	albumTitle := "Vacation 2025"
	localPath := "/home/user/photos/vacation"

	// Save a cloud mirror
	if err := store.SaveCloudMirror(providerID, albumID, albumTitle, localPath); err != nil {
		t.Fatalf("SaveCloudMirror failed: %v", err)
	}

	// Get it back
	m, err := store.GetCloudMirror(providerID, albumID)
	if err != nil {
		t.Fatalf("GetCloudMirror failed: %v", err)
	}
	if m.ProviderID != providerID {
		t.Errorf("Expected ProviderID %q, got %q", providerID, m.ProviderID)
	}
	if m.AlbumID != albumID {
		t.Errorf("Expected AlbumID %q, got %q", albumID, m.AlbumID)
	}
	if m.AlbumTitle != albumTitle {
		t.Errorf("Expected AlbumTitle %q, got %q", albumTitle, m.AlbumTitle)
	}
	if m.LocalPath != localPath {
		t.Errorf("Expected LocalPath %q, got %q", localPath, m.LocalPath)
	}

	// Save a second mirror
	if err := store.SaveCloudMirror("apple", "album-002", "Family", "/home/user/photos/family"); err != nil {
		t.Fatalf("SaveCloudMirror (second) failed: %v", err)
	}

	// List mirrors
	mirrors, err := store.ListCloudMirrors()
	if err != nil {
		t.Fatalf("ListCloudMirrors failed: %v", err)
	}
	if len(mirrors) != 2 {
		t.Errorf("Expected 2 cloud mirrors, got %d", len(mirrors))
	}

	// Upsert (INSERT OR REPLACE) — update title for existing record
	updatedTitle := "Vacation 2025 Updated"
	if err := store.SaveCloudMirror(providerID, albumID, updatedTitle, localPath); err != nil {
		t.Fatalf("SaveCloudMirror upsert failed: %v", err)
	}
	m, err = store.GetCloudMirror(providerID, albumID)
	if err != nil {
		t.Fatalf("GetCloudMirror after upsert failed: %v", err)
	}
	if m.AlbumTitle != updatedTitle {
		t.Errorf("Expected updated title %q, got %q", updatedTitle, m.AlbumTitle)
	}

	// List still has 2 entries
	mirrors, err = store.ListCloudMirrors()
	if err != nil {
		t.Fatalf("ListCloudMirrors after upsert failed: %v", err)
	}
	if len(mirrors) != 2 {
		t.Errorf("Expected 2 cloud mirrors after upsert, got %d", len(mirrors))
	}

	// Delete the first mirror
	if err := store.DeleteCloudMirror(providerID, albumID); err != nil {
		t.Fatalf("DeleteCloudMirror failed: %v", err)
	}

	// Get after delete should return error
	_, err = store.GetCloudMirror(providerID, albumID)
	if err == nil {
		t.Error("Expected error after GetCloudMirror on deleted record, got nil")
	}

	// List should now have 1 entry
	mirrors, err = store.ListCloudMirrors()
	if err != nil {
		t.Fatalf("ListCloudMirrors after delete failed: %v", err)
	}
	if len(mirrors) != 1 {
		t.Errorf("Expected 1 cloud mirror after delete, got %d", len(mirrors))
	}
}

func TestCloudMediaMeta(t *testing.T) {
	dbPath := "test_cloud_media_meta.db"
	defer os.Remove(dbPath)

	store, err := NewSQLiteStore(dbPath)
	if err != nil {
		t.Fatalf("Failed to create store: %v", err)
	}
	defer store.Close()

	localPath := "/home/user/photos/img1.jpg"
	remoteID := "remote-xyz-123"
	providerID := "google"
	remoteUpdatedAt := time.Date(2025, 6, 15, 10, 0, 0, 0, time.UTC)

	// Save cloud media meta
	if err := store.SaveCloudMediaMeta(localPath, remoteID, providerID, remoteUpdatedAt); err != nil {
		t.Fatalf("SaveCloudMediaMeta failed: %v", err)
	}

	// Get it back
	meta, err := store.GetCloudMediaMeta(localPath)
	if err != nil {
		t.Fatalf("GetCloudMediaMeta failed: %v", err)
	}
	if meta.LocalPath != localPath {
		t.Errorf("Expected LocalPath %q, got %q", localPath, meta.LocalPath)
	}
	if meta.RemoteID != remoteID {
		t.Errorf("Expected RemoteID %q, got %q", remoteID, meta.RemoteID)
	}
	if meta.ProviderID != providerID {
		t.Errorf("Expected ProviderID %q, got %q", providerID, meta.ProviderID)
	}
	if !meta.RemoteUpdatedAt.Equal(remoteUpdatedAt) {
		t.Errorf("Expected RemoteUpdatedAt %v, got %v", remoteUpdatedAt, meta.RemoteUpdatedAt)
	}

	// Upsert with new remoteID
	newRemoteID := "remote-abc-999"
	if err := store.SaveCloudMediaMeta(localPath, newRemoteID, providerID, remoteUpdatedAt); err != nil {
		t.Fatalf("SaveCloudMediaMeta upsert failed: %v", err)
	}
	meta, err = store.GetCloudMediaMeta(localPath)
	if err != nil {
		t.Fatalf("GetCloudMediaMeta after upsert failed: %v", err)
	}
	if meta.RemoteID != newRemoteID {
		t.Errorf("Expected updated RemoteID %q, got %q", newRemoteID, meta.RemoteID)
	}

	// Get a non-existent path should return an error
	_, err = store.GetCloudMediaMeta("/nonexistent/path.jpg")
	if err == nil {
		t.Error("Expected error for non-existent cloud media meta, got nil")
	}
	if !errors.Is(err, errors.New("")) && err == nil {
		t.Error("Expected non-nil error for missing record")
	}
}

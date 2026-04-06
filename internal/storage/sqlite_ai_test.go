package storage

import (
	"testing"
)

func TestSaveAndGetAIScore(t *testing.T) {
	store := newTestStore(t)

	err := store.SaveAIScore(&AIScore{PhotoPath: "/photos/test.jpg", OverallScore: 0.87, FaceCount: 2, Provider: "Local ONNX"})
	if err != nil {
		t.Fatalf("SaveAIScore failed: %v", err)
	}

	score, err := store.GetAIScore("/photos/test.jpg")
	if err != nil {
		t.Fatalf("GetAIScore failed: %v", err)
	}
	if score == nil {
		t.Fatal("GetAIScore returned nil")
	}
	if score.OverallScore != 0.87 {
		t.Errorf("OverallScore = %f, want 0.87", score.OverallScore)
	}
	if score.FaceCount != 2 {
		t.Errorf("FaceCount = %d, want 2", score.FaceCount)
	}
	if score.Provider != "Local ONNX" {
		t.Errorf("Provider = %s, want Local ONNX", score.Provider)
	}
}

func TestGetAIScore_NotFound(t *testing.T) {
	store := newTestStore(t)

	score, err := store.GetAIScore("/nonexistent.jpg")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if score != nil {
		t.Error("expected nil for nonexistent path")
	}
}

func TestGetAIScoresForFolder(t *testing.T) {
	store := newTestStore(t)

	_ = store.SaveAIScore(&AIScore{PhotoPath: "/photos/a.jpg", OverallScore: 0.9, FaceCount: 1, Provider: "Local ONNX"})
	_ = store.SaveAIScore(&AIScore{PhotoPath: "/photos/b.jpg", OverallScore: 0.7, FaceCount: 0, Provider: "Local ONNX"})
	_ = store.SaveAIScore(&AIScore{PhotoPath: "/other/c.jpg", OverallScore: 0.5, FaceCount: 1, Provider: "Cloud"})

	scores, err := store.GetAIScoresForFolder("/photos")
	if err != nil {
		t.Fatalf("GetAIScoresForFolder failed: %v", err)
	}
	if len(scores) != 2 {
		t.Errorf("got %d scores, want 2", len(scores))
	}
}

func TestSaveAndGetFaceDetection(t *testing.T) {
	store := newTestStore(t)

	det := &FaceDetection{
		PhotoPath:    "/photos/test.jpg",
		BboxX:        10,
		BboxY:        20,
		BboxW:        100,
		BboxH:        120,
		EyeSharpness: 0.92,
		EyesOpen:     true,
		Expression:   0.78,
		Confidence:   0.95,
		Embedding:    []byte{1, 2, 3, 4},
	}

	id, err := store.SaveFaceDetection(det)
	if err != nil {
		t.Fatalf("SaveFaceDetection failed: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}

	dets, err := store.GetFaceDetections("/photos/test.jpg")
	if err != nil {
		t.Fatalf("GetFaceDetections failed: %v", err)
	}
	if len(dets) != 1 {
		t.Fatalf("got %d detections, want 1", len(dets))
	}
	if dets[0].EyeSharpness != 0.92 {
		t.Errorf("EyeSharpness = %f, want 0.92", dets[0].EyeSharpness)
	}
}

func TestSaveAndGetFaceCluster(t *testing.T) {
	store := newTestStore(t)

	cluster := &FaceCluster{
		FolderPath:         "/photos",
		Label:              "Person 1",
		RepresentativePath: "/photos/test.jpg",
		PhotoCount:         5,
	}

	id, err := store.SaveFaceCluster(cluster)
	if err != nil {
		t.Fatalf("SaveFaceCluster failed: %v", err)
	}
	if id == 0 {
		t.Error("expected non-zero ID")
	}

	clusters, err := store.GetFaceClusters("/photos")
	if err != nil {
		t.Fatalf("GetFaceClusters failed: %v", err)
	}
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(clusters))
	}
	if clusters[0].Label != "Person 1" {
		t.Errorf("Label = %s, want Person 1", clusters[0].Label)
	}
}

func TestRenameFaceCluster(t *testing.T) {
	store := newTestStore(t)

	id, _ := store.SaveFaceCluster(&FaceCluster{
		FolderPath: "/photos", Label: "Person 1", PhotoCount: 3,
	})

	err := store.RenameFaceCluster(id, "Sarah")
	if err != nil {
		t.Fatalf("RenameFaceCluster failed: %v", err)
	}

	clusters, _ := store.GetFaceClusters("/photos")
	if clusters[0].Label != "Sarah" {
		t.Errorf("Label = %s, want Sarah", clusters[0].Label)
	}
}

func TestMergeFaceClusters(t *testing.T) {
	store := newTestStore(t)

	id1, _ := store.SaveFaceCluster(&FaceCluster{
		FolderPath: "/photos", Label: "Person 1", PhotoCount: 3,
	})
	id2, _ := store.SaveFaceCluster(&FaceCluster{
		FolderPath: "/photos", Label: "Person 2", PhotoCount: 2,
	})

	// Add detections to both clusters
	_, _ = store.SaveFaceDetection(&FaceDetection{
		PhotoPath: "/photos/a.jpg", ClusterID: &id1,
		BboxX: 10, BboxY: 10, BboxW: 50, BboxH: 50, Confidence: 0.9,
	})
	_, _ = store.SaveFaceDetection(&FaceDetection{
		PhotoPath: "/photos/b.jpg", ClusterID: &id2,
		BboxX: 20, BboxY: 20, BboxW: 60, BboxH: 60, Confidence: 0.9,
	})

	err := store.MergeFaceClusters(id2, id1) // merge id2 INTO id1
	if err != nil {
		t.Fatalf("MergeFaceClusters failed: %v", err)
	}

	clusters, _ := store.GetFaceClusters("/photos")
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters after merge, want 1", len(clusters))
	}
	if clusters[0].PhotoCount != 5 {
		t.Errorf("merged PhotoCount = %d, want 5", clusters[0].PhotoCount)
	}

	// Check detections were reassigned
	dets, _ := store.GetFaceDetections("/photos/b.jpg")
	if len(dets) != 1 || *dets[0].ClusterID != id1 {
		t.Error("detection not reassigned to target cluster")
	}
}

func TestHideFaceCluster(t *testing.T) {
	store := newTestStore(t)

	id, _ := store.SaveFaceCluster(&FaceCluster{
		FolderPath: "/photos", Label: "Person 1", PhotoCount: 1,
	})

	err := store.HideFaceCluster(id, true)
	if err != nil {
		t.Fatalf("HideFaceCluster failed: %v", err)
	}

	clusters, _ := store.GetFaceClusters("/photos")
	if len(clusters) != 0 {
		t.Error("hidden cluster should not appear in GetFaceClusters")
	}

	all, _ := store.GetAllFaceClusters("/photos")
	if len(all) != 1 {
		t.Error("hidden cluster should appear in GetAllFaceClusters")
	}
}

// newTestStore creates a temporary in-memory store for testing.
// Reuse the pattern from existing tests.
func newTestStore(t *testing.T) *SQLiteStore {
	t.Helper()
	store, err := NewSQLiteStore(":memory:")
	if err != nil {
		t.Fatalf("failed to create test store: %v", err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

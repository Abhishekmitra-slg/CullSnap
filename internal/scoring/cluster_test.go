package scoring

import (
	"cullsnap/internal/logger"
	"math"
	"os"
	"path/filepath"
	"testing"
)

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "cullsnap-scoring-test-*")
	if err != nil {
		panic("failed to create temp dir for logger: " + err.Error())
	}
	logPath := filepath.Join(tmpDir, "test.log")
	if err := logger.Init(logPath); err != nil {
		panic("failed to init logger: " + err.Error())
	}
	code := m.Run()
	os.RemoveAll(tmpDir)
	os.Exit(code)
}

func TestCosineSimilarity(t *testing.T) {
	// Identical vectors → similarity 1.0
	a := []float32{1, 0, 0, 1}
	b := []float32{1, 0, 0, 1}
	sim := CosineSimilarity(a, b)
	if math.Abs(float64(sim)-1.0) > 0.001 {
		t.Errorf("identical vectors: similarity = %f, want 1.0", sim)
	}

	// Orthogonal vectors → similarity 0.0
	c := []float32{1, 0, 0, 0}
	d := []float32{0, 1, 0, 0}
	sim = CosineSimilarity(c, d)
	if math.Abs(float64(sim)) > 0.001 {
		t.Errorf("orthogonal vectors: similarity = %f, want 0.0", sim)
	}

	// Opposite vectors → similarity -1.0
	e := []float32{1, 0}
	f := []float32{-1, 0}
	sim = CosineSimilarity(e, f)
	if math.Abs(float64(sim)+1.0) > 0.001 {
		t.Errorf("opposite vectors: similarity = %f, want -1.0", sim)
	}
}

func TestCosineSimilarity_ZeroVector(t *testing.T) {
	a := []float32{0, 0, 0}
	b := []float32{1, 2, 3}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("zero vector: similarity = %f, want 0", sim)
	}
}

func TestCosineSimilarity_DifferentLengths(t *testing.T) {
	a := []float32{1, 2}
	b := []float32{1, 2, 3}
	sim := CosineSimilarity(a, b)
	if sim != 0 {
		t.Errorf("different lengths: similarity = %f, want 0", sim)
	}
}

func TestClusterFaces(t *testing.T) {
	faces := []FaceEmbedding{
		{PhotoPath: "/a.jpg", DetectionID: 1, Embedding: []float32{1, 0, 0}},
		{PhotoPath: "/b.jpg", DetectionID: 2, Embedding: []float32{0.95, 0.05, 0}},    // similar to face 0
		{PhotoPath: "/c.jpg", DetectionID: 3, Embedding: []float32{0, 0, 1}},          // different person
		{PhotoPath: "/d.jpg", DetectionID: 4, Embedding: []float32{0.02, 0.01, 0.99}}, // similar to face 2
	}

	clusters := ClusterFaces(faces, 0.6)

	if len(clusters) != 2 {
		t.Fatalf("got %d clusters, want 2", len(clusters))
	}

	// Verify each cluster has the right face count
	counts := map[int]int{}
	for _, c := range clusters {
		counts[len(c)]++
	}
	if counts[2] != 2 {
		t.Errorf("expected 2 clusters of size 2, got counts: %v", counts)
	}
}

func TestClusterFaces_Empty(t *testing.T) {
	clusters := ClusterFaces(nil, 0.6)
	if len(clusters) != 0 {
		t.Errorf("expected 0 clusters for empty input, got %d", len(clusters))
	}
}

func TestClusterFaces_SingleFace(t *testing.T) {
	faces := []FaceEmbedding{
		{PhotoPath: "/a.jpg", DetectionID: 1, Embedding: []float32{1, 0, 0}},
	}
	clusters := ClusterFaces(faces, 0.6)
	if len(clusters) != 1 {
		t.Fatalf("got %d clusters, want 1", len(clusters))
	}
	if len(clusters[0]) != 1 {
		t.Errorf("cluster size = %d, want 1", len(clusters[0]))
	}
}

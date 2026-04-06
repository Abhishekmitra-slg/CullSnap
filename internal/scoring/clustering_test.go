package scoring

import (
	"math"
	"testing"
)

// ---- CosineSim tests ----

func TestCosineSim_Identical(t *testing.T) {
	t.Parallel()
	a := []float32{1, 0, 0}
	sim := CosineSim(a, a)
	if math.Abs(float64(sim)-1.0) > 1e-6 {
		t.Fatalf("expected ~1.0, got %f", sim)
	}
}

func TestCosineSim_Orthogonal(t *testing.T) {
	t.Parallel()
	a := []float32{1, 0, 0}
	b := []float32{0, 1, 0}
	sim := CosineSim(a, b)
	if math.Abs(float64(sim)) > 1e-6 {
		t.Fatalf("expected ~0, got %f", sim)
	}
}

func TestCosineSim_Opposite(t *testing.T) {
	t.Parallel()
	a := []float32{1, 0, 0}
	b := []float32{-1, 0, 0}
	sim := CosineSim(a, b)
	if math.Abs(float64(sim)+1.0) > 1e-6 {
		t.Fatalf("expected ~-1.0, got %f", sim)
	}
}

func TestCosineSim_EmptyVectors(t *testing.T) {
	t.Parallel()
	sim := CosineSim([]float32{}, []float32{})
	if sim != 0 {
		t.Fatalf("expected 0 for empty vectors, got %f", sim)
	}
}

func TestCosineSim_DifferentLengths(t *testing.T) {
	t.Parallel()
	a := []float32{1, 0}
	b := []float32{1, 0, 0}
	sim := CosineSim(a, b)
	if sim != 0 {
		t.Fatalf("expected 0 for different-length vectors, got %f", sim)
	}
}

// ---- ClusterFacesAgglomerative tests ----

func TestClusterFacesAgglomerative_TwoPeople(t *testing.T) {
	t.Parallel()
	// Two distinct pairs: pair A ~ [1,0,0], pair B ~ [0,1,0].
	// Within each pair the embeddings are nearly identical; between pairs they are orthogonal.
	a1 := []float32{1.0, 0.0, 0.0}
	a2 := []float32{0.99, 0.01, 0.0}
	b1 := []float32{0.0, 1.0, 0.0}
	b2 := []float32{0.01, 0.99, 0.0}

	faces := []FaceEmbedding{
		{PhotoPath: "p1.jpg", DetectionID: 1, Embedding: a1},
		{PhotoPath: "p2.jpg", DetectionID: 2, Embedding: a2},
		{PhotoPath: "p3.jpg", DetectionID: 3, Embedding: b1},
		{PhotoPath: "p4.jpg", DetectionID: 4, Embedding: b2},
	}

	clusters := ClusterFacesAgglomerative(faces, DefaultClusterThreshold)
	if len(clusters) != 2 {
		t.Fatalf("expected 2 clusters, got %d", len(clusters))
	}
	for _, c := range clusters {
		if len(c.Faces) != 2 {
			t.Errorf("expected 2 faces per cluster, got %d", len(c.Faces))
		}
	}
}

func TestClusterFacesAgglomerative_SingleFace(t *testing.T) {
	t.Parallel()
	faces := []FaceEmbedding{
		{PhotoPath: "p1.jpg", DetectionID: 1, Embedding: []float32{1, 0, 0}},
	}
	clusters := ClusterFacesAgglomerative(faces, DefaultClusterThreshold)
	if len(clusters) != 1 {
		t.Fatalf("expected 1 cluster, got %d", len(clusters))
	}
	if len(clusters[0].Faces) != 1 {
		t.Errorf("expected 1 face, got %d", len(clusters[0].Faces))
	}
}

func TestClusterFacesAgglomerative_Empty(t *testing.T) {
	t.Parallel()
	clusters := ClusterFacesAgglomerative(nil, DefaultClusterThreshold)
	if clusters != nil {
		t.Fatalf("expected nil for empty input, got %v", clusters)
	}
}

func TestClusterFacesAgglomerative_AllDifferent(t *testing.T) {
	t.Parallel()
	// Three mutually orthogonal embeddings → three separate clusters.
	faces := []FaceEmbedding{
		{PhotoPath: "p1.jpg", DetectionID: 1, Embedding: []float32{1, 0, 0}},
		{PhotoPath: "p2.jpg", DetectionID: 2, Embedding: []float32{0, 1, 0}},
		{PhotoPath: "p3.jpg", DetectionID: 3, Embedding: []float32{0, 0, 1}},
	}
	clusters := ClusterFacesAgglomerative(faces, DefaultClusterThreshold)
	if len(clusters) != 3 {
		t.Fatalf("expected 3 clusters, got %d", len(clusters))
	}
}

// ---- averageEmbeddings tests ----

func TestAverageEmbeddings(t *testing.T) {
	t.Parallel()
	faces := []FaceEmbedding{
		{Embedding: []float32{1, 0}},
		{Embedding: []float32{0, 1}},
	}
	result := averageEmbeddings(faces)
	if len(result) != 2 {
		t.Fatalf("expected length 2, got %d", len(result))
	}
	if math.Abs(float64(result[0])-0.5) > 1e-6 || math.Abs(float64(result[1])-0.5) > 1e-6 {
		t.Errorf("expected [0.5, 0.5], got [%f, %f]", result[0], result[1])
	}
}

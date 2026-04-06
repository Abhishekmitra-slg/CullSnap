package scoring

import (
	"cullsnap/internal/logger"
	"math"
)

// DefaultClusterThreshold is the default cosine similarity threshold for agglomerative clustering.
const DefaultClusterThreshold float32 = 0.45

// maxClusterFaces is the maximum number of faces to cluster. Beyond this the
// O(n^2) distance matrix becomes prohibitively expensive.
const maxClusterFaces = 2000

// FaceClusterResult holds the faces and centroid embedding for a single cluster.
type FaceClusterResult struct {
	Faces    []FaceEmbedding
	Centroid []float32
}

// ClusterFacesAgglomerative groups face embeddings using agglomerative clustering.
// Each face starts in its own cluster; pairs whose centroid cosine similarity exceeds
// threshold are merged iteratively until no qualifying pair remains.
// Returns only active (non-merged) clusters.
func ClusterFacesAgglomerative(faces []FaceEmbedding, threshold float32) []FaceClusterResult {
	if len(faces) == 0 {
		logger.Log.Debug("scoring: agglomerative clustering: no faces to cluster")
		return nil
	}

	if len(faces) > maxClusterFaces {
		logger.Log.Warn("scoring: agglomerative clustering: capping face count",
			"actual", len(faces),
			"max", maxClusterFaces,
		)
		faces = faces[:maxClusterFaces]
	}

	// Initialise one cluster per face.
	clusters := make([]FaceClusterResult, len(faces))
	active := make([]bool, len(faces))
	for i, f := range faces {
		clusters[i] = FaceClusterResult{
			Faces:    []FaceEmbedding{copyEmbeddingFace(f)},
			Centroid: copyEmbedding(f.Embedding),
		}
		active[i] = true
	}

	for {
		bestSim := float32(-2)
		bestI, bestJ := -1, -1

		// Find the closest active pair.
		for i := 0; i < len(clusters); i++ {
			if !active[i] {
				continue
			}
			for j := i + 1; j < len(clusters); j++ {
				if !active[j] {
					continue
				}
				sim := CosineSim(clusters[i].Centroid, clusters[j].Centroid)
				logger.Log.Debug("scoring: agglomerative: comparing clusters",
					"clusterI", i,
					"clusterJ", j,
					"similarity", sim,
				)
				if sim > bestSim {
					bestSim = sim
					bestI, bestJ = i, j
				}
			}
		}

		// Stop if no pair exceeds the threshold.
		if bestI < 0 || bestSim < threshold {
			logger.Log.Debug("scoring: agglomerative: no qualifying pair found, stopping",
				"bestSim", bestSim,
				"threshold", threshold,
			)
			break
		}

		// Merge cluster bestJ into cluster bestI.
		logger.Log.Debug("scoring: agglomerative: merging clusters",
			"clusterI", bestI,
			"clusterJ", bestJ,
			"similarity", bestSim,
		)
		clusters[bestI].Faces = append(clusters[bestI].Faces, clusters[bestJ].Faces...)
		clusters[bestI].Centroid = averageEmbeddings(clusters[bestI].Faces)
		active[bestJ] = false
	}

	// Collect active clusters.
	var result []FaceClusterResult
	for i := range clusters {
		if active[i] {
			result = append(result, clusters[i])
		}
	}

	logger.Log.Info("scoring: agglomerative clustering complete",
		"inputFaces", len(faces),
		"clusters", len(result),
		"threshold", threshold,
	)

	return result
}

// CosineSim computes the cosine similarity between two float32 vectors.
// Returns 0 for empty or length-mismatched inputs.
func CosineSim(a, b []float32) float32 {
	if len(a) == 0 || len(a) != len(b) {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		fa, fb := float64(a[i]), float64(b[i])
		dot += fa * fb
		normA += fa * fa
		normB += fb * fb
	}

	denom := math.Sqrt(normA) * math.Sqrt(normB)
	if denom == 0 {
		return 0
	}

	return float32(dot / denom)
}

// copyEmbedding returns a shallow copy of an embedding slice.
func copyEmbedding(e []float32) []float32 {
	if e == nil {
		return nil
	}
	dst := make([]float32, len(e))
	copy(dst, e)
	return dst
}

// copyEmbeddingFace returns a copy of a FaceEmbedding with its embedding slice duplicated.
func copyEmbeddingFace(f FaceEmbedding) FaceEmbedding {
	return FaceEmbedding{
		PhotoPath:   f.PhotoPath,
		DetectionID: f.DetectionID,
		Embedding:   copyEmbedding(f.Embedding),
	}
}

// averageEmbeddings computes the element-wise mean of all face embeddings.
// Returns nil if faces is empty or no face has an embedding.
func averageEmbeddings(faces []FaceEmbedding) []float32 {
	if len(faces) == 0 {
		return nil
	}

	dim := 0
	for _, f := range faces {
		if len(f.Embedding) > 0 {
			dim = len(f.Embedding)
			break
		}
	}
	if dim == 0 {
		return nil
	}

	sum := make([]float64, dim)
	count := 0
	for _, f := range faces {
		if len(f.Embedding) != dim {
			continue
		}
		for i, v := range f.Embedding {
			sum[i] += float64(v)
		}
		count++
	}
	if count == 0 {
		return nil
	}

	result := make([]float32, dim)
	for i, s := range sum {
		result[i] = float32(s / float64(count))
	}
	return result
}

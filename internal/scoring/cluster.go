package scoring

import (
	"cullsnap/internal/logger"
	"math"
)

// FaceEmbedding holds the embedding vector for a single detected face.
type FaceEmbedding struct {
	PhotoPath   string
	DetectionID int64
	Embedding   []float32
}

// CosineSimilarity computes the cosine similarity between two embedding vectors.
// Returns 0 if vectors have different lengths or either is zero-length.
func CosineSimilarity(a, b []float32) float32 {
	if len(a) != len(b) || len(a) == 0 {
		return 0
	}

	var dot, normA, normB float64
	for i := range a {
		dot += float64(a[i]) * float64(b[i])
		normA += float64(a[i]) * float64(a[i])
		normB += float64(b[i]) * float64(b[i])
	}

	denominator := math.Sqrt(normA) * math.Sqrt(normB)
	if denominator == 0 {
		return 0
	}

	return float32(dot / denominator)
}

// ClusterFaces groups face embeddings by cosine similarity using greedy clustering.
// Faces with similarity >= threshold are placed in the same cluster.
// Returns a slice of clusters, where each cluster is a slice of FaceEmbeddings.
func ClusterFaces(faces []FaceEmbedding, threshold float32) [][]FaceEmbedding {
	if len(faces) == 0 {
		return nil
	}

	assigned := make([]bool, len(faces))
	var clusters [][]FaceEmbedding

	for i := range faces {
		if assigned[i] {
			continue
		}

		cluster := []FaceEmbedding{faces[i]}
		assigned[i] = true

		// Compare centroid (first face in cluster) against all unassigned faces
		for j := i + 1; j < len(faces); j++ {
			if assigned[j] {
				continue
			}

			sim := CosineSimilarity(faces[i].Embedding, faces[j].Embedding)
			logger.Log.Debug("scoring: face similarity",
				"faceA", faces[i].DetectionID,
				"faceB", faces[j].DetectionID,
				"similarity", sim,
				"threshold", threshold,
			)

			if sim >= threshold {
				cluster = append(cluster, faces[j])
				assigned[j] = true
			}
		}

		clusters = append(clusters, cluster)
		logger.Log.Debug("scoring: created cluster",
			"clusterIndex", len(clusters)-1,
			"faceCount", len(cluster),
		)
	}

	logger.Log.Info("scoring: clustering complete",
		"totalFaces", len(faces),
		"clusterCount", len(clusters),
		"threshold", threshold,
	)

	return clusters
}

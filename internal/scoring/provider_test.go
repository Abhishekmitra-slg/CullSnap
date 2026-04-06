package scoring

import (
	"image"
	"testing"
)

func TestPluginCategory_String(t *testing.T) {
	cases := []struct {
		cat  PluginCategory
		want string
	}{
		{CategoryDetection, "detection"},
		{CategoryRecognition, "recognition"},
		{CategoryQuality, "quality"},
		{PluginCategory(99), "unknown"},
	}
	for _, tc := range cases {
		got := tc.cat.String()
		if got != tc.want {
			t.Errorf("PluginCategory(%d).String() = %q, want %q", tc.cat, got, tc.want)
		}
	}
}

func TestCompositeScore_OverallScore_WithoutFaces(t *testing.T) {
	cs := CompositeScore{
		AestheticScore: 0.8,
		SharpnessScore: 0.6,
		FaceCount:      0,
	}
	w := DefaultWeights()
	score := cs.OverallScore(w)
	if score < 0.0 || score > 1.0 {
		t.Errorf("OverallScore() = %f, want value in [0, 1]", score)
	}
}

func TestCompositeScore_OverallScore_WithFaces(t *testing.T) {
	cs := CompositeScore{
		AestheticScore: 0.7,
		SharpnessScore: 0.8,
		FaceCount:      2,
		BestFaceSharp:  0.9,
		EyeOpenness:    0.85,
		BestFaceIdx:    1,
		Faces: []FaceRegion{
			{BoundingBox: image.Rect(0, 0, 50, 50), Confidence: 0.88},
			{BoundingBox: image.Rect(60, 60, 110, 110), Confidence: 0.95},
		},
	}
	w := DefaultWeights()
	score := cs.OverallScore(w)
	if score < 0.0 || score > 1.0 {
		t.Errorf("OverallScore() = %f, want value in [0, 1]", score)
	}
}

func TestFaceRegion_BoundingBox(t *testing.T) {
	r := image.Rect(10, 20, 110, 220)
	fr := FaceRegion{
		BoundingBox: r,
		Confidence:  0.93,
	}
	if fr.BoundingBox != r {
		t.Errorf("FaceRegion.BoundingBox = %v, want %v", fr.BoundingBox, r)
	}
	if fr.BoundingBox.Dx() != 100 {
		t.Errorf("BoundingBox width = %d, want 100", fr.BoundingBox.Dx())
	}
	if fr.BoundingBox.Dy() != 200 {
		t.Errorf("BoundingBox height = %d, want 200", fr.BoundingBox.Dy())
	}
}

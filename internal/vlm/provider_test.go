package vlm

import (
	"testing"
)

func TestHardwareTierString(t *testing.T) {
	tests := []struct {
		tier HardwareTier
		want string
	}{
		{TierLegacy, "legacy"},
		{TierBasic, "basic"},
		{TierCapable, "capable"},
		{TierPower, "power"},
	}
	for _, tt := range tests {
		if got := tt.tier.String(); got != tt.want {
			t.Errorf("HardwareTier(%d).String() = %q, want %q", tt.tier, got, tt.want)
		}
	}
}

func TestVLMScoreValidate(t *testing.T) {
	valid := VLMScore{
		Aesthetic:     0.8,
		Composition:   0.7,
		Expression:    0.6,
		TechnicalQual: 0.9,
		SceneType:     "portrait",
		Explanation:   "Sharp focus, nice composition",
		TokensUsed:    150,
	}
	if err := valid.Validate(); err != nil {
		t.Errorf("valid score got error: %v", err)
	}

	outOfRange := valid
	outOfRange.Aesthetic = 1.5
	if err := outOfRange.Validate(); err == nil {
		t.Error("expected error for aesthetic > 1.0")
	}

	noExplanation := valid
	noExplanation.Explanation = ""
	if err := noExplanation.Validate(); err == nil {
		t.Error("expected error for empty explanation")
	}
}

func TestScoreRequestDefaults(t *testing.T) {
	req := ScoreRequest{PhotoPath: "/tmp/test.jpg"}
	if req.TokenBudget != 0 {
		t.Error("expected zero token budget before setting")
	}
}

func TestVLMScoreValidateIssuesLimit(t *testing.T) {
	score := VLMScore{
		Aesthetic:     0.5,
		Composition:   0.5,
		Expression:    0.5,
		TechnicalQual: 0.5,
		SceneType:     "portrait",
		Explanation:   "test",
		Issues:        []string{"a", "b", "c", "d"},
	}
	if err := score.Validate(); err == nil {
		t.Error("expected error for >3 issues")
	}
}

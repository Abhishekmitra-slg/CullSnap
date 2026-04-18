package vlm

import (
	"encoding/json"
	"fmt"
	"log/slog"
	"regexp"
)

var jsonBlockRe = regexp.MustCompile("(?s)```(?:json)?\\s*\\n?({.*?})\\s*\\n?```")

// extractFirstJSONObject finds the first balanced JSON object in text using brace-depth counting.
// This handles nested objects/arrays correctly, unlike a non-greedy regex.
func extractFirstJSONObject(text string) string {
	start := -1
	depth := 0
	inString := false
	escaped := false
	for i, ch := range text {
		if escaped {
			escaped = false
			continue
		}
		if ch == '\\' && inString {
			escaped = true
			continue
		}
		if ch == '"' {
			inString = !inString
			continue
		}
		if inString {
			continue
		}
		if ch == '{' {
			if depth == 0 {
				start = i
			}
			depth++
		} else if ch == '}' {
			depth--
			if depth == 0 && start >= 0 {
				return text[start : i+1]
			}
		}
	}
	return ""
}

// ParseVLMScore parses a VLM response string into a VLMScore using a three-tier strategy:
// Tier 1: Direct JSON unmarshal.
// Tier 2: Extract from markdown code fences.
// Tier 3: Extract first JSON object in text.
func ParseVLMScore(raw string) (*VLMScore, error) {
	slog.Debug("vlm: parsing VLMScore", "raw_len", len(raw))

	var s VLMScore

	// Tier 1: direct unmarshal.
	if err := json.Unmarshal([]byte(raw), &s); err == nil {
		slog.Debug("vlm: VLMScore parsed via direct JSON")
		return &s, nil
	}

	// Tier 2: extract from markdown code fences.
	if m := jsonBlockRe.FindStringSubmatch(raw); len(m) >= 2 {
		slog.Debug("vlm: VLMScore trying markdown code fence extraction")
		if err := json.Unmarshal([]byte(m[1]), &s); err == nil {
			slog.Debug("vlm: VLMScore parsed via markdown code fence")
			return &s, nil
		}
	}

	// Tier 3: extract first JSON object.
	if m := extractFirstJSONObject(raw); m != "" {
		slog.Debug("vlm: VLMScore trying first JSON object extraction")
		if err := json.Unmarshal([]byte(m), &s); err == nil {
			slog.Debug("vlm: VLMScore parsed via first JSON object")
			return &s, nil
		}
	}

	preview := raw
	if len(preview) > 100 {
		preview = preview[:100]
	}
	return nil, fmt.Errorf("vlm: failed to parse VLMScore from response: %q", preview)
}

// rawRankingResult is used for intermediate JSON decoding of ranking results.
// RankedPhoto entries use a 1-based PhotoIndex instead of a path.
type rawRankingResult struct {
	Ranked []struct {
		PhotoIndex int     `json:"photo_index"`
		Rank       int     `json:"rank"`
		Score      float64 `json:"score"`
		Notes      string  `json:"notes"`
	} `json:"ranked"`
	Explanation string `json:"explanation"`
	TokensUsed  int    `json:"tokens_used"`
}

// ParseRankingResult parses a VLM response string into a RankingResult using a three-tier strategy.
// photoPaths maps 1-based PhotoIndex values returned by the VLM to actual file paths.
func ParseRankingResult(raw string, photoPaths []string) (*RankingResult, error) {
	slog.Debug("vlm: parsing RankingResult", "raw_len", len(raw), "photo_count", len(photoPaths))

	var rr rawRankingResult

	// Tier 1: direct unmarshal.
	if err := json.Unmarshal([]byte(raw), &rr); err == nil {
		slog.Debug("vlm: RankingResult parsed via direct JSON")
		return mapRankingResult(&rr, photoPaths)
	}

	// Tier 2: extract from markdown code fences.
	if m := jsonBlockRe.FindStringSubmatch(raw); len(m) >= 2 {
		slog.Debug("vlm: RankingResult trying markdown code fence extraction")
		if err := json.Unmarshal([]byte(m[1]), &rr); err == nil {
			slog.Debug("vlm: RankingResult parsed via markdown code fence")
			return mapRankingResult(&rr, photoPaths)
		}
	}

	// Tier 3: extract first JSON object.
	if m := extractFirstJSONObject(raw); m != "" {
		slog.Debug("vlm: RankingResult trying first JSON object extraction")
		if err := json.Unmarshal([]byte(m), &rr); err == nil {
			slog.Debug("vlm: RankingResult parsed via first JSON object")
			return mapRankingResult(&rr, photoPaths)
		}
	}

	preview := raw
	if len(preview) > 100 {
		preview = preview[:100]
	}
	return nil, fmt.Errorf("vlm: failed to parse RankingResult from response: %q", preview)
}

// mapRankingResult converts a rawRankingResult to a RankingResult, resolving PhotoIndex to paths.
func mapRankingResult(rr *rawRankingResult, photoPaths []string) (*RankingResult, error) {
	result := &RankingResult{
		Explanation: rr.Explanation,
		TokensUsed:  rr.TokensUsed,
		Ranked:      make([]RankedPhoto, 0, len(rr.Ranked)),
	}
	for _, item := range rr.Ranked {
		idx := item.PhotoIndex - 1 // convert 1-based to 0-based
		var path string
		if idx >= 0 && idx < len(photoPaths) {
			path = photoPaths[idx]
		} else {
			slog.Warn("vlm: photo_index out of range, skipping", "photo_index", item.PhotoIndex, "photo_count", len(photoPaths))
			continue
		}
		result.Ranked = append(result.Ranked, RankedPhoto{
			PhotoPath: path,
			Rank:      item.Rank,
			Score:     item.Score,
			Notes:     item.Notes,
		})
	}
	slog.Debug("vlm: RankingResult mapped", "ranked_count", len(result.Ranked))
	return result, nil
}

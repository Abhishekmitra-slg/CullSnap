package vlm

import (
	"strings"
	"time"
)

// Config KV key constants for the SQLite app_config table.
const (
	ConfigKeyModelName          = "vlm_model_name"
	ConfigKeyModelVariant       = "vlm_model_variant"
	ConfigKeyBackend            = "vlm_backend"
	ConfigKeyKeepAlive          = "vlm_keep_alive"
	ConfigKeyIdleTimeout        = "vlm_idle_timeout_sec"
	ConfigKeyCustomInstructions = "vlm_custom_instructions"
	ConfigKeySetupComplete      = "vlm_setup_complete"

	// MaxCustomInstructionsLen is the maximum number of characters allowed in custom instructions.
	MaxCustomInstructionsLen = 500
)

// ManagerConfig holds runtime configuration for the VLM manager.
type ManagerConfig struct {
	KeepAlive        bool
	IdleTimeout      time.Duration
	MaxRestarts      int
	RestartBackoff   time.Duration
	ModelName        string
	PreferredBackend string
}

// DefaultManagerConfig returns a ManagerConfig populated with safe defaults.
func DefaultManagerConfig() ManagerConfig {
	return ManagerConfig{
		KeepAlive:        false,
		IdleTimeout:      5 * time.Minute,
		MaxRestarts:      3,
		RestartBackoff:   2 * time.Second,
		ModelName:        "",
		PreferredBackend: "auto",
	}
}

// sanitizePrefixes lists line prefixes that are rejected from custom instructions
// to prevent prompt injection attempts.
var sanitizePrefixes = []string{
	"System:",
	"Respond with:",
	"{",
	"}",
}

// SanitizeCustomInstructions removes potentially dangerous lines from user-supplied
// custom instructions and enforces the MaxCustomInstructionsLen limit.
//
// Lines are rejected if they start with "System:", "Respond with:", "{", or "}".
// The result is trimmed and truncated to MaxCustomInstructionsLen runes.
func SanitizeCustomInstructions(input string) string {
	lines := strings.Split(input, "\n")
	kept := make([]string, 0, len(lines))

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)
		rejected := false

		for _, prefix := range sanitizePrefixes {
			if strings.HasPrefix(trimmed, prefix) {
				rejected = true
				break
			}
		}

		if !rejected {
			kept = append(kept, line)
		}
	}

	result := strings.TrimSpace(strings.Join(kept, "\n"))

	// Truncate to MaxCustomInstructionsLen runes.
	runes := []rune(result)
	if len(runes) > MaxCustomInstructionsLen {
		result = string(runes[:MaxCustomInstructionsLen])
	}

	return result
}

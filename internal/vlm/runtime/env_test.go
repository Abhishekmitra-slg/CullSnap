package runtime

import (
	"strings"
	"testing"
)

func TestSanitizedEnvAllowList(t *testing.T) {
	env := sanitizedEnv("/venv", map[string]string{
		"PATH":       "/system/bin",
		"HOME":       "/home/x",
		"PYTHONPATH": "/evil",
		"PIP_INDEX":  "http://attacker",
		"LANG":       "ignored",
	})
	joined := strings.Join(env, "\x00")
	if !strings.Contains(joined, "VIRTUAL_ENV=/venv") {
		t.Fatal("VIRTUAL_ENV missing")
	}
	if strings.Contains(joined, "PYTHONPATH=") {
		t.Fatal("PYTHONPATH leaked")
	}
	if strings.Contains(joined, "PIP_INDEX=") {
		t.Fatal("PIP_INDEX leaked")
	}
	if !strings.Contains(joined, "LANG=C.UTF-8") {
		t.Fatal("LANG override missing")
	}
}

func TestSanitizedEnvEmptyVenv(t *testing.T) {
	env := sanitizedEnv("", map[string]string{
		"PATH": "/system/bin",
		"HOME": "/home/x",
	})
	joined := strings.Join(env, "\x00")
	// VIRTUAL_ENV must be present (empty value) so uv knows it is not inside a venv.
	if !strings.Contains(joined, "VIRTUAL_ENV=") {
		t.Fatal("VIRTUAL_ENV entry missing")
	}
	// PATH must not be prefixed with "/bin:" when venvPath is empty.
	for _, entry := range env {
		if entry == "PATH=/bin:/system/bin" {
			t.Fatalf("spurious /bin: prefix in PATH: %s", entry)
		}
	}
	// PATH must equal the inherited value without any venv prefix.
	if !strings.Contains(joined, "PATH=/system/bin") {
		t.Fatalf("PATH not inherited correctly: %s", joined)
	}
}

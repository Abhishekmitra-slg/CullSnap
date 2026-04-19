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

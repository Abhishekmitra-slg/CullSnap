package device

import (
	"encoding/base64"
	"encoding/json"
	"testing"
	"unicode/utf16"
)

// --- encodePowerShellScript tests ---

// decodePS decodes a base64 UTF-16LE encoded PowerShell script back to a Go string.
// Used to verify roundtrips.
func decodePS(t *testing.T, encoded string) string {
	t.Helper()
	raw, err := base64.StdEncoding.DecodeString(encoded)
	if err != nil {
		t.Fatalf("base64 decode failed: %v", err)
	}
	if len(raw)%2 != 0 {
		t.Fatalf("decoded bytes have odd length %d, not valid UTF-16LE", len(raw))
	}
	u16 := make([]uint16, len(raw)/2)
	for i := range u16 {
		u16[i] = uint16(raw[2*i]) | uint16(raw[2*i+1])<<8
	}
	runes := utf16.Decode(u16)
	return string(runes)
}

func TestEncodePowerShellScript_SimpleString(t *testing.T) {
	script := `Write-Output "hello world"`
	encoded := encodePowerShellScript(script)
	if encoded == "" {
		t.Fatal("encodePowerShellScript returned empty string")
	}
	got := decodePS(t, encoded)
	if got != script {
		t.Errorf("roundtrip mismatch: got %q, want %q", got, script)
	}
}

func TestEncodePowerShellScript_Unicode(t *testing.T) {
	script := "Write-Output \"Ünïcödé テスト 日本語\""
	encoded := encodePowerShellScript(script)
	if encoded == "" {
		t.Fatal("encodePowerShellScript returned empty string for unicode input")
	}
	got := decodePS(t, encoded)
	if got != script {
		t.Errorf("unicode roundtrip mismatch: got %q, want %q", got, script)
	}
}

func TestEncodePowerShellScript_EmptyString(t *testing.T) {
	encoded := encodePowerShellScript("")
	// base64 of zero bytes is the empty string.
	if encoded != "" {
		t.Errorf("expected empty base64 for empty script, got %q", encoded)
	}
}

// --- parseProgressLine tests ---

func mustMarshal(t *testing.T, v any) []byte {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("json.Marshal: %v", err)
	}
	return b
}

func TestParseProgressLine_EnumerateDone(t *testing.T) {
	line := mustMarshal(t, map[string]any{
		"type":   "enumerate_done",
		"total":  42,
		"device": "Apple iPhone",
	})
	ev, err := parseProgressLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "enumerate_done" {
		t.Errorf("Type = %q, want %q", ev.Type, "enumerate_done")
	}
	if ev.Total != 42 {
		t.Errorf("Total = %d, want 42", ev.Total)
	}
	if ev.Device != "Apple iPhone" {
		t.Errorf("Device = %q, want %q", ev.Device, "Apple iPhone")
	}
}

func TestParseProgressLine_Copied(t *testing.T) {
	line := mustMarshal(t, map[string]any{
		"type":     "copied",
		"name":     "IMG_0001.JPG",
		"progress": 5,
		"total":    20,
		"copied":   5,
	})
	ev, err := parseProgressLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "copied" {
		t.Errorf("Type = %q, want %q", ev.Type, "copied")
	}
	if ev.Name != "IMG_0001.JPG" {
		t.Errorf("Name = %q, want %q", ev.Name, "IMG_0001.JPG")
	}
	if ev.Progress != 5 {
		t.Errorf("Progress = %d, want 5", ev.Progress)
	}
	if ev.Total != 20 {
		t.Errorf("Total = %d, want 20", ev.Total)
	}
	if ev.Copied != 5 {
		t.Errorf("Copied = %d, want 5", ev.Copied)
	}
}

func TestParseProgressLine_Error(t *testing.T) {
	line := mustMarshal(t, map[string]any{
		"type":    "error",
		"code":    "ACCESS_DENIED",
		"message": "cannot access device",
	})
	ev, err := parseProgressLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "error" {
		t.Errorf("Type = %q, want %q", ev.Type, "error")
	}
	if ev.Code != "ACCESS_DENIED" {
		t.Errorf("Code = %q, want %q", ev.Code, "ACCESS_DENIED")
	}
	if ev.Message != "cannot access device" {
		t.Errorf("Message = %q, want %q", ev.Message, "cannot access device")
	}
}

func TestParseProgressLine_Complete(t *testing.T) {
	line := mustMarshal(t, map[string]any{
		"type":   "complete",
		"copied": 100,
		"total":  100,
	})
	ev, err := parseProgressLine(line)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if ev.Type != "complete" {
		t.Errorf("Type = %q, want %q", ev.Type, "complete")
	}
	if ev.Copied != 100 {
		t.Errorf("Copied = %d, want 100", ev.Copied)
	}
	if ev.Total != 100 {
		t.Errorf("Total = %d, want 100", ev.Total)
	}
}

func TestParseProgressLine_InvalidJSON(t *testing.T) {
	_, err := parseProgressLine([]byte(`not json {`))
	if err == nil {
		t.Fatal("expected error for invalid JSON, got nil")
	}
}

func TestParseProgressLine_EmptyLine(t *testing.T) {
	_, err := parseProgressLine([]byte{})
	if err == nil {
		t.Fatal("expected error for empty line, got nil")
	}
}

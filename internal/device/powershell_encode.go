package device

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"unicode/utf16"
)

// ProgressEvent is a structured JSON line emitted by CullSnap PowerShell scripts
// to report import progress back to the Go host over stdout.
type ProgressEvent struct {
	Type     string `json:"type"`
	Name     string `json:"name,omitempty"`
	Progress int    `json:"progress,omitempty"`
	Total    int    `json:"total,omitempty"`
	Copied   int    `json:"copied,omitempty"`
	Device   string `json:"device,omitempty"`
	Code     string `json:"code,omitempty"`
	Message  string `json:"message,omitempty"`
}

// encodePowerShellScript encodes a PowerShell script as base64 UTF-16LE suitable
// for use with powershell.exe -EncodedCommand. Encoding prevents quoting issues
// and allows arbitrary scripts to be passed safely on the command line.
func encodePowerShellScript(script string) string {
	runes := []rune(script)
	u16 := utf16.Encode(runes)
	buf := make([]byte, len(u16)*2)
	for i, c := range u16 {
		buf[2*i] = byte(c)
		buf[2*i+1] = byte(c >> 8)
	}
	return base64.StdEncoding.EncodeToString(buf)
}

// parseProgressLine decodes a single JSON progress line emitted by a CullSnap
// PowerShell script. Returns an error if the line is empty or not valid JSON.
func parseProgressLine(line []byte) (ProgressEvent, error) {
	if len(line) == 0 {
		return ProgressEvent{}, fmt.Errorf("powershell: empty progress line")
	}
	var ev ProgressEvent
	if err := json.Unmarshal(line, &ev); err != nil {
		return ProgressEvent{}, fmt.Errorf("powershell: parse progress line: %w", err)
	}
	return ev, nil
}

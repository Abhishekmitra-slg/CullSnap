package app

import (
	"testing"
	"time"
)

func TestIsQuickRestart(t *testing.T) {
	tests := []struct {
		name      string
		lastStart time.Time
		now       time.Time
		want      bool
	}{
		{"first run ever", time.Time{}, time.Now(), false},
		{"restart after 1 second", time.Now().Add(-1 * time.Second), time.Now(), true},
		{"restart after 1 hour", time.Now().Add(-1 * time.Hour), time.Now(), false},
		{"restart after 29 seconds", time.Now().Add(-29 * time.Second), time.Now(), true},
		{"restart after 31 seconds", time.Now().Add(-31 * time.Second), time.Now(), false},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := isQuickRestart(tc.lastStart, tc.now)
			if got != tc.want {
				t.Errorf("isQuickRestart() = %v, want %v", got, tc.want)
			}
		})
	}
}

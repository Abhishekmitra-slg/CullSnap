package model

import (
	"testing"
	"time"
)

func TestPhotoModel(t *testing.T) {
	now := time.Now()
	p := Photo{
		Path:    "/tmp/test.jpg",
		Width:   1920,
		Height:  1080,
		Size:    1024,
		TakenAt: now,
	}

	if p.Path != "/tmp/test.jpg" {
		t.Error("Photo struct field mismatch")
	}
}

func TestSessionModel(t *testing.T) {
	s := Session{
		ID:        "session-1",
		CreatedAt: time.Now(),
		Name:      "Test Session",
	}

	if s.ID != "session-1" {
		t.Error("Session struct field mismatch")
	}
}

package vlm

import (
	"testing"
)

func TestTokenTrackerLocalUnlimited(t *testing.T) {
	tracker := NewTokenTracker(TokenBudgetPolicy{Mode: "local"})

	if !tracker.CanSpend(10000) {
		t.Fatal("expected CanSpend(10000)=true for local mode before any recording")
	}

	tracker.Record(5000)

	if !tracker.CanSpend(10000) {
		t.Fatal("expected CanSpend(10000)=true for local mode after Record(5000)")
	}
}

func TestTokenTrackerCloudBudget(t *testing.T) {
	tracker := NewTokenTracker(TokenBudgetPolicy{
		Mode:          "cloud",
		MaxPerSession: 1000,
	})

	if !tracker.CanSpend(500) {
		t.Fatal("expected CanSpend(500)=true before any recording")
	}

	tracker.Record(800)

	if tracker.CanSpend(500) {
		t.Fatal("expected CanSpend(500)=false after Record(800): 800+500=1300 > 1000")
	}
	if tracker.CanSpend(201) {
		t.Fatal("expected CanSpend(201)=false after Record(800): 800+201=1001 > 1000")
	}
	if !tracker.CanSpend(200) {
		t.Fatal("expected CanSpend(200)=true after Record(800): 800+200=1000 <= 1000")
	}
}

func TestTokenTrackerRemaining(t *testing.T) {
	tracker := NewTokenTracker(TokenBudgetPolicy{
		Mode:          "cloud",
		MaxPerSession: 1000,
	})

	tracker.Record(300)

	if got := tracker.Remaining(); got != 700 {
		t.Fatalf("expected Remaining()=700, got %d", got)
	}
}

func TestTokenTrackerRemainingLocal(t *testing.T) {
	tracker := NewTokenTracker(TokenBudgetPolicy{Mode: "local"})

	tracker.Record(999999)

	if got := tracker.Remaining(); got != -1 {
		t.Fatalf("expected Remaining()=-1 for local mode, got %d", got)
	}
}

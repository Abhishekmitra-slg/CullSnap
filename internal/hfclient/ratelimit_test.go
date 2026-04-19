package hfclient

import (
	"net/http"
	"testing"
	"time"
)

func TestParseRateLimitHeader(t *testing.T) {
	h := http.Header{}
	h.Set("RateLimit", `r=3, t=42`)
	got, ok := parseRateLimit(h)
	if !ok || got.Remaining != 3 || got.Reset != 42*time.Second {
		t.Fatalf("got %+v ok=%v", got, ok)
	}
}

func TestParseRateLimitMissing(t *testing.T) {
	if _, ok := parseRateLimit(http.Header{}); ok {
		t.Fatal("missing header should return ok=false")
	}
}

func TestParseRateLimitMalformed(t *testing.T) {
	h := http.Header{}
	h.Set("RateLimit", `garbage`)
	if _, ok := parseRateLimit(h); ok {
		t.Fatal("malformed header should return ok=false")
	}
}

func TestBackoffSchedule(t *testing.T) {
	durs := []time.Duration{}
	for i := 0; i < 5; i++ {
		durs = append(durs, backoffFor(i))
	}
	// Each strictly larger than the previous.
	for i := 1; i < len(durs); i++ {
		if durs[i] <= durs[i-1] {
			t.Fatalf("backoff not monotonic: %v", durs)
		}
	}
}

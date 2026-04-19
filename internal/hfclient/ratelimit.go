package hfclient

import (
	"math/rand/v2" // nosemgrep: go.lang.security.audit.crypto.math_random.math-random-used -- jitter for retry backoff, not security-sensitive
	"net/http"
	"strconv"
	"strings"
	"time"
)

// RateLimitInfo summarizes IETF draft-ietf-httpapi-ratelimit-headers headers.
type RateLimitInfo struct {
	Remaining int
	Reset     time.Duration
}

// parseRateLimit reads the IETF "RateLimit" header (e.g. "r=3, t=42").
// Returns ok=false if header is absent or unparseable.
func parseRateLimit(h http.Header) (RateLimitInfo, bool) {
	raw := h.Get("RateLimit")
	if raw == "" {
		return RateLimitInfo{}, false
	}
	var info RateLimitInfo
	var sawR, sawT bool
	for _, part := range strings.Split(raw, ",") {
		part = strings.TrimSpace(part)
		kv := strings.SplitN(part, "=", 2)
		if len(kv) != 2 {
			continue
		}
		v, err := strconv.Atoi(strings.TrimSpace(kv[1]))
		if err != nil {
			continue
		}
		switch strings.TrimSpace(kv[0]) {
		case "r":
			info.Remaining = v
			sawR = true
		case "t":
			info.Reset = time.Duration(v) * time.Second
			sawT = true
		}
	}
	if !sawR && !sawT {
		return RateLimitInfo{}, false
	}
	return info, true
}

// backoffFor returns the wait duration before retry attempt n (0-indexed),
// using exponential schedule 1s, 2s, 4s, 8s, 16s with ±20% jitter.
func backoffFor(attempt int) time.Duration {
	base := time.Duration(1<<attempt) * time.Second
	if base > 16*time.Second {
		base = 16 * time.Second
	}
	jitter := time.Duration(rand.Int64N(int64(base) / 5))
	if rand.IntN(2) == 0 {
		return base - jitter
	}
	return base + jitter
}

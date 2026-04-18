package vlm

import (
	"cullsnap/internal/logger"
	"sync"
)

// TokenTracker tracks token usage against a TokenBudgetPolicy.
type TokenTracker struct {
	mu     sync.Mutex
	policy TokenBudgetPolicy
	used   int
}

// NewTokenTracker creates a TokenTracker governed by the given policy.
func NewTokenTracker(policy TokenBudgetPolicy) *TokenTracker {
	if logger.Log != nil {
		logger.Log.Debug("vlm: token tracker created", "mode", policy.Mode, "max_per_session", policy.MaxPerSession)
	}
	return &TokenTracker{policy: policy}
}

// CanSpend reports whether spending tokens additional tokens is permitted.
// Local mode always returns true. Cloud mode returns true when
// used+tokens <= MaxPerSession, or when MaxPerSession is 0 (unlimited).
func (t *TokenTracker) CanSpend(tokens int) bool {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.policy.Mode == "local" {
		if logger.Log != nil {
			logger.Log.Debug("vlm: CanSpend local=true (unlimited)", "tokens", tokens)
		}
		return true
	}

	if t.policy.MaxPerSession == 0 {
		if logger.Log != nil {
			logger.Log.Debug("vlm: CanSpend cloud unlimited (MaxPerSession=0)", "tokens", tokens)
		}
		return true
	}

	ok := t.used+tokens <= t.policy.MaxPerSession
	if logger.Log != nil {
		logger.Log.Debug("vlm: CanSpend cloud check",
			"tokens", tokens,
			"used", t.used,
			"max_per_session", t.policy.MaxPerSession,
			"can_spend", ok,
		)
	}
	return ok
}

// Record adds n tokens to the usage counter under the lock.
func (t *TokenTracker) Record(tokens int) {
	t.mu.Lock()
	defer t.mu.Unlock()

	t.used += tokens
	if logger.Log != nil {
		logger.Log.Debug("vlm: token usage recorded", "recorded", tokens, "total_used", t.used)
	}
}

// Remaining returns the number of tokens still available in this session.
// Local mode returns -1 (unlimited). Cloud mode returns MaxPerSession - used,
// floored at 0.
func (t *TokenTracker) Remaining() int {
	t.mu.Lock()
	defer t.mu.Unlock()

	if t.policy.Mode == "local" {
		return -1
	}

	rem := t.policy.MaxPerSession - t.used
	if rem < 0 {
		rem = 0
	}
	if logger.Log != nil {
		logger.Log.Debug("vlm: token remaining query", "remaining", rem, "used", t.used)
	}
	return rem
}

// Used returns the total tokens recorded so far.
func (t *TokenTracker) Used() int {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.used
}

// Reset clears the usage counter to zero.
func (t *TokenTracker) Reset() {
	t.mu.Lock()
	defer t.mu.Unlock()

	if logger.Log != nil {
		logger.Log.Debug("vlm: token tracker reset", "was_used", t.used)
	}
	t.used = 0
}

package vlm

import "sync"

// boundedBuffer is a thread-safe byte buffer that caps retained content at maxBytes,
// evicting from the front when the cap is exceeded. Used to capture subprocess stderr
// for crash diagnostics without growing unbounded over a long-lived process.
type boundedBuffer struct {
	mu  sync.Mutex
	buf []byte
	max int
}

func newBoundedBuffer(max int) *boundedBuffer {
	if max <= 0 {
		max = 4096
	}
	return &boundedBuffer{max: max}
}

// Write appends p and truncates older bytes to stay within the cap.
// Always returns len(p), nil so it satisfies io.Writer cleanly.
func (b *boundedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.buf = append(b.buf, p...)
	if len(b.buf) > b.max {
		b.buf = b.buf[len(b.buf)-b.max:]
	}
	return len(p), nil
}

// String returns a copy of the currently retained bytes.
func (b *boundedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return string(b.buf)
}

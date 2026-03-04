package cmd

import (
	"io"
	"sync/atomic"
)

// cappedWriter wraps a writer and stops writing after maxBytes.
// Writes beyond the cap silently succeed (no error) but produce no output.
// Thread-safe via atomic counter.
type cappedWriter struct {
	w        io.Writer
	written  int64
	maxBytes int64
}

// newCappedWriter creates a writer that stops after maxBytes.
func newCappedWriter(w io.Writer, maxBytes int64) *cappedWriter {
	return &cappedWriter{w: w, maxBytes: maxBytes}
}

func (c *cappedWriter) Write(p []byte) (int, error) {
	cur := atomic.LoadInt64(&c.written)
	if cur >= c.maxBytes {
		return len(p), nil // silently discard
	}
	remaining := c.maxBytes - cur
	toWrite := p
	if int64(len(p)) > remaining {
		toWrite = p[:remaining]
	}
	n, err := c.w.Write(toWrite)
	atomic.AddInt64(&c.written, int64(n))
	if err != nil {
		return n, err
	}
	return len(p), nil // report full write to caller
}

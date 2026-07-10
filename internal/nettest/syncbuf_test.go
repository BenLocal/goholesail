package nettest

import (
	"bytes"
	"sync"
)

// syncBuf is an io.Writer + reader safe for concurrent use, so a test can read
// a logger's output while background goroutines still write to it.
type syncBuf struct {
	mu sync.Mutex
	b  bytes.Buffer
}

func (s *syncBuf) Write(p []byte) (int, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.Write(p)
}

func (s *syncBuf) String() string {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.b.String()
}

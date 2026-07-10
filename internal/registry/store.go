package registry

import (
	"sync"
	"time"
)

type entry struct {
	svc     Service
	expires time.Time
}

// Store is an in-memory, TTL-expiring service directory. Safe for concurrent use.
type Store struct {
	mu  sync.Mutex
	m   map[string]entry
	now func() time.Time // injectable clock for tests
}

// NewStore returns an empty directory using the real clock.
func NewStore() *Store {
	return &Store{m: make(map[string]entry), now: time.Now}
}

// Put stores or overwrites a service with the given TTL.
func (s *Store) Put(svc Service, ttl time.Duration) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[svc.Name] = entry{svc: svc, expires: s.now().Add(ttl)}
}

// Renew extends an existing entry's TTL. Returns false if it is absent/expired.
func (s *Store) Renew(name string, ttl time.Duration) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[name]
	if !ok || !e.expires.After(s.now()) {
		return false
	}
	e.expires = s.now().Add(ttl)
	s.m[name] = e
	return true
}

// Remove deletes an entry if present.
func (s *Store) Remove(name string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, name)
}

// Get returns a live service by name.
func (s *Store) Get(name string) (Service, bool) {
	s.mu.Lock()
	defer s.mu.Unlock()
	e, ok := s.m[name]
	if !ok || !e.expires.After(s.now()) {
		return Service{}, false
	}
	return e.svc, true
}

// List returns all live services, optionally filtered by tag ("" = all).
func (s *Store) List(tag string) []Service {
	s.mu.Lock()
	defer s.mu.Unlock()
	now := s.now()
	out := make([]Service, 0, len(s.m))
	for _, e := range s.m {
		if !e.expires.After(now) {
			continue
		}
		if tag != "" && !hasTag(e.svc.Tags, tag) {
			continue
		}
		out = append(out, e.svc)
	}
	return out
}

func hasTag(tags []string, tag string) bool {
	for _, t := range tags {
		if t == tag {
			return true
		}
	}
	return false
}

package registry

import (
	"testing"
	"time"
)

func TestStorePutGet(t *testing.T) {
	s := NewStore()
	s.Put(Service{Name: "a", PeerID: "p", Hub: "h"}, time.Minute)
	got, ok := s.Get("a")
	if !ok || got.PeerID != "p" {
		t.Fatalf("Get(a) = %+v, %v", got, ok)
	}
	if _, ok := s.Get("missing"); ok {
		t.Fatal("Get(missing) should be false")
	}
}

func TestStoreExpiry(t *testing.T) {
	s := NewStore()
	now := time.Unix(1000, 0)
	s.now = func() time.Time { return now }
	s.Put(Service{Name: "a", PeerID: "p"}, 30*time.Second)
	now = now.Add(20 * time.Second)
	if _, ok := s.Get("a"); !ok {
		t.Fatal("a should still be live at +20s")
	}
	now = now.Add(20 * time.Second) // +40s total, past 30s TTL
	if _, ok := s.Get("a"); ok {
		t.Fatal("a should have expired at +40s")
	}
}

func TestStoreRenew(t *testing.T) {
	s := NewStore()
	now := time.Unix(1000, 0)
	s.now = func() time.Time { return now }
	s.Put(Service{Name: "a", PeerID: "p"}, 30*time.Second)
	now = now.Add(20 * time.Second)
	if ok := s.Renew("a", 30*time.Second); !ok {
		t.Fatal("Renew(a) should succeed")
	}
	now = now.Add(20 * time.Second) // +40s, but renewed at +20s so expires at +50s
	if _, ok := s.Get("a"); !ok {
		t.Fatal("a should still be live after renew")
	}
	if ok := s.Renew("missing", time.Minute); ok {
		t.Fatal("Renew(missing) should fail")
	}
}

func TestStoreListAndRemove(t *testing.T) {
	s := NewStore()
	s.Put(Service{Name: "a", Tags: []string{"ssh"}}, time.Minute)
	s.Put(Service{Name: "b", Tags: []string{"web"}}, time.Minute)
	if got := s.List(""); len(got) != 2 {
		t.Fatalf("List() = %d, want 2", len(got))
	}
	if got := s.List("web"); len(got) != 1 || got[0].Name != "b" {
		t.Fatalf("List(web) = %+v", got)
	}
	s.Remove("a")
	if _, ok := s.Get("a"); ok {
		t.Fatal("a should be removed")
	}
}

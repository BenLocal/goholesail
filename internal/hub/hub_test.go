package hub

import "testing"

// TestNewSeedStableIdentity is the point of --seed: the same seed must yield the
// same peer id across restarts, so the --hub string never changes.
func TestNewSeedStableIdentity(t *testing.T) {
	h1, err := New("/ip4/127.0.0.1/tcp/0", "hub-seed", "", "")
	if err != nil {
		t.Fatalf("hub 1: %v", err)
	}
	defer h1.Close()
	h2, err := New("/ip4/127.0.0.1/tcp/0", "hub-seed", "", "")
	if err != nil {
		t.Fatalf("hub 2: %v", err)
	}
	defer h2.Close()
	if h1.ID() != h2.ID() {
		t.Fatalf("same seed must give same peer id, got %s vs %s", h1.ID(), h2.ID())
	}

	h3, err := New("/ip4/127.0.0.1/tcp/0", "other-seed", "", "")
	if err != nil {
		t.Fatalf("hub 3: %v", err)
	}
	defer h3.Close()
	if h3.ID() == h1.ID() {
		t.Fatal("a different seed must give a different peer id")
	}
}

// TestNewEmptySeedRandom confirms the empty-seed default stays ephemeral.
func TestNewEmptySeedRandom(t *testing.T) {
	h1, err := New("/ip4/127.0.0.1/tcp/0", "", "", "")
	if err != nil {
		t.Fatalf("hub 1: %v", err)
	}
	defer h1.Close()
	h2, err := New("/ip4/127.0.0.1/tcp/0", "", "", "")
	if err != nil {
		t.Fatalf("hub 2: %v", err)
	}
	defer h2.Close()
	if h1.ID() == h2.ID() {
		t.Fatal("empty seed must be random; two hubs should differ")
	}
}

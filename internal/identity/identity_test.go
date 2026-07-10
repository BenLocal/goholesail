package identity

import (
	"testing"

	"github.com/libp2p/go-libp2p/core/peer"
)

func peerID(t *testing.T, seed string) peer.ID {
	t.Helper()
	priv, err := FromSeed(seed)
	if err != nil {
		t.Fatalf("FromSeed(%q): %v", seed, err)
	}
	id, err := peer.IDFromPublicKey(priv.GetPublic())
	if err != nil {
		t.Fatalf("IDFromPublicKey: %v", err)
	}
	return id
}

func TestFromSeedIsDeterministic(t *testing.T) {
	if a, b := peerID(t, "fixed-seed"), peerID(t, "fixed-seed"); a != b {
		t.Fatalf("same seed gave different PeerIDs: %s vs %s", a, b)
	}
}

func TestFromSeedDistinctSeeds(t *testing.T) {
	if a, b := peerID(t, "seed-one"), peerID(t, "seed-two"); a == b {
		t.Fatal("different seeds gave the same PeerID")
	}
}

func TestRandomIsUnique(t *testing.T) {
	p1, err := Random()
	if err != nil {
		t.Fatalf("Random: %v", err)
	}
	p2, err := Random()
	if err != nil {
		t.Fatalf("Random: %v", err)
	}
	if p1.Equals(p2) {
		t.Fatal("two Random keys were equal")
	}
}

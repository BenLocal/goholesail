// Package identity derives libp2p private keys.
//
// FromSeed maps an arbitrary seed string to a deterministic Ed25519 key, so a
// fixed --seed yields a stable PeerID (and therefore a stable connection
// string) across restarts. Random produces an ephemeral key for clients.
package identity

import (
	"bytes"
	"crypto/rand"
	"crypto/sha256"

	"github.com/libp2p/go-libp2p/core/crypto"
)

// domain separates our seed hashing from any other use of the same seed.
const domain = "goholesail/identity/v1:"

// FromSeed derives a deterministic Ed25519 libp2p private key from seed.
func FromSeed(seed string) (crypto.PrivKey, error) {
	// Ed25519 needs exactly 32 bytes of seed material; SHA-256 gives us that.
	sum := sha256.Sum256([]byte(domain + seed))
	priv, _, err := crypto.GenerateEd25519Key(bytes.NewReader(sum[:]))
	if err != nil {
		return nil, err
	}
	return priv, nil
}

// Random returns a fresh Ed25519 libp2p private key.
func Random() (crypto.PrivKey, error) {
	priv, _, err := crypto.GenerateEd25519Key(rand.Reader)
	if err != nil {
		return nil, err
	}
	return priv, nil
}

// Package swarm derives a libp2p private-network (pnet) PSK from a passphrase and
// builds the libp2p options that put every node onto that private swarm. A shared
// passphrase is the "network password": only nodes holding it complete the libp2p
// wire handshake, so the swarm is invisible to scanners that lack it.
package swarm

import (
	"crypto/sha256"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/pnet"
	"github.com/libp2p/go-libp2p/p2p/transport/tcp"
)

// domain separates our swarm-key hashing from any other use of the same passphrase.
const domain = "goholesail/swarm/v1:"

// PSK derives a 32-byte pnet pre-shared key from passphrase via a
// domain-separated SHA-256, mapping any-length input to the fixed 32 bytes pnet
// requires.
func PSK(passphrase string) pnet.PSK {
	sum := sha256.Sum256([]byte(domain + passphrase))
	return pnet.PSK(sum[:])
}

// Options returns the libp2p options that join the private swarm keyed by
// passphrase. An empty passphrase returns nil, leaving libp2p at its defaults
// (no private network — current behavior). A non-empty passphrase enables the
// pnet protector and pins the transport to TCP: pnet is a stream-transport
// protector and QUIC/WebTransport do not support it, so the default transport
// set (which includes QUIC) must be narrowed to TCP.
func Options(passphrase string) []libp2p.Option {
	if passphrase == "" {
		return nil
	}
	return []libp2p.Option{
		libp2p.PrivateNetwork(PSK(passphrase)),
		libp2p.Transport(tcp.NewTCPTransport),
	}
}

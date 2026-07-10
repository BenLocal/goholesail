// Package hub runs the goholesail central node: a libp2p host that also
// provides a circuit-relay v2 service so NAT-bound hosts are reachable.
package hub

import (
	"fmt"

	"github.com/BenLocal/goholesail/internal/identity"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
)

// New creates a libp2p host listening on listenAddr (a multiaddr string such as
// "/ip4/0.0.0.0/tcp/4001") and starts the relay service on it. seed sets a
// stable identity; empty means a random ephemeral key. A fixed seed keeps the
// hub's peer id constant across restarts, so the --hub string and every ghs://
// connection string that embeds it stay valid. The caller owns closing the host.
// The relay runs with infinite per-connection limits (WithInfiniteLimits) so
// long-lived tunnels are not reset at the default 2 min / 128 KB.
func New(listenAddr, seed string) (host.Host, error) {
	priv, err := keyFor(seed)
	if err != nil {
		return nil, err
	}
	h, err := libp2p.New(libp2p.Identity(priv), libp2p.ListenAddrStrings(listenAddr))
	if err != nil {
		return nil, fmt.Errorf("hub: new host: %w", err)
	}
	if _, err := relay.New(h, relay.WithInfiniteLimits()); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("hub: start relay: %w", err)
	}
	return h, nil
}

// keyFor returns a deterministic key when seed is set, else a random one.
func keyFor(seed string) (crypto.PrivKey, error) {
	if seed != "" {
		return identity.FromSeed(seed)
	}
	return identity.Random()
}

// P2pAddrs returns the host's dialable /p2p multiaddrs (transport addr + peer
// id) as strings, suitable for pasting into a --hub flag or a connection string.
func P2pAddrs(h host.Host) []string {
	info := peer.AddrInfo{ID: h.ID(), Addrs: h.Addrs()}
	maddrs, err := peer.AddrInfoToP2pAddrs(&info)
	if err != nil {
		return nil
	}
	out := make([]string, 0, len(maddrs))
	for _, m := range maddrs {
		out = append(out, m.String())
	}
	return out
}

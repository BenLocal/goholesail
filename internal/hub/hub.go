// Package hub runs the goholesail central node: a libp2p host that also
// provides a circuit-relay v2 service so NAT-bound hosts are reachable.
package hub

import (
	"fmt"

	"github.com/BenLocal/goholesail/internal/identity"
	"github.com/BenLocal/goholesail/internal/swarm"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	"github.com/multiformats/go-multiaddr"
)

// New creates a libp2p host listening on listenAddr (a multiaddr string such as
// "/ip4/0.0.0.0/tcp/4001") and starts the relay service on it. seed sets a
// stable identity; empty means a random ephemeral key. A fixed seed keeps the
// hub's peer id constant across restarts, so the --hub string and every ghs://
// connection string that embeds it stay valid. The caller owns closing the host.
// The relay runs with infinite per-connection limits (WithInfiniteLimits) so
// long-lived tunnels are not reset at the default 2 min / 128 KB.
//
// announceAddr is an optional multiaddr (e.g. "/ip4/203.0.113.10/tcp/4001") that
// will be appended to the host's advertised addresses. This is useful when the
// hub runs behind NAT or inside a Docker container and cannot see its public IP.
//
// swarmKey, when non-empty, puts the hub on a private swarm (pnet): only peers
// sharing the same key can connect, and the transport is pinned to TCP. It must
// match the --swarm-key given to every host and client. Empty = public (default).
//
// NOTE: New now takes four positional args (listen/seed/announce/swarmKey). This
// is at the edge of what positional params should carry; a future refactor to an
// Options struct is reasonable if it grows further (out of scope here).
func New(listenAddr, seed, announceAddr, swarmKey string) (host.Host, error) {
	priv, err := keyFor(seed)
	if err != nil {
		return nil, err
	}
	opts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.ListenAddrStrings(listenAddr),
	}
	if announceAddr != "" {
		a, err := multiaddr.NewMultiaddr(announceAddr)
		if err != nil {
			return nil, fmt.Errorf("hub: parse announce address: %w", err)
		}
		opts = append(opts, libp2p.AddrsFactory(func(addrs []multiaddr.Multiaddr) []multiaddr.Multiaddr {
			return append(addrs, a)
		}))
	}
	opts = append(opts, swarm.Options(swarmKey)...)
	h, err := libp2p.New(opts...)
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

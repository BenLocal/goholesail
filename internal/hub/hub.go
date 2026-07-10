// Package hub runs the goholesail central node: a libp2p host that also
// provides a circuit-relay v2 service so NAT-bound hosts are reachable.
package hub

import (
	"fmt"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
)

// New creates a libp2p host listening on listenAddr (a multiaddr string such as
// "/ip4/0.0.0.0/tcp/4001") and starts the relay service on it. The caller owns
// closing the returned host.
func New(listenAddr string) (host.Host, error) {
	h, err := libp2p.New(libp2p.ListenAddrStrings(listenAddr))
	if err != nil {
		return nil, fmt.Errorf("hub: new host: %w", err)
	}
	if _, err := relay.New(h); err != nil {
		_ = h.Close()
		return nil, fmt.Errorf("hub: start relay: %w", err)
	}
	return h, nil
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

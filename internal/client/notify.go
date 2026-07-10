package client

import (
	"log"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	ma "github.com/multiformats/go-multiaddr"
)

// attachConnLogger logs connect/disconnect events on h, tagging each new
// connection as direct or relayed so operators can see whether a tunnel is
// hole-punched or riding the relay. A nil logger is a no-op.
func attachConnLogger(h host.Host, logger *log.Logger) {
	if logger == nil {
		return
	}
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, c network.Conn) {
			logger.Printf("connected %s via %s", c.RemotePeer(), connKind(c))
		},
		DisconnectedF: func(_ network.Network, c network.Conn) {
			logger.Printf("disconnected from %s", c.RemotePeer())
		},
	})
}

// connKind reports whether c rode a relay circuit or a direct connection.
//
// It inspects the remote multiaddr for a /p2p-circuit component rather than
// conn.Stat().Limited. That is deliberate: the hub runs its relay with
// WithInfiniteLimits (Task 1), so the reservation carries no Limit, and
// go-libp2p only sets Stat().Limited=true when the relay sends a Limit
// (see circuitv2/client/dial.go). Every relayed connection through this hub
// therefore reports Limited=false, so the multiaddr is the reliable signal.
func connKind(c network.Conn) string {
	if _, err := c.RemoteMultiaddr().ValueForProtocol(ma.P_CIRCUIT); err == nil {
		return "relay"
	}
	return "direct"
}

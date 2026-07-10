package hub

import (
	"log"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
)

// AttachConnLogger logs peer connect/disconnect events on h to logger. These
// surface relay reservations (hosts) and circuit dials (clients) as they land.
// A nil logger is a no-op, so tests and library users stay quiet unless the hub
// binary opts in.
func AttachConnLogger(h host.Host, logger *log.Logger) {
	if logger == nil {
		return
	}
	h.Network().Notify(&network.NotifyBundle{
		ConnectedF: func(_ network.Network, c network.Conn) {
			logger.Printf("peer connected %s (%s)", c.RemotePeer(), c.RemoteMultiaddr())
		},
		DisconnectedF: func(_ network.Network, c network.Conn) {
			logger.Printf("peer disconnected %s", c.RemotePeer())
		},
	})
}

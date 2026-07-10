package client

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	ma "github.com/multiformats/go-multiaddr"
)

const (
	supervisorInterval  = 10 * time.Second
	reconnectBackoffMax = 30 * time.Second
	dialRetryWindow     = 30 * time.Second
	dialRetryStart      = 500 * time.Millisecond
	dialRetryMax        = 4 * time.Second
)

// ensureConnected makes sure h has a live connection to the hub and to the host
// (over the relay circuit), refreshing the circuit addr so a re-dial always has
// an address to try. It is idempotent and cheap when already connected. A
// relayed (Limited) or direct (Connected) link to the host both count as up.
func ensureConnected(ctx context.Context, h host.Host, hubInfo peer.AddrInfo, hostID peer.ID, circuit ma.Multiaddr) error {
	if h.Network().Connectedness(hubInfo.ID) != network.Connected {
		if err := h.Connect(ctx, hubInfo); err != nil {
			return fmt.Errorf("connect hub: %w", err)
		}
	}
	h.Peerstore().AddAddr(hostID, circuit, peerstore.PermanentAddrTTL)
	switch h.Network().Connectedness(hostID) {
	case network.Connected, network.Limited:
		return nil
	default:
		if err := h.Connect(ctx, peer.AddrInfo{ID: hostID, Addrs: []ma.Multiaddr{circuit}}); err != nil {
			return fmt.Errorf("connect host: %w", err)
		}
		return nil
	}
}

// superviseConnection keeps h connected to the hub and host, reconnecting with
// capped exponential backoff, until ctx is done. It keeps the relay path warm
// so an idle tunnel does not lose reachability and a new connection has no cold
// start.
func superviseConnection(ctx context.Context, h host.Host, hubInfo peer.AddrInfo, hostID peer.ID, circuit ma.Multiaddr, logger *log.Logger) {
	backoff := time.Second
	for {
		if err := ensureConnected(ctx, h, hubInfo, hostID, circuit); err != nil {
			logf(logger, "supervisor: reconnect failed: %v", err)
			select {
			case <-ctx.Done():
				return
			case <-time.After(backoff):
			}
			if backoff < reconnectBackoffMax {
				backoff *= 2
			}
			continue
		}
		backoff = time.Second
		select {
		case <-ctx.Done():
			return
		case <-time.After(supervisorInterval):
		}
	}
}

// openStreamWithRetry (re)connects and opens a tunnel stream to the host,
// retrying with capped backoff until it succeeds or dialRetryWindow elapses, so
// a local connection that arrives during a transient outage waits for the path
// to heal instead of failing at once. Returns nil if the window elapses or ctx
// is cancelled. It retries only stream opening (a connectivity problem); the
// caller does the handshake once, since a handshake failure is an auth problem,
// not a transient one.
func openStreamWithRetry(ctx context.Context, h host.Host, hubInfo peer.AddrInfo, hostID peer.ID, circuit ma.Multiaddr, protocolID protocol.ID, logger *log.Logger) network.Stream {
	deadline := time.Now().Add(dialRetryWindow)
	backoff := dialRetryStart
	for {
		_ = ensureConnected(ctx, h, hubInfo, hostID, circuit)
		sctx := network.WithAllowLimitedConn(ctx, "goholesail")
		s, err := h.NewStream(sctx, hostID, protocolID)
		if err == nil {
			return s
		}
		if !time.Now().Before(deadline) {
			logf(logger, "stream open to %s failed after %s: %v", hostID, dialRetryWindow, err)
			return nil
		}
		select {
		case <-ctx.Done():
			return nil
		case <-time.After(backoff):
		}
		if backoff < dialRetryMax {
			backoff *= 2
		}
	}
}

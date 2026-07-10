// Package client implements the goholesail connect role: dial a host through
// the hub's relay circuit (best-effort DCUtR upgrade to a direct connection),
// then bind a local TCP listener whose connections are piped over tunnel streams.
package client

import (
	"context"
	"fmt"
	"net"

	"github.com/BenLocal/goholesail/internal/connstr"
	"github.com/BenLocal/goholesail/internal/identity"
	"github.com/BenLocal/goholesail/internal/tunnel"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Options configures a connect role.
type Options struct {
	ConnString string // ghs://... produced by the host
	LocalPort  int    // local TCP port to bind, e.g. 2222

	Secret string // secret for private hosts (used with the tunnel handshake)
}

// Run resolves the connection string, dials the host via the relay, and serves
// a local listener until ctx is cancelled. It returns the libp2p host and the
// bound listener (pass LocalPort 0 for an OS-assigned port, useful in tests).
func Run(ctx context.Context, opts Options) (host.Host, net.Listener, error) {
	cs, err := connstr.Decode(opts.ConnString)
	if err != nil {
		return nil, nil, fmt.Errorf("client: decode connstr: %w", err)
	}
	secret := cs.Secret
	if secret == "" {
		secret = opts.Secret
	}
	hostID, err := peer.Decode(cs.HostID)
	if err != nil {
		return nil, nil, fmt.Errorf("client: bad host id: %w", err)
	}
	hubInfo, err := peer.AddrInfoFromString(cs.Hub)
	if err != nil {
		return nil, nil, fmt.Errorf("client: parse hub addr: %w", err)
	}

	priv, err := identity.Random()
	if err != nil {
		return nil, nil, err
	}
	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		return nil, nil, fmt.Errorf("client: new: %w", err)
	}

	// Connect to the relay first so its transport address is in the peerstore,
	// then the short circuit address below is dialable.
	if err := h.Connect(ctx, *hubInfo); err != nil {
		_ = h.Close()
		return nil, nil, fmt.Errorf("client: connect hub: %w", err)
	}
	circuit, err := ma.NewMultiaddr("/p2p/" + hubInfo.ID.String() + "/p2p-circuit/p2p/" + hostID.String())
	if err != nil {
		_ = h.Close()
		return nil, nil, fmt.Errorf("client: build circuit addr: %w", err)
	}
	if err := h.Connect(ctx, peer.AddrInfo{ID: hostID, Addrs: []ma.Multiaddr{circuit}}); err != nil {
		_ = h.Close()
		return nil, nil, fmt.Errorf("client: connect host via relay: %w", err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opts.LocalPort))
	if err != nil {
		_ = h.Close()
		return nil, nil, fmt.Errorf("client: listen: %w", err)
	}

	// Close the listener when ctx is cancelled so the accept loop below exits,
	// honoring the "serves ... until ctx is cancelled" contract.
	go func() {
		<-ctx.Done()
		_ = ln.Close()
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return // listener closed
			}
			go func() {
				// Allow the stream over a limited (relay) connection; if DCUtR
				// has upgraded to a direct connection this is a harmless no-op.
				sctx := network.WithAllowLimitedConn(ctx, "goholesail")
				s, err := h.NewStream(sctx, hostID, tunnel.ProtocolID)
				if err != nil {
					_ = conn.Close()
					return
				}
				if err := tunnel.ClientHandshake(s, secret); err != nil {
					_ = s.Reset()
					_ = conn.Close()
					return
				}
				tunnel.Pump(conn, s)
			}()
		}
	}()

	return h, ln, nil
}

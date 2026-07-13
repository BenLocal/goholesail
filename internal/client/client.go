// Package client implements the goholesail connect role: dial a host through
// the hub's relay circuit (best-effort DCUtR upgrade to a direct connection),
// then bind a local TCP listener whose connections are piped over tunnel streams.
package client

import (
	"context"
	"fmt"
	"log"
	"net"
	"strings"

	"github.com/BenLocal/goholesail/internal/connstr"
	"github.com/BenLocal/goholesail/internal/identity"
	"github.com/BenLocal/goholesail/internal/registry"
	"github.com/BenLocal/goholesail/internal/swarm"
	"github.com/BenLocal/goholesail/internal/tunnel"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
	ma "github.com/multiformats/go-multiaddr"
)

// Options configures a connect role.
type Options struct {
	ConnString string // ghs://... OR a bare registry name
	LocalPort  int

	Secret string // secret for private hosts
	Hub    string // hub /p2p multiaddr; required when ConnString is a bare name

	SwarmKey string // shared swarm passphrase (pnet); empty => no private network

	Logger *log.Logger // nil => silent; the CLI injects a [connect] logger
}

// Run resolves the connection string, dials the host via the relay, and serves
// a local listener until ctx is cancelled. It returns the libp2p host and the
// bound listener (pass LocalPort 0 for an OS-assigned port, useful in tests).
func Run(ctx context.Context, opts Options) (host.Host, net.Listener, error) {
	priv, err := identity.Random()
	if err != nil {
		return nil, nil, err
	}
	libp2pOpts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.EnableHolePunching(),
	}
	libp2pOpts = append(libp2pOpts, swarm.Options(opts.SwarmKey)...)
	h, err := libp2p.New(libp2pOpts...)
	if err != nil {
		return nil, nil, fmt.Errorf("client: new: %w", err)
	}
	attachConnLogger(h, opts.Logger)

	// A bare name is resolved over the hub's registry, which needs h connected
	// to the hub first; a ghs:// string is decoded locally.
	cs, hubInfo, err := resolveConnString(ctx, h, opts)
	if err != nil {
		_ = h.Close()
		return nil, nil, err
	}
	secret := cs.Secret
	if secret == "" {
		secret = opts.Secret
	}
	hostID, err := peer.Decode(cs.HostID)
	if err != nil {
		_ = h.Close()
		return nil, nil, fmt.Errorf("client: bad host id: %w", err)
	}
	circuit, err := ma.NewMultiaddr("/p2p/" + hubInfo.ID.String() + "/p2p-circuit/p2p/" + hostID.String())
	if err != nil {
		_ = h.Close()
		return nil, nil, fmt.Errorf("client: build circuit addr: %w", err)
	}

	ln, err := net.Listen("tcp", fmt.Sprintf("127.0.0.1:%d", opts.LocalPort))
	if err != nil {
		_ = h.Close()
		return nil, nil, fmt.Errorf("client: listen: %w", err)
	}

	// Keep hub+host connectivity warm in the background instead of connecting
	// synchronously, so connect can start before the host (and, for a ghs://
	// target, the hub) is up, and so a dropped link is re-established.
	go superviseConnection(ctx, h, *hubInfo, hostID, circuit, opts.Logger)

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
				// Hold this connection through a transient outage: re-dial with
				// bounded backoff (addr refresh happens inside ensureConnected).
				s := openStreamWithRetry(ctx, h, *hubInfo, hostID, circuit, tunnel.ProtocolID, opts.Logger)
				if s == nil {
					_ = conn.Close()
					return
				}
				logf(opts.Logger, "stream opened to %s", hostID)
				if err := tunnel.ClientHandshake(s, secret); err != nil {
					logf(opts.Logger, "handshake failed: %v", err)
					_ = s.Reset()
					_ = conn.Close()
					return
				}
				logf(opts.Logger, "handshake ok")
				tunnel.Pump(conn, s)
				logf(opts.Logger, "local conn closed for %s", hostID)
			}()
		}
	}()

	return h, ln, nil
}

// resolveConnString turns Options.ConnString into a ConnString plus the hub's
// AddrInfo: either by decoding a ghs:// string directly, or by resolving a bare
// name against the hub's registry (over h) and combining it with --hub/--secret.
func resolveConnString(ctx context.Context, h host.Host, opts Options) (connstr.ConnString, *peer.AddrInfo, error) {
	if strings.HasPrefix(opts.ConnString, "ghs://") {
		cs, err := connstr.Decode(opts.ConnString)
		if err != nil {
			return connstr.ConnString{}, nil, fmt.Errorf("client: decode connstr: %w", err)
		}
		hubInfo, err := peer.AddrInfoFromString(cs.Hub)
		if err != nil {
			return connstr.ConnString{}, nil, fmt.Errorf("client: parse hub addr: %w", err)
		}
		return cs, hubInfo, nil
	}
	if opts.Hub == "" {
		return connstr.ConnString{}, nil, fmt.Errorf("client: %q is not a ghs:// string; pass --hub to resolve it as a name", opts.ConnString)
	}
	hubInfo, err := peer.AddrInfoFromString(opts.Hub)
	if err != nil {
		return connstr.ConnString{}, nil, fmt.Errorf("client: parse hub addr: %w", err)
	}
	if err := h.Connect(ctx, *hubInfo); err != nil {
		return connstr.ConnString{}, nil, fmt.Errorf("client: connect hub: %w", err)
	}
	svc, err := registry.NewClient(h, hubInfo.ID).Resolve(ctx, opts.ConnString)
	if err != nil {
		return connstr.ConnString{}, nil, err
	}
	if svc.Private && opts.Secret == "" {
		return connstr.ConnString{}, nil, fmt.Errorf("client: service %q is private; pass --secret", svc.Name)
	}
	return connstr.ConnString{
		Version: 1,
		Private: svc.Private,
		HostID:  svc.PeerID,
		Hub:     opts.Hub,
		Secret:  opts.Secret,
	}, hubInfo, nil
}

// logf logs when a logger was provided, else is a no-op.
func logf(l *log.Logger, format string, args ...any) {
	if l != nil {
		l.Printf(format, args...)
	}
}

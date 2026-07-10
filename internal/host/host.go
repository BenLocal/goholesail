// Package host implements the goholesail host role: expose a local TCP port to
// clients by reserving a relay slot on the hub and serving a stream protocol
// that pipes each incoming stream to the local port.
package host

import (
	"context"
	"fmt"
	"net"

	"github.com/BenLocal/goholesail/internal/connstr"
	"github.com/BenLocal/goholesail/internal/identity"
	"github.com/BenLocal/goholesail/internal/tunnel"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/crypto"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
)

// Options configures a host role.
type Options struct {
	Seed      string // stable identity seed; empty => random ephemeral key
	LocalPort int    // local TCP port to expose, e.g. 22
	HubAddr   string // hub /p2p multiaddr, e.g. /ip4/1.2.3.4/tcp/4001/p2p/<hubID>
}

// Run starts the host: builds identity, connects to the hub, reserves a relay
// slot, and serves tunnel streams. It returns the libp2p host and the ghs://
// connection string a client should use. The caller owns closing the host.
func Run(ctx context.Context, opts Options) (host.Host, string, error) {
	priv, err := keyFor(opts.Seed)
	if err != nil {
		return nil, "", err
	}
	h, err := libp2p.New(
		libp2p.Identity(priv),
		libp2p.EnableHolePunching(),
	)
	if err != nil {
		return nil, "", fmt.Errorf("host: new: %w", err)
	}

	hubInfo, err := peer.AddrInfoFromString(opts.HubAddr)
	if err != nil {
		_ = h.Close()
		return nil, "", fmt.Errorf("host: parse hub addr: %w", err)
	}
	if err := h.Connect(ctx, *hubInfo); err != nil {
		_ = h.Close()
		return nil, "", fmt.Errorf("host: connect hub: %w", err)
	}
	if _, err := relayclient.Reserve(ctx, h, *hubInfo); err != nil {
		_ = h.Close()
		return nil, "", fmt.Errorf("host: reserve relay slot: %w", err)
	}

	local := fmt.Sprintf("127.0.0.1:%d", opts.LocalPort)
	h.SetStreamHandler(tunnel.ProtocolID, func(s network.Stream) {
		conn, err := net.Dial("tcp", local)
		if err != nil {
			_ = s.Reset()
			return
		}
		tunnel.Pump(s, conn)
	})

	cs := connstr.ConnString{
		Version: 1,
		Private: false, // M2 is public mode; private/token lands in a later milestone
		HostID:  h.ID().String(),
		Hub:     opts.HubAddr,
	}
	str, err := connstr.Encode(cs)
	if err != nil {
		_ = h.Close()
		return nil, "", fmt.Errorf("host: encode connstr: %w", err)
	}
	return h, str, nil
}

// keyFor returns a deterministic key when seed is set, else a random one.
func keyFor(seed string) (crypto.PrivKey, error) {
	if seed != "" {
		return identity.FromSeed(seed)
	}
	return identity.Random()
}

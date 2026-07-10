// Package host implements the goholesail host role: expose a local TCP port to
// clients by reserving a relay slot on the hub and serving a stream protocol
// that pipes each incoming stream to the local port.
package host

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"net"
	"time"

	"github.com/BenLocal/goholesail/internal/connstr"
	"github.com/BenLocal/goholesail/internal/identity"
	"github.com/BenLocal/goholesail/internal/registry"
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
	HubAddr   string // hub /p2p multiaddr

	Private bool   // require an HMAC token from clients
	Secret  string // shared secret; if Private and empty, one is generated

	Name     string   // registry name; empty => no auto-register
	Registry string   // registry ws url, e.g. ws://hub:8080/reg
	Tags     []string // registry tags
}

// Run starts the host: builds identity, connects to the hub, reserves a relay
// slot, and serves tunnel streams. It returns the libp2p host and the ghs://
// connection string a client should use. The caller owns closing the host.
//
// Teardown contract: when Name+Registry are set, the registry lifecycle (renew
// loop + deregister) is bound to ctx, not to the returned host. Cancel ctx to
// tear a registered host down cleanly; closing the host alone leaves the renew
// goroutine running and the directory entry live. The CLI honors this by
// pairing signal.NotifyContext's cancel with h.Close().
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

	secret := ""
	if opts.Private {
		secret = opts.Secret
		if secret == "" {
			secret = randomSecret()
		}
	}

	local := fmt.Sprintf("127.0.0.1:%d", opts.LocalPort)
	h.SetStreamHandler(tunnel.ProtocolID, func(s network.Stream) {
		_ = s.SetReadDeadline(time.Now().Add(10 * time.Second))
		if err := tunnel.ServerHandshake(s, secret); err != nil {
			_ = s.Reset()
			return
		}
		_ = s.SetReadDeadline(time.Time{})
		conn, err := net.Dial("tcp", local)
		if err != nil {
			_ = s.Reset()
			return
		}
		tunnel.Pump(s, conn)
	})

	cs := connstr.ConnString{
		Version: 1,
		Private: opts.Private,
		HostID:  h.ID().String(),
		Hub:     opts.HubAddr,
		Secret:  secret,
	}
	str, err := connstr.Encode(cs)
	if err != nil {
		_ = h.Close()
		return nil, "", fmt.Errorf("host: encode connstr: %w", err)
	}
	if opts.Name != "" && opts.Registry != "" {
		rc, err := registry.Dial(ctx, opts.Registry)
		if err != nil {
			_ = h.Close()
			return nil, "", fmt.Errorf("host: dial registry: %w", err)
		}
		svc := registry.Service{
			Name:    opts.Name,
			PeerID:  h.ID().String(),
			Hub:     opts.HubAddr,
			Private: opts.Private,
			Tags:    opts.Tags,
		}
		if err := rc.Register(svc, 90*time.Second); err != nil {
			_ = rc.Close()
			_ = h.Close()
			return nil, "", fmt.Errorf("host: register: %w", err)
		}
		// Lifetime is bound to ctx (see Run's teardown contract). On cancel we
		// make a best-effort deregister — an unbounded ws round-trip with no
		// deadline, so an unreachable registry at shutdown leaves the entry to
		// expire by TTL rather than being cleanly removed. Acceptable for M3
		// (resilience is a non-goal); a bounded shutdown is M4 work.
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					_ = rc.Deregister(opts.Name)
					_ = rc.Close()
					return
				case <-t.C:
					_ = rc.Renew(opts.Name, 90*time.Second)
				}
			}
		}()
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

// randomSecret returns a fresh 32-byte secret, base64url-encoded.
func randomSecret() string {
	var b [32]byte
	_, _ = rand.Read(b[:])
	return base64.RawURLEncoding.EncodeToString(b[:])
}

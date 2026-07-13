// Package host implements the goholesail host role: expose a local TCP port to
// clients by reserving a relay slot on the hub and serving a stream protocol
// that pipes each incoming stream to the local port.
package host

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"log"
	"net"
	"time"

	"github.com/BenLocal/goholesail/internal/connstr"
	"github.com/BenLocal/goholesail/internal/identity"
	"github.com/BenLocal/goholesail/internal/registry"
	"github.com/BenLocal/goholesail/internal/swarm"
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

	SwarmKey string // shared swarm passphrase (pnet); empty => no private network

	Name string   // registry name; empty => no auto-register (registers to --hub)
	Tags []string // registry tags

	Logger *log.Logger // nil => silent; the CLI injects a [host] logger
}

// Run starts the host: builds identity, connects to the hub, reserves a relay
// slot, and serves tunnel streams. It returns the libp2p host and the ghs://
// connection string a client should use. The caller owns closing the host.
//
// Teardown contract: when Name is set the host auto-registers with the hub's
// registry, and that lifecycle (renew loop + deregister) is bound to ctx, not to
// the returned host. Cancel ctx to tear a registered host down cleanly; closing
// the host alone leaves the renew goroutine running and the directory entry
// live. The CLI honors this by pairing signal.NotifyContext's cancel with
// h.Close().
func Run(ctx context.Context, opts Options) (host.Host, string, error) {
	priv, err := keyFor(opts.Seed)
	if err != nil {
		return nil, "", err
	}
	libp2pOpts := []libp2p.Option{
		libp2p.Identity(priv),
		libp2p.EnableHolePunching(),
	}
	libp2pOpts = append(libp2pOpts, swarm.Options(opts.SwarmKey)...)
	h, err := libp2p.New(libp2pOpts...)
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
	res, err := relayclient.Reserve(ctx, h, *hubInfo)
	if err != nil {
		_ = h.Close()
		return nil, "", fmt.Errorf("host: reserve relay slot: %w", err)
	}
	logReservation(opts.Logger, res)

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
	if opts.Name != "" {
		// The registry lives on the hub itself; reuse the connection already
		// established above (Connect + Reserve). No secret is ever sent.
		rc := registry.NewClient(h, hubInfo.ID)
		svc := registry.Service{
			Name:    opts.Name,
			PeerID:  h.ID().String(),
			Hub:     opts.HubAddr,
			Private: opts.Private,
			Tags:    opts.Tags,
		}
		if err := rc.Register(ctx, svc, 90*time.Second); err != nil {
			_ = h.Close()
			return nil, "", fmt.Errorf("host: register: %w", err)
		}
		// Lifetime is bound to ctx (see Run's teardown contract). On cancel we
		// make a best-effort deregister; if the hub is unreachable at shutdown
		// the entry lingers until its TTL expires. A bounded shutdown is M4 work.
		go func() {
			t := time.NewTicker(30 * time.Second)
			defer t.Stop()
			for {
				select {
				case <-ctx.Done():
					_ = rc.Deregister(context.Background(), opts.Name)
					return
				case <-t.C:
					_ = rc.Renew(ctx, opts.Name, 90*time.Second)
				}
			}
		}()
	}
	// Keep the reservation alive: it expires after the relay's ReservationTTL
	// (default 1h) and is lost if the host<->hub connection drops, after which
	// the relay can no longer forward circuits to this host. Launched only after
	// every fallible step above, so a Run that errors out (and closes h) does not
	// leak this goroutine. Bound to ctx, like the registry renew loop above.
	go maintainReservation(ctx, h, *hubInfo, res, opts.Logger)
	return h, str, nil
}

// reservationRenewWait returns how long to wait before renewing a relay
// reservation that expires at exp. It renews at 3/4 of the remaining lifetime
// (so a 1h reservation renews ~15m early and a 2s test reservation renews at
// 1.5s), with a 1s floor so an already-expired reservation renews promptly
// instead of spinning.
func reservationRenewWait(exp, now time.Time) time.Duration {
	wait := exp.Sub(now) * 3 / 4
	if wait < time.Second {
		wait = time.Second
	}
	return wait
}

// logf logs when a logger was provided, else is a no-op.
func logf(l *log.Logger, format string, args ...any) {
	if l != nil {
		l.Printf(format, args...)
	}
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

// logReservation records a granted relay reservation and the limits the relay
// attached to it (0 duration/data means the relay imposes no per-connection cap).
func logReservation(l *log.Logger, res *relayclient.Reservation) {
	logf(l, "relay reservation ok, expires %s (limit dur=%s data=%d)",
		res.Expiration.Format(time.RFC3339), res.LimitDuration, res.LimitData)
}

// hostLivenessInterval bounds how long a dropped host<->hub link (which drops
// the relay reservation with it) can go unnoticed before the host reconnects
// and re-reserves.
const hostLivenessInterval = 5 * time.Second

// maintainReservation keeps the relay reservation alive until ctx is cancelled.
// It re-reserves when the reservation is due for renewal (Stage 1 behavior) OR
// when the host<->hub connection has dropped (which loses the reservation and
// makes the host unreachable until it reconnects and reserves again). It wakes
// at least every hostLivenessInterval so a drop is caught promptly.
func maintainReservation(ctx context.Context, h host.Host, hubInfo peer.AddrInfo, res *relayclient.Reservation, logger *log.Logger) {
	renewAt := time.Now().Add(reservationRenewWait(res.Expiration, time.Now()))
	for {
		wait := time.Until(renewAt)
		if wait > hostLivenessInterval {
			wait = hostLivenessInterval
		}
		if wait < 0 {
			wait = 0
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(wait):
		}
		dueForRenewal := !time.Now().Before(renewAt)
		hubDown := h.Network().Connectedness(hubInfo.ID) != network.Connected
		if dueForRenewal || hubDown {
			next, ok := reserveWithBackoff(ctx, h, hubInfo, logger)
			if !ok {
				return // ctx cancelled
			}
			res = next
			renewAt = time.Now().Add(reservationRenewWait(res.Expiration, time.Now()))
		}
	}
}

// reserveWithBackoff reconnects to the hub if needed and reserves a relay slot,
// retrying with capped exponential backoff until it succeeds or ctx is done.
func reserveWithBackoff(ctx context.Context, h host.Host, hubInfo peer.AddrInfo, logger *log.Logger) (*relayclient.Reservation, bool) {
	backoff := time.Second
	for {
		if err := h.Connect(ctx, hubInfo); err != nil {
			logf(logger, "relay reservation: reconnect hub failed: %v", err)
		} else if res, err := relayclient.Reserve(ctx, h, hubInfo); err != nil {
			logf(logger, "relay reservation: reserve failed: %v", err)
		} else {
			logReservation(logger, res)
			return res, true
		}
		select {
		case <-ctx.Done():
			return nil, false
		case <-time.After(backoff):
		}
		if backoff < 30*time.Second {
			backoff *= 2
		}
	}
}

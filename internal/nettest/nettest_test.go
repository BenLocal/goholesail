package nettest

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"log"
	"net"
	"strings"
	"testing"
	"time"

	gclient "github.com/BenLocal/goholesail/internal/client"
	"github.com/BenLocal/goholesail/internal/connstr"
	ghost "github.com/BenLocal/goholesail/internal/host"
	ghub "github.com/BenLocal/goholesail/internal/hub"
	"github.com/BenLocal/goholesail/internal/registry"
	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/network"
	"github.com/libp2p/go-libp2p/core/peer"
	"github.com/libp2p/go-libp2p/core/peerstore"
	"github.com/libp2p/go-libp2p/core/protocol"
	relayclient "github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/client"
	"github.com/libp2p/go-libp2p/p2p/protocol/circuitv2/relay"
	ma "github.com/multiformats/go-multiaddr"
)

// startEcho starts a TCP echo server and returns its port.
func startEcho(t *testing.T) int {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("echo listen: %v", err)
	}
	t.Cleanup(func() { _ = ln.Close() })
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer c.Close()
				sc := bufio.NewScanner(c)
				for sc.Scan() {
					fmt.Fprintf(c, "echo:%s\n", sc.Text())
				}
			}()
		}
	}()
	return ln.Addr().(*net.TCPAddr).Port
}

func TestEndToEndTCPTunnel(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	echoPort := startEcho(t)

	// Hub on a random local port.
	h, err := ghub.New("/ip4/127.0.0.1/tcp/0", "")
	if err != nil {
		t.Fatalf("hub: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	hubAddrs := ghub.P2pAddrs(h)
	if len(hubAddrs) == 0 {
		t.Fatal("hub has no dialable addrs")
	}
	hubAddr := hubAddrs[0]

	// Host exposes the echo server's port.
	hostH, cs, err := ghost.Run(ctx, ghost.Options{Seed: "test-seed", LocalPort: echoPort, HubAddr: hubAddr})
	if err != nil {
		t.Fatalf("host run: %v", err)
	}
	t.Cleanup(func() { _ = hostH.Close() })

	// Client binds a random local port.
	clientH, ln, err := gclient.Run(ctx, gclient.Options{ConnString: cs, LocalPort: 0})
	if err != nil {
		t.Fatalf("client run: %v", err)
	}
	t.Cleanup(func() { _ = clientH.Close(); _ = ln.Close() })

	// Talk to the local listener; bytes should reach the echo server and back.
	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(conn, "hello\n")
	got, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read echoed: %v", err)
	}
	if got != "echo:hello\n" {
		t.Fatalf("got %q, want %q", got, "echo:hello\n")
	}
}

func TestPrivateTunnelRightSecret(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	echoPort := startEcho(t)

	h, err := ghub.New("/ip4/127.0.0.1/tcp/0", "")
	if err != nil {
		t.Fatalf("hub: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	hubAddr := ghub.P2pAddrs(h)[0]

	hostH, cs, err := ghost.Run(ctx, ghost.Options{
		Seed: "priv-seed", LocalPort: echoPort, HubAddr: hubAddr,
		Private: true, Secret: "the-secret",
	})
	if err != nil {
		t.Fatalf("host run: %v", err)
	}
	t.Cleanup(func() { _ = hostH.Close() })

	// The printed connstr carries the secret; the client uses it directly.
	clientH, ln, err := gclient.Run(ctx, gclient.Options{ConnString: cs, LocalPort: 0})
	if err != nil {
		t.Fatalf("client run: %v", err)
	}
	t.Cleanup(func() { _ = clientH.Close(); _ = ln.Close() })

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(conn, "hi\n")
	got, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read echoed: %v", err)
	}
	if got != "echo:hi\n" {
		t.Fatalf("got %q, want %q", got, "echo:hi\n")
	}
}

func TestPrivateTunnelWrongSecretRejected(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	echoPort := startEcho(t)

	h, err := ghub.New("/ip4/127.0.0.1/tcp/0", "")
	if err != nil {
		t.Fatalf("hub: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	hubAddr := ghub.P2pAddrs(h)[0]

	hostH, cs, err := ghost.Run(ctx, ghost.Options{
		Seed: "priv-seed2", LocalPort: echoPort, HubAddr: hubAddr,
		Private: true, Secret: "correct-secret",
	})
	if err != nil {
		t.Fatalf("host run: %v", err)
	}
	t.Cleanup(func() { _ = hostH.Close() })

	// Re-encode the connstr with a wrong secret; the client uses the connstr's
	// own secret first, so this drives a failing handshake at the host.
	clientH, ln, err := gclient.Run(ctx, gclient.Options{ConnString: mustReSecret(t, cs, "wrong-secret"), LocalPort: 0})
	if err != nil {
		t.Fatalf("client run: %v", err)
	}
	t.Cleanup(func() { _ = clientH.Close(); _ = ln.Close() })

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer conn.Close()
	_ = conn.SetReadDeadline(time.Now().Add(3 * time.Second))
	fmt.Fprintf(conn, "hi\n")
	// Handshake fails on the host, which resets the stream; the client closes
	// the local conn, so the read returns an error (EOF/reset), not echoed data.
	buf := make([]byte, 16)
	if n, err := conn.Read(buf); err == nil && n > 0 && string(buf[:n]) == "echo:hi\n" {
		t.Fatalf("wrong secret should not have tunneled data, got %q", buf[:n])
	}
}

// mustReSecret decodes a connstr, replaces its Secret, and re-encodes it.
func mustReSecret(t *testing.T, s, newSecret string) string {
	t.Helper()
	cs, err := connstr.Decode(s)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	cs.Secret = newSecret
	out, err := connstr.Encode(cs)
	if err != nil {
		t.Fatalf("encode: %v", err)
	}
	return out
}

func TestRegistryNameResolutionPrivate(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	echoPort := startEcho(t)

	// Hub with the registry protocol mounted (as the hub binary does).
	h, err := ghub.New("/ip4/127.0.0.1/tcp/0", "")
	if err != nil {
		t.Fatalf("hub: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	h.SetStreamHandler(registry.RegistryProtocolID, registry.NewServer(registry.NewStore()).HandleStream)
	hubAddr := ghub.P2pAddrs(h)[0]

	// Host: private + auto-register by name to the hub (secret NOT sent).
	hostH, _, err := ghost.Run(ctx, ghost.Options{
		Seed: "reg-seed", LocalPort: echoPort, HubAddr: hubAddr,
		Private: true, Secret: "reg-secret",
		Name: "home-ssh", Tags: []string{"ssh"},
	})
	if err != nil {
		t.Fatalf("host run: %v", err)
	}
	t.Cleanup(func() { _ = hostH.Close() })

	// Client: resolve by name via --hub + supply the secret out-of-band.
	clientH, ln, err := gclient.Run(ctx, gclient.Options{
		ConnString: "home-ssh", Hub: hubAddr, Secret: "reg-secret", LocalPort: 0,
	})
	if err != nil {
		t.Fatalf("client run: %v", err)
	}
	t.Cleanup(func() { _ = clientH.Close(); _ = ln.Close() })

	conn, err := net.DialTimeout("tcp", ln.Addr().String(), 5*time.Second)
	if err != nil {
		t.Fatalf("dial local: %v", err)
	}
	defer conn.Close()
	_ = conn.SetDeadline(time.Now().Add(10 * time.Second))
	fmt.Fprintf(conn, "yo\n")
	got, err := bufio.NewReader(conn).ReadString('\n')
	if err != nil {
		t.Fatalf("read echoed: %v", err)
	}
	if got != "echo:yo\n" {
		t.Fatalf("got %q, want %q", got, "echo:yo\n")
	}
}

func TestRegistryPrivateWithoutSecretFails(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	echoPort := startEcho(t)

	h, err := ghub.New("/ip4/127.0.0.1/tcp/0", "")
	if err != nil {
		t.Fatalf("hub: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	h.SetStreamHandler(registry.RegistryProtocolID, registry.NewServer(registry.NewStore()).HandleStream)
	hubAddr := ghub.P2pAddrs(h)[0]

	hostH, _, err := ghost.Run(ctx, ghost.Options{
		Seed: "reg-seed2", LocalPort: echoPort, HubAddr: hubAddr,
		Private: true, Secret: "s", Name: "svc",
	})
	if err != nil {
		t.Fatalf("host run: %v", err)
	}
	t.Cleanup(func() { _ = hostH.Close() })

	// Resolve a private service without --secret -> client.Run must error.
	if _, _, err := gclient.Run(ctx, gclient.Options{ConnString: "svc", Hub: hubAddr, LocalPort: 0}); err == nil {
		t.Fatal("connecting to a private service without --secret should fail")
	}
}

// isRelayAddr reports whether a multiaddr is a circuit-relay address (ends in
// /p2p-circuit[/p2p/<id>]), i.e. the connection actually rode a relay rather
// than a direct transport.
func isRelayAddr(a ma.Multiaddr) bool {
	_, err := a.ValueForProtocol(ma.P_CIRCUIT)
	return err == nil
}

// TestRelayNoDataLimit forces a tunnel onto the relayed path (neither peer
// enables hole punching, and the client only knows the server via the circuit
// addr) and pushes 256 KB through it. go-libp2p's default relay limit resets a
// relayed connection at 128 KB, so this fails unless the hub lifts the limit
// with relay.WithInfiniteLimits().
func TestRelayNoDataLimit(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	hubH, err := ghub.New("/ip4/127.0.0.1/tcp/0", "")
	if err != nil {
		t.Fatalf("hub: %v", err)
	}
	t.Cleanup(func() { _ = hubH.Close() })
	hubInfo := peer.AddrInfo{ID: hubH.ID(), Addrs: hubH.Addrs()}

	// Server peer: no hole punching (never upgrades off the relay); reserves a
	// slot and drains whatever arrives.
	const drainProto = protocol.ID("/goholesail-test/drain/1.0.0")
	srv, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("srv: %v", err)
	}
	t.Cleanup(func() { _ = srv.Close() })
	if err := srv.Connect(ctx, hubInfo); err != nil {
		t.Fatalf("srv connect hub: %v", err)
	}
	if _, err := relayclient.Reserve(ctx, srv, hubInfo); err != nil {
		t.Fatalf("srv reserve: %v", err)
	}
	drained := make(chan int64, 1)
	srv.SetStreamHandler(drainProto, func(s network.Stream) {
		defer s.Close()
		n, _ := io.Copy(io.Discard, s)
		drained <- n
	})

	// Client peer: no hole punching; only knows srv via the circuit addr.
	cli, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("cli: %v", err)
	}
	t.Cleanup(func() { _ = cli.Close() })
	if err := cli.Connect(ctx, hubInfo); err != nil {
		t.Fatalf("cli connect hub: %v", err)
	}
	circuit, err := ma.NewMultiaddr("/p2p/" + hubInfo.ID.String() + "/p2p-circuit/p2p/" + srv.ID().String())
	if err != nil {
		t.Fatalf("circuit addr: %v", err)
	}
	cli.Peerstore().AddAddr(srv.ID(), circuit, peerstore.PermanentAddrTTL)

	sctx := network.WithAllowLimitedConn(ctx, "test")
	s, err := cli.NewStream(sctx, srv.ID(), drainProto)
	if err != nil {
		t.Fatalf("open stream via relay: %v", err)
	}
	// Confirm the stream actually rode the circuit rather than a direct
	// connection. We can't assert on Stat().Limited here: the hub's relay runs
	// with WithInfiniteLimits(), which by design makes the relay omit the Limit
	// field from the handshake, so both sides report Limited=false for every
	// relayed conn once the fix is in place (see relay.makeLimitMsg). The
	// remote multiaddr ending in /p2p-circuit is what's actually reliable.
	if !isRelayAddr(s.Conn().RemoteMultiaddr()) {
		t.Fatalf("expected a relayed conn (remote addr ending /p2p-circuit); got %s, so the test is not exercising the relay path", s.Conn().RemoteMultiaddr())
	}

	payload := make([]byte, 256*1024) // 256 KB > the 128 KB default cap
	if _, err := s.Write(payload); err != nil {
		t.Fatalf("write over relay: %v", err)
	}
	if err := s.CloseWrite(); err != nil {
		t.Fatalf("close write: %v", err)
	}
	select {
	case n := <-drained:
		if n < int64(len(payload)) {
			t.Fatalf("relay truncated the stream at %d of %d bytes (data limit not lifted)", n, len(payload))
		}
	case <-ctx.Done():
		t.Fatalf("timed out waiting for %d bytes to drain: %v", len(payload), ctx.Err())
	}
}

// shortTTLRelayHub builds a hub whose relay grants short-lived reservations, so
// a test can watch the host renew them. Limits are left infinite so the relay
// data cap is not the variable under test.
func shortTTLRelayHub(t *testing.T, ttl time.Duration) host.Host {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("relay hub: %v", err)
	}
	rc := relay.DefaultResources()
	rc.Limit = nil
	rc.ReservationTTL = ttl
	if _, err := relay.New(h, relay.WithResources(rc)); err != nil {
		_ = h.Close()
		t.Fatalf("relay: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	return h
}

func TestHostRenewsReservation(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	echoPort := startEcho(t)

	hubH := shortTTLRelayHub(t, 2*time.Second)
	hubAddr := ghub.P2pAddrs(hubH)[0]

	var sb syncBuf
	hostH, _, err := ghost.Run(ctx, ghost.Options{
		Seed: "renew-seed", LocalPort: echoPort, HubAddr: hubAddr,
		Logger: log.New(&sb, "[host] ", 0),
	})
	if err != nil {
		t.Fatalf("host run: %v", err)
	}
	t.Cleanup(func() { _ = hostH.Close() })

	// With a 2s TTL the host renews at ~1.5s; within ~5s expect the initial
	// reservation plus at least one renewal.
	deadline := time.Now().Add(6 * time.Second)
	for time.Now().Before(deadline) {
		if strings.Count(sb.String(), "relay reservation ok") >= 2 {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("expected >=2 relay reservations, got log:\n%s", sb.String())
}

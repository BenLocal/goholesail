package nettest

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	gclient "github.com/BenLocal/goholesail/internal/client"
	"github.com/BenLocal/goholesail/internal/connstr"
	ghost "github.com/BenLocal/goholesail/internal/host"
	ghub "github.com/BenLocal/goholesail/internal/hub"
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
	h, err := ghub.New("/ip4/127.0.0.1/tcp/0")
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

	h, err := ghub.New("/ip4/127.0.0.1/tcp/0")
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

	h, err := ghub.New("/ip4/127.0.0.1/tcp/0")
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

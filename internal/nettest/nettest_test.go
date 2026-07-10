package nettest

import (
	"bufio"
	"context"
	"fmt"
	"net"
	"testing"
	"time"

	gclient "github.com/BenLocal/goholesail/internal/client"
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

package registry

import (
	"context"
	"testing"
	"time"

	"github.com/libp2p/go-libp2p"
	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// newHubHost starts a libp2p host serving the registry protocol and returns it
// plus its dialable AddrInfo.
func newHubHost(t *testing.T) (host.Host, peer.AddrInfo) {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("hub host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	h.SetStreamHandler(RegistryProtocolID, NewServer(NewStore()).HandleStream)
	return h, peer.AddrInfo{ID: h.ID(), Addrs: h.Addrs()}
}

// newCallerHost starts a libp2p host already connected to hub.
func newCallerHost(t *testing.T, hub peer.AddrInfo) host.Host {
	t.Helper()
	h, err := libp2p.New(libp2p.ListenAddrStrings("/ip4/127.0.0.1/tcp/0"))
	if err != nil {
		t.Fatalf("caller host: %v", err)
	}
	t.Cleanup(func() { _ = h.Close() })
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := h.Connect(ctx, hub); err != nil {
		t.Fatalf("connect hub: %v", err)
	}
	return h
}

func TestRegisterResolveRoundTrip(t *testing.T) {
	_, hubInfo := newHubHost(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	rc := NewClient(newCallerHost(t, hubInfo), hubInfo.ID)
	svc := Service{Name: "home-ssh", PeerID: "12D3KooWabc", Hub: "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWhub", Private: true, Tags: []string{"ssh"}}
	if err := rc.Register(ctx, svc, 90*time.Second); err != nil {
		t.Fatalf("register: %v", err)
	}

	cc := NewClient(newCallerHost(t, hubInfo), hubInfo.ID)
	got, err := cc.Resolve(ctx, "home-ssh")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.PeerID != svc.PeerID || got.Hub != svc.Hub || !got.Private {
		t.Fatalf("resolved %+v, want %+v", got, svc)
	}

	if _, err := cc.Resolve(ctx, "nope"); err == nil {
		t.Fatal("resolve of unknown name should error")
	}

	if err := rc.Deregister(ctx, "home-ssh"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if _, err := cc.Resolve(ctx, "home-ssh"); err == nil {
		t.Fatal("resolve after deregister should error")
	}
}

func TestListReturnsRegistered(t *testing.T) {
	_, hubInfo := newHubHost(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	rc := NewClient(newCallerHost(t, hubInfo), hubInfo.ID)
	if err := rc.Register(ctx, Service{Name: "a", PeerID: "p", Tags: []string{"x"}}, time.Minute); err != nil {
		t.Fatalf("register a: %v", err)
	}
	if err := rc.Register(ctx, Service{Name: "b", PeerID: "q"}, time.Minute); err != nil {
		t.Fatalf("register b: %v", err)
	}

	all, err := rc.List(ctx, "")
	if err != nil {
		t.Fatalf("list all: %v", err)
	}
	if len(all) != 2 {
		t.Fatalf("list all = %d, want 2", len(all))
	}

	tagged, err := rc.List(ctx, "x")
	if err != nil {
		t.Fatalf("list tag: %v", err)
	}
	if len(tagged) != 1 || tagged[0].Name != "a" {
		t.Fatalf("list tag=x = %+v, want [a]", tagged)
	}
}

package registry

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func startServer(t *testing.T) string {
	t.Helper()
	ts := httptest.NewServer(NewServer(NewStore()))
	t.Cleanup(ts.Close)
	// httptest gives http://127.0.0.1:port ; ws dial needs ws://.../reg
	return "ws" + strings.TrimPrefix(ts.URL, "http") + "/reg"
}

func TestRegisterResolveRoundTrip(t *testing.T) {
	url := startServer(t)
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	host, err := Dial(ctx, url)
	if err != nil {
		t.Fatalf("host dial: %v", err)
	}
	defer host.Close()
	svc := Service{Name: "home-ssh", PeerID: "12D3KooWabc", Hub: "/ip4/1.2.3.4/tcp/4001/p2p/12D3KooWhub", Private: true, Tags: []string{"ssh"}}
	if err := host.Register(svc, 90*time.Second); err != nil {
		t.Fatalf("register: %v", err)
	}

	client, err := Dial(ctx, url)
	if err != nil {
		t.Fatalf("client dial: %v", err)
	}
	defer client.Close()
	got, err := client.Resolve("home-ssh")
	if err != nil {
		t.Fatalf("resolve: %v", err)
	}
	if got.PeerID != svc.PeerID || got.Hub != svc.Hub || !got.Private {
		t.Fatalf("resolved %+v, want %+v", got, svc)
	}

	if _, err := client.Resolve("nope"); err == nil {
		t.Fatal("resolve of unknown name should error")
	}

	if err := host.Deregister("home-ssh"); err != nil {
		t.Fatalf("deregister: %v", err)
	}
	if _, err := client.Resolve("home-ssh"); err == nil {
		t.Fatal("resolve after deregister should error")
	}
}

func TestServicesHTTPListing(t *testing.T) {
	ts := httptest.NewServer(NewServer(NewStore()))
	defer ts.Close()
	url := "ws" + strings.TrimPrefix(ts.URL, "http") + "/reg"
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	host, err := Dial(ctx, url)
	if err != nil {
		t.Fatalf("dial: %v", err)
	}
	defer host.Close()
	if err := host.Register(Service{Name: "a", PeerID: "p"}, time.Minute); err != nil {
		t.Fatalf("register: %v", err)
	}

	resp, err := http.Get(ts.URL + "/services")
	if err != nil {
		t.Fatalf("GET /services: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status %d", resp.StatusCode)
	}
}

package registry

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	"github.com/libp2p/go-libp2p/core/host"
	"github.com/libp2p/go-libp2p/core/peer"
)

// Client talks to a hub's registry over the RegistryProtocolID stream protocol.
// It is stateless: each call opens a fresh stream on the (already-open)
// libp2p connection to the hub, so it is safe for concurrent use.
type Client struct {
	h   host.Host
	hub peer.ID
}

// NewClient returns a registry client that talks to hub over h. The caller is
// responsible for having connected h to hub (host.Connect) before use.
func NewClient(h host.Host, hub peer.ID) *Client {
	return &Client{h: h, hub: hub}
}

// roundtrip opens one stream, writes m, reads one response, and closes.
func (c *Client) roundtrip(ctx context.Context, m Msg) (Msg, error) {
	s, err := c.h.NewStream(ctx, c.hub, RegistryProtocolID)
	if err != nil {
		return Msg{}, fmt.Errorf("registry: open stream: %w", err)
	}
	defer s.Close()
	if err := json.NewEncoder(s).Encode(m); err != nil {
		_ = s.Reset()
		return Msg{}, fmt.Errorf("registry: write: %w", err)
	}
	var resp Msg
	if err := json.NewDecoder(s).Decode(&resp); err != nil {
		_ = s.Reset()
		return Msg{}, fmt.Errorf("registry: read: %w", err)
	}
	if resp.Type == "error" {
		return resp, fmt.Errorf("registry: %s", resp.Error)
	}
	return resp, nil
}

// ttlSeconds converts a Duration to the whole-second granularity of the wire
// protocol. A positive sub-second TTL rounds up to 1s rather than truncating to
// 0, since the server reads a 0 TTL as "use the default" — without this, e.g.
// 500ms would silently balloon into the 90s default instead of expiring soon.
// A zero or negative TTL stays 0, deliberately requesting the server default.
func ttlSeconds(ttl time.Duration) int {
	secs := int(ttl / time.Second)
	if secs == 0 && ttl > 0 {
		secs = 1
	}
	return secs
}

// Register stores a service with the given TTL.
func (c *Client) Register(ctx context.Context, svc Service, ttl time.Duration) error {
	_, err := c.roundtrip(ctx, Msg{Type: "register", Service: &svc, TTLSeconds: ttlSeconds(ttl)})
	return err
}

// Renew refreshes a service's TTL by name.
func (c *Client) Renew(ctx context.Context, name string, ttl time.Duration) error {
	_, err := c.roundtrip(ctx, Msg{Type: "renew", Name: name, TTLSeconds: ttlSeconds(ttl)})
	return err
}

// Deregister removes a service by name.
func (c *Client) Deregister(ctx context.Context, name string) error {
	_, err := c.roundtrip(ctx, Msg{Type: "deregister", Name: name})
	return err
}

// Resolve returns the Service registered under name.
func (c *Client) Resolve(ctx context.Context, name string) (Service, error) {
	resp, err := c.roundtrip(ctx, Msg{Type: "resolve", Name: name})
	if err != nil {
		return Service{}, err
	}
	if resp.Service == nil {
		return Service{}, fmt.Errorf("registry: resolve %s: empty response", name)
	}
	return *resp.Service, nil
}

// List returns the services in the directory, optionally filtered by tag.
func (c *Client) List(ctx context.Context, tag string) ([]Service, error) {
	resp, err := c.roundtrip(ctx, Msg{Type: "list", Tag: tag})
	if err != nil {
		return nil, err
	}
	return resp.Services, nil
}

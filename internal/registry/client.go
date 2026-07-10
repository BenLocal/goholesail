package registry

import (
	"context"
	"fmt"
	"sync"
	"time"

	"github.com/gorilla/websocket"
)

// Client is a registry ws client. Its request/response calls are serialized, so
// it is safe to share between a host's register call and its renew goroutine.
type Client struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

// Dial connects to a registry ws endpoint, e.g. "ws://hub:8080/reg".
func Dial(ctx context.Context, url string) (*Client, error) {
	c, _, err := websocket.DefaultDialer.DialContext(ctx, url, nil)
	if err != nil {
		return nil, fmt.Errorf("registry: dial %s: %w", url, err)
	}
	return &Client{conn: c}, nil
}

func (c *Client) roundtrip(m Msg) (Msg, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if err := c.conn.WriteJSON(m); err != nil {
		return Msg{}, fmt.Errorf("registry: write: %w", err)
	}
	var resp Msg
	if err := c.conn.ReadJSON(&resp); err != nil {
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
func (c *Client) Register(svc Service, ttl time.Duration) error {
	_, err := c.roundtrip(Msg{Type: "register", Service: &svc, TTLSeconds: ttlSeconds(ttl)})
	return err
}

// Renew refreshes a service's TTL by name.
func (c *Client) Renew(name string, ttl time.Duration) error {
	_, err := c.roundtrip(Msg{Type: "renew", Name: name, TTLSeconds: ttlSeconds(ttl)})
	return err
}

// Deregister removes a service by name.
func (c *Client) Deregister(name string) error {
	_, err := c.roundtrip(Msg{Type: "deregister", Name: name})
	return err
}

// Resolve returns the Service registered under name.
func (c *Client) Resolve(name string) (Service, error) {
	resp, err := c.roundtrip(Msg{Type: "resolve", Name: name})
	if err != nil {
		return Service{}, err
	}
	if resp.Service == nil {
		return Service{}, fmt.Errorf("registry: resolve %s: empty response", name)
	}
	return *resp.Service, nil
}

// Close closes the underlying connection.
func (c *Client) Close() error { return c.conn.Close() }

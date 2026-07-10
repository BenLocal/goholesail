package registry

import (
	"encoding/json"
	"log"
	"time"

	"github.com/libp2p/go-libp2p/core/network"
)

// Server serves the registry over the libp2p RegistryProtocolID stream protocol.
// The hub mounts HandleStream on its host with SetStreamHandler.
type Server struct {
	store  *Store
	logger *log.Logger // nil => silent; the hub injects one, tests stay quiet
}

// NewServer wraps a Store with no logging. If store is nil a fresh one is
// created.
func NewServer(store *Store) *Server {
	return NewServerWithLogger(store, nil)
}

// NewServerWithLogger wraps a Store and logs each request to logger. A nil
// logger is silent, so callers opt into logging explicitly. Secrets are never
// logged (the registry is zero-trust and holds none).
func NewServerWithLogger(store *Store, logger *log.Logger) *Server {
	if store == nil {
		store = NewStore()
	}
	return &Server{store: store, logger: logger}
}

// logf logs a request event when a logger was injected, else it is a no-op.
func (s *Server) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

// HandleStream serves one registry request on a libp2p stream: read a Msg,
// dispatch, write the response, then close. libp2p authenticates the remote
// peer (Noise), so stream.Conn().RemotePeer() is the true caller identity.
func (s *Server) HandleStream(stream network.Stream) {
	defer stream.Close()
	s.logf("stream from %s", stream.Conn().RemotePeer())
	var m Msg
	if err := json.NewDecoder(stream).Decode(&m); err != nil {
		_ = stream.Reset()
		return
	}
	if resp, ok := s.handle(m); ok {
		if err := json.NewEncoder(stream).Encode(resp); err != nil {
			_ = stream.Reset()
		}
	}
}

func (s *Server) handle(m Msg) (Msg, bool) {
	switch m.Type {
	case "register":
		if m.Service == nil || m.Service.Name == "" {
			s.logf("register: missing service")
			return Msg{Type: "error", Error: "register: missing service"}, true
		}
		ttl := time.Duration(m.TTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 90 * time.Second
		}
		s.store.Put(*m.Service, ttl)
		s.logf("register name=%s peer=%s private=%v tags=%v ttl=%s",
			m.Service.Name, m.Service.PeerID, m.Service.Private, m.Service.Tags, ttl)
		return Msg{Type: "ok"}, true
	case "renew":
		ttl := time.Duration(m.TTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 90 * time.Second
		}
		if !s.store.Renew(m.Name, ttl) {
			s.logf("renew name=%s: unknown", m.Name)
			return Msg{Type: "error", Error: "renew: unknown name"}, true
		}
		s.logf("renew name=%s ttl=%s", m.Name, ttl)
		return Msg{Type: "ok"}, true
	case "deregister":
		s.store.Remove(m.Name)
		s.logf("deregister name=%s", m.Name)
		return Msg{Type: "ok"}, true
	case "resolve":
		svc, ok := s.store.Get(m.Name)
		if !ok {
			s.logf("resolve name=%s: not found", m.Name)
			return Msg{Type: "error", Error: "resolve: unknown name"}, true
		}
		s.logf("resolve name=%s -> %s", m.Name, svc.PeerID)
		return Msg{Type: "resolved", Service: &svc}, true
	case "list":
		svcs := s.store.List(m.Tag)
		s.logf("list tag=%q -> %d services", m.Tag, len(svcs))
		return Msg{Type: "services", Services: svcs}, true
	default:
		s.logf("unknown message type %q", m.Type)
		return Msg{Type: "error", Error: "unknown message type"}, true
	}
}

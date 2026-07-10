package registry

import (
	"encoding/json"
	"log"
	"net/http"
	"time"

	"github.com/gorilla/websocket"
)

// Server exposes a Store over websockets (path /reg) plus a read-only JSON
// listing (GET /services). It is a plain http.Handler, so the hub mounts it on
// whatever --registry-listen address it chooses.
type Server struct {
	store    *Store
	upgrader websocket.Upgrader
	mux      *http.ServeMux
	logger   *log.Logger // nil => silent; the hub injects one, tests stay quiet
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
	s := &Server{store: store, mux: http.NewServeMux(), logger: logger}
	s.mux.HandleFunc("/reg", s.handleWS)
	s.mux.HandleFunc("/services", s.handleList)
	return s
}

// logf logs a request event when a logger was injected, else it is a no-op.
func (s *Server) logf(format string, args ...any) {
	if s.logger != nil {
		s.logger.Printf(format, args...)
	}
}

func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) { s.mux.ServeHTTP(w, r) }

func (s *Server) handleList(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(s.store.List(r.URL.Query().Get("tag")))
}

func (s *Server) handleWS(w http.ResponseWriter, r *http.Request) {
	c, err := s.upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer c.Close()
	s.logf("ws conn open from %s", r.RemoteAddr)
	defer s.logf("ws conn close from %s", r.RemoteAddr)
	// One reader, one writer per connection: request in, response out. No
	// server-initiated pushes (subscribe is deferred), so no write mutex needed.
	//
	// TODO(resilience, M4): this loop sets no read deadline and runs no
	// ping/pong keepalive, so a client that connects and then goes idle (or is
	// silently dropped at the TCP layer) parks this goroutine and its FD in
	// ReadJSON until an error arrives, which may be never. The default upgrader
	// also accepts any Origin. Both are accepted debt for M3 — an internal tool
	// where resilience is an explicit non-goal — and belong with M4's hardening.
	for {
		var m Msg
		if err := c.ReadJSON(&m); err != nil {
			return
		}
		if resp, ok := s.handle(m); ok {
			if err := c.WriteJSON(resp); err != nil {
				return
			}
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

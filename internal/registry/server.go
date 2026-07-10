package registry

import (
	"encoding/json"
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
}

// NewServer wraps a Store. If store is nil a fresh one is created.
func NewServer(store *Store) *Server {
	if store == nil {
		store = NewStore()
	}
	s := &Server{store: store, mux: http.NewServeMux()}
	s.mux.HandleFunc("/reg", s.handleWS)
	s.mux.HandleFunc("/services", s.handleList)
	return s
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
			return Msg{Type: "error", Error: "register: missing service"}, true
		}
		ttl := time.Duration(m.TTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 90 * time.Second
		}
		s.store.Put(*m.Service, ttl)
		return Msg{Type: "ok"}, true
	case "renew":
		ttl := time.Duration(m.TTLSeconds) * time.Second
		if ttl <= 0 {
			ttl = 90 * time.Second
		}
		if !s.store.Renew(m.Name, ttl) {
			return Msg{Type: "error", Error: "renew: unknown name"}, true
		}
		return Msg{Type: "ok"}, true
	case "deregister":
		s.store.Remove(m.Name)
		return Msg{Type: "ok"}, true
	case "resolve":
		svc, ok := s.store.Get(m.Name)
		if !ok {
			return Msg{Type: "error", Error: "resolve: unknown name"}, true
		}
		return Msg{Type: "resolved", Service: &svc}, true
	case "list":
		return Msg{Type: "services", Services: s.store.List(m.Tag)}, true
	default:
		return Msg{Type: "error", Error: "unknown message type"}, true
	}
}

// Package registry is a zero-trust service directory: it maps a human name to a
// host's {peerID, hub, private, tags}. It NEVER stores a private secret; clients
// supply the secret out-of-band via --secret.
package registry

// Service is a directory entry. It deliberately has no Secret field.
type Service struct {
	Name    string   `json:"name"`
	PeerID  string   `json:"peer_id"`
	Hub     string   `json:"hub"`
	Private bool     `json:"private"`
	Tags    []string `json:"tags,omitempty"`
}

// Msg is the ws wire frame. Type discriminates; unused fields are omitted.
type Msg struct {
	Type       string    `json:"type"`
	Service    *Service  `json:"service,omitempty"`
	Services   []Service `json:"services,omitempty"`
	Name       string    `json:"name,omitempty"`
	Tag        string    `json:"tag,omitempty"`
	TTLSeconds int       `json:"ttl_seconds,omitempty"`
	Error      string    `json:"error,omitempty"`
}

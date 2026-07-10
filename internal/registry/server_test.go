package registry

import (
	"bytes"
	"log"
	"strings"
	"testing"
)

func TestServerLogsEvents(t *testing.T) {
	var buf bytes.Buffer
	s := NewServerWithLogger(NewStore(), log.New(&buf, "", 0))

	svc := Service{Name: "home-ssh", PeerID: "12D3KooWTEST", Private: true, Tags: []string{"ssh"}}
	if _, ok := s.handle(Msg{Type: "register", Service: &svc, TTLSeconds: 90}); !ok {
		t.Fatal("register: expected a response")
	}
	if _, ok := s.handle(Msg{Type: "resolve", Name: "home-ssh"}); !ok {
		t.Fatal("resolve: expected a response")
	}
	if _, ok := s.handle(Msg{Type: "deregister", Name: "home-ssh"}); !ok {
		t.Fatal("deregister: expected a response")
	}

	out := buf.String()
	for _, want := range []string{
		"register name=home-ssh peer=12D3KooWTEST private=true",
		"resolve name=home-ssh -> 12D3KooWTEST",
		"deregister name=home-ssh",
	} {
		if !strings.Contains(out, want) {
			t.Errorf("log missing %q\nfull log:\n%s", want, out)
		}
	}
}

func TestServerNilLoggerSilent(t *testing.T) {
	// NewServer (nil logger) must handle requests without panicking.
	s := NewServer(NewStore())
	svc := Service{Name: "x", PeerID: "p"}
	if _, ok := s.handle(Msg{Type: "register", Service: &svc, TTLSeconds: 30}); !ok {
		t.Fatal("register: expected a response")
	}
	if _, ok := s.handle(Msg{Type: "resolve", Name: "missing"}); !ok {
		t.Fatal("resolve: expected a response")
	}
}

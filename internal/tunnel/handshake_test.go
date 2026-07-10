package tunnel

import (
	"net"
	"testing"
)

func TestHandshakePublicNoOp(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	errc := make(chan error, 1)
	go func() { errc <- ServerHandshake(a, "") }()
	if err := ClientHandshake(b, ""); err != nil {
		t.Fatalf("client public: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("server public: %v", err)
	}
}

func TestHandshakePrivateSuccess(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	errc := make(chan error, 1)
	go func() { errc <- ServerHandshake(a, "s3cr3t") }()
	if err := ClientHandshake(b, "s3cr3t"); err != nil {
		t.Fatalf("client private: %v", err)
	}
	if err := <-errc; err != nil {
		t.Fatalf("server private: %v", err)
	}
}

func TestHandshakeWrongSecretRejected(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	defer b.Close()
	errc := make(chan error, 1)
	go func() { errc <- ServerHandshake(a, "right") }()
	_ = ClientHandshake(b, "wrong")
	if err := <-errc; err == nil {
		t.Fatal("server accepted a wrong-secret token")
	}
}

func TestHandshakeClientMissingSecretRejected(t *testing.T) {
	a, b := net.Pipe()
	defer a.Close()
	errc := make(chan error, 1)
	go func() { errc <- ServerHandshake(a, "right") }()
	if err := ClientHandshake(b, ""); err == nil {
		t.Fatal("client with no secret should error against a private host")
	}
	_ = b.Close()
	if err := <-errc; err == nil {
		t.Fatal("server should reject when no token arrives")
	}
	_ = a.Close()
}

package tunnel

import (
	"io"
	"net"
	"testing"
	"time"
)

// TestPumpForwardsBothDirections wires two net.Pipe pairs together with Pump
// and asserts bytes cross in both directions.
func TestPumpForwardsBothDirections(t *testing.T) {
	leftClient, leftServer := net.Pipe()
	rightServer, rightClient := net.Pipe()

	go Pump(leftServer, rightServer)

	// left -> right
	go func() { _, _ = leftClient.Write([]byte("ping")) }()
	got := make([]byte, 4)
	_ = rightClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(rightClient, got); err != nil {
		t.Fatalf("read right: %v", err)
	}
	if string(got) != "ping" {
		t.Fatalf("left->right got %q, want ping", got)
	}

	// right -> left
	go func() { _, _ = rightClient.Write([]byte("pong")) }()
	got2 := make([]byte, 4)
	_ = leftClient.SetReadDeadline(time.Now().Add(2 * time.Second))
	if _, err := io.ReadFull(leftClient, got2); err != nil {
		t.Fatalf("read left: %v", err)
	}
	if string(got2) != "pong" {
		t.Fatalf("right->left got %q, want pong", got2)
	}
}

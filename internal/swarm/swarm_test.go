package swarm

import (
	"bytes"
	"testing"
)

func TestPSKDeterministicAndLength(t *testing.T) {
	a := PSK("hunter2")
	b := PSK("hunter2")
	if !bytes.Equal(a, b) {
		t.Fatal("PSK not deterministic for the same passphrase")
	}
	if len(a) != 32 {
		t.Fatalf("PSK len = %d, want 32", len(a))
	}
	if bytes.Equal(a, PSK("different")) {
		t.Fatal("different passphrases produced the same PSK")
	}
}

func TestOptionsEmptyVsSet(t *testing.T) {
	if got := Options(""); got != nil {
		t.Fatalf("Options(\"\") = %d opts, want nil", len(got))
	}
	if got := Options("hunter2"); len(got) != 2 {
		t.Fatalf("Options(passphrase) = %d opts, want 2", len(got))
	}
}

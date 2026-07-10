package connstr

import (
	"strings"
	"testing"
)

func TestEncodeDecodeRoundTripPrivate(t *testing.T) {
	in := ConnString{
		Version: 1,
		Private: true,
		HostID:  "12D3KooWHostExampleID",
		Hub:     "/dns4/hub.example.com/tcp/4001/p2p/12D3KooWHubExampleID",
		Secret:  "shared-secret-value",
	}
	s, err := Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(s, "ghs://s1_") {
		t.Fatalf("private v1 must start with ghs://s1_, got %q", s)
	}
	out, err := Decode(s)
	if err != nil {
		t.Fatalf("Decode: %v", err)
	}
	if out != in {
		t.Fatalf("round trip mismatch:\n got %+v\nwant %+v", out, in)
	}
}

func TestEncodePublicHasPPrefixAndNoSecret(t *testing.T) {
	in := ConnString{Version: 1, Private: false, HostID: "id", Hub: "/p2p/hub", Secret: ""}
	s, err := Encode(in)
	if err != nil {
		t.Fatalf("Encode: %v", err)
	}
	if !strings.HasPrefix(s, "ghs://p1_") {
		t.Fatalf("public v1 must start with ghs://p1_, got %q", s)
	}
}

func TestDecodeRejectsBadScheme(t *testing.T) {
	if _, err := Decode("http://nope"); err == nil {
		t.Fatal("expected error for non-ghs scheme")
	}
}

func TestDecodeRejectsGarbagePayload(t *testing.T) {
	if _, err := Decode("ghs://s1_!!!not-base64!!!"); err == nil {
		t.Fatal("expected error for undecodable payload")
	}
}

func TestDecodeRejectsModeMismatch(t *testing.T) {
	// Encode a private string, then flip the mode char to 'p'.
	s, _ := Encode(ConnString{Version: 1, Private: true, HostID: "id", Hub: "/p2p/hub", Secret: "x"})
	tampered := "ghs://p1_" + strings.TrimPrefix(s, "ghs://s1_")
	if _, err := Decode(tampered); err == nil {
		t.Fatal("expected error when prefix mode disagrees with payload")
	}
}

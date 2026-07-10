package tunnel

import (
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"fmt"
	"io"
)

// Handshake wire format, exchanged at the very start of a tunnel stream,
// on top of libp2p's Noise encryption:
//
//	host -> client: [1-byte flag][16-byte nonce]   flag 0=public, 1=private
//	if flag==1, client -> host: [32-byte HMAC-SHA256(secret, nonce)]
//
// The host's flag is authoritative: a public host sends flag 0 and the client
// sends nothing back, so public streams carry zero extra bytes.
const (
	flagPublic  = 0
	flagPrivate = 1
	nonceLen    = 16
	macLen      = sha256.Size // 32
)

// ServerHandshake runs the host side. secret == "" means public mode (no auth).
// For private mode the caller should set a read deadline on the stream before
// calling, to bound how long an unauthenticated peer can hold the handler.
func ServerHandshake(rw io.ReadWriter, secret string) error {
	var hdr [1 + nonceLen]byte
	if secret == "" {
		hdr[0] = flagPublic
		if _, err := rw.Write(hdr[:]); err != nil {
			return fmt.Errorf("tunnel: write public header: %w", err)
		}
		return nil
	}
	hdr[0] = flagPrivate
	if _, err := rand.Read(hdr[1:]); err != nil {
		return fmt.Errorf("tunnel: nonce: %w", err)
	}
	if _, err := rw.Write(hdr[:]); err != nil {
		return fmt.Errorf("tunnel: write private header: %w", err)
	}
	var got [macLen]byte
	if _, err := io.ReadFull(rw, got[:]); err != nil {
		return fmt.Errorf("tunnel: read token: %w", err)
	}
	want := mac(secret, hdr[1:])
	if !hmac.Equal(got[:], want) {
		return fmt.Errorf("tunnel: auth failed")
	}
	return nil
}

// ClientHandshake runs the connect side. secret == "" means the client holds no
// secret; that is fine against a public host but an error against a private one.
func ClientHandshake(rw io.ReadWriter, secret string) error {
	var hdr [1 + nonceLen]byte
	if _, err := io.ReadFull(rw, hdr[:]); err != nil {
		return fmt.Errorf("tunnel: read header: %w", err)
	}
	switch hdr[0] {
	case flagPublic:
		return nil
	case flagPrivate:
		if secret == "" {
			return fmt.Errorf("tunnel: host requires a secret but none was provided")
		}
		if _, err := rw.Write(mac(secret, hdr[1:])); err != nil {
			return fmt.Errorf("tunnel: write token: %w", err)
		}
		return nil
	default:
		return fmt.Errorf("tunnel: unknown auth flag %d", hdr[0])
	}
}

func mac(secret string, nonce []byte) []byte {
	m := hmac.New(sha256.New, []byte(secret))
	m.Write(nonce)
	return m.Sum(nil)
}

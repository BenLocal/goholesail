// Package tunnel defines the goholesail data-plane stream protocol and the
// byte pump that splices two connections together.
package tunnel

import (
	"io"

	"github.com/libp2p/go-libp2p/core/protocol"
)

// ProtocolID is the libp2p protocol spoken over each tunnel stream.
const ProtocolID protocol.ID = "/goholesail/tunnel/1.0.0"

// Pump copies bytes bidirectionally between a and b until both directions
// finish (either side closing ends its direction). It closes both ends.
func Pump(a, b io.ReadWriteCloser) {
	done := make(chan struct{}, 2)
	cp := func(dst io.WriteCloser, src io.Reader) {
		_, _ = io.Copy(dst, src)
		_ = dst.Close()
		done <- struct{}{}
	}
	go cp(a, b)
	go cp(b, a)
	<-done
	<-done
}

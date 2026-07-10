package registry

import (
	"testing"
	"time"
)

func TestTTLSeconds(t *testing.T) {
	cases := []struct {
		name string
		ttl  time.Duration
		want int
	}{
		{"whole seconds", 90 * time.Second, 90},
		{"renew interval", 30 * time.Second, 30},
		{"sub-second rounds up to 1s (not the server default)", 500 * time.Millisecond, 1},
		{"just under a second rounds up", 999 * time.Millisecond, 1},
		{"fractional truncates its remainder", 1500 * time.Millisecond, 1},
		{"zero requests the server default", 0, 0},
		{"negative requests the server default", -5 * time.Second, -5},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ttlSeconds(tc.ttl); got != tc.want {
				t.Fatalf("ttlSeconds(%v) = %d, want %d", tc.ttl, got, tc.want)
			}
		})
	}
}

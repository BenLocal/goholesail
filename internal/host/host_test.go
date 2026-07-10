package host

import (
	"testing"
	"time"
)

func TestReservationRenewWait(t *testing.T) {
	now := time.Unix(1_000_000, 0)
	cases := []struct {
		name string
		exp  time.Time
		want time.Duration
	}{
		{"two seconds -> renew at 1.5s", now.Add(2 * time.Second), 1500 * time.Millisecond},
		{"one hour -> renew at 45m", now.Add(time.Hour), 45 * time.Minute},
		{"already expired -> floor 1s", now.Add(-time.Second), time.Second},
		{"exactly now -> floor 1s", now, time.Second},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			if got := reservationRenewWait(c.exp, now); got != c.want {
				t.Fatalf("reservationRenewWait = %s, want %s", got, c.want)
			}
		})
	}
}

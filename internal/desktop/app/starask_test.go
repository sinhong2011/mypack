package app

import (
	"testing"
	"time"
)

func TestStarAskDue(t *testing.T) {
	now := time.Date(2026, 11, 12, 12, 0, 0, 0, time.UTC)
	cases := []struct {
		name string
		s    starAsk
		want bool
	}{
		{"too few builds", starAsk{Builds: 2}, false},
		{"first ask", starAsk{Builds: 3}, true},
		{"starred", starAsk{Builds: 3, Done: true}, false},
		{"second ask too soon in builds", starAsk{Builds: 11, Asked: 1, Last: now.Add(-30 * 24 * time.Hour)}, false},
		{"second ask too soon in time", starAsk{Builds: 12, Asked: 1, Last: now.Add(-13 * 24 * time.Hour)}, false},
		{"second ask", starAsk{Builds: 12, Asked: 1, Last: now.Add(-14 * 24 * time.Hour)}, true},
		{"asked twice", starAsk{Builds: 50, Asked: 2, Last: now.Add(-365 * 24 * time.Hour)}, false},
	}
	for _, c := range cases {
		if got := c.s.due(now); got != c.want {
			t.Errorf("%s: due = %v, want %v", c.name, got, c.want)
		}
	}
}

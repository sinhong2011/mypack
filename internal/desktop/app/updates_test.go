package app

import (
	"errors"
	"testing"
)

func TestMissingUpdateManifest(t *testing.T) {
	err404 := errors.New("mygo: checking for updates: GET https://github.com/sinhong2011/mypack/releases/download/v0.1.0-beta.4/update-darwin-universal.json: 404 Not Found")
	cases := []struct {
		err  error
		want bool
	}{
		{nil, false},
		{errors.New("network down"), false},
		{errors.New("404 page not found"), false},
		{err404, true},
	}
	for _, c := range cases {
		if got := missingUpdateManifest(c.err); got != c.want {
			t.Errorf("missingUpdateManifest(%v) = %v, want %v", c.err, got, c.want)
		}
	}
}

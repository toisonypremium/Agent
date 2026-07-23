package main

import "testing"

func TestLoopbackTCPAddr(t *testing.T) {
	for _, tc := range []struct {
		in      string
		wantErr bool
	}{
		{"127.0.0.1:8787", false},
		{"[::1]:8787", false},
		{"0.0.0.0:8787", true},
		{"localhost:8787", true},
		{"198.51.100.1:8787", true},
		{"127.0.0.1", true},
	} {
		_, err := loopbackTCPAddr(tc.in)
		if (err != nil) != tc.wantErr {
			t.Fatalf("addr=%q err=%v", tc.in, err)
		}
	}
}

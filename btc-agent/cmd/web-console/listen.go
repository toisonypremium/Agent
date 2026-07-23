package main

import (
	"fmt"
	"net"
)

// loopbackTCPAddr rejects public or wildcard bindings. Production access, if
// approved, must terminate at a separate identity-aware proxy that connects to
// this local-only process.
func loopbackTCPAddr(addr string) (string, error) {
	host, port, err := net.SplitHostPort(addr)
	if err != nil || port == "" {
		return "", fmt.Errorf("web console listen address must include host and port")
	}
	ip := net.ParseIP(host)
	if ip == nil || !ip.IsLoopback() {
		return "", fmt.Errorf("web console must bind a loopback IP address")
	}
	return net.JoinHostPort(ip.String(), port), nil
}

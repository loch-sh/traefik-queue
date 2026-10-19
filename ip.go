package loch_queue

import (
	"net"
	"net/http"
	"strings"
)

// getClientIP extracts the client's real IP address, respecting common proxy headers.
func getClientIP(req *http.Request) string {
	// 1. X-Forwarded-For: Can be a comma-separated list. The first one is often the original client.
	xff := req.Header.Get("X-Forwarded-For")
	if xff != "" {
		ips := strings.Split(xff, ",")
		firstIP := strings.TrimSpace(ips[0])
		if firstIP != "" {
			return firstIP
		}
	}

	// 2. X-Real-IP: Typically a single IP.
	xrip := req.Header.Get("X-Real-IP")
	if xrip != "" {
		return strings.TrimSpace(xrip)
	}

	// 3. Fly-Client-IP: Header used by Fly.io
	flyIP := req.Header.Get("Fly-Client-IP")
	if flyIP != "" {
		return strings.TrimSpace(flyIP)
	}

	// 4. CF-Connecting-IP: Header used by Cloudflare
	cfIP := req.Header.Get("CF-Connecting-IP")
	if cfIP != "" {
		return strings.TrimSpace(cfIP)
	}

	// 5. Fallback to RemoteAddr: This might be the IP of the immediate upstream proxy.
	// RemoteAddr is typically in "ip:port" format.
	remoteAddr := req.RemoteAddr
	host, _, err := net.SplitHostPort(remoteAddr) // net is not imported, need to import "net"
	if err == nil && host != "" {
		return host
	}

	// If SplitHostPort fails (e.g. RemoteAddr is just an IP), return RemoteAddr as is.
	return remoteAddr
}

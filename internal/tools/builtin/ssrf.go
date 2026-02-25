package builtin

import (
	"context"
	"fmt"
	"net"
	"net/url"
	"time"
)

var (
	// privateNetworks contains CIDR ranges that should be blocked.
	privateNetworks = []string{
		"127.0.0.0/8",    // Loopback
		"10.0.0.0/8",     // RFC 1918
		"172.16.0.0/12",  // RFC 1918
		"192.168.0.0/16", // RFC 1918
		"169.254.0.0/16", // Link-local
		"100.64.0.0/10",  // CGNAT
		"0.0.0.0/8",      // Unspecified
		"::1/128",        // IPv6 loopback
		"fc00::/7",       // IPv6 ULA
		"fe80::/10",      // IPv6 link-local
	}

	parsedNetworks []*net.IPNet
)

func init() {
	for _, cidr := range privateNetworks {
		_, network, _ := net.ParseCIDR(cidr)
		if network != nil {
			parsedNetworks = append(parsedNetworks, network)
		}
	}
}

// checkSSRF validates that the target URL does not resolve to a private/internal IP.
func checkSSRF(rawURL string, allowedDomains []string) error {
	u, err := url.Parse(rawURL)
	if err != nil {
		return fmt.Errorf("invalid URL: %w", err)
	}

	// Scheme check
	if u.Scheme != "http" && u.Scheme != "https" {
		return fmt.Errorf("blocked scheme: %s (only http/https allowed)", u.Scheme)
	}

	hostname := u.Hostname()

	// Check allowed domains whitelist
	for _, d := range allowedDomains {
		if hostname == d {
			return nil // Whitelisted
		}
	}

	// Check if hostname is a raw IP address first
	if ip := net.ParseIP(hostname); ip != nil {
		if isPrivateIP(ip) {
			return fmt.Errorf("blocked: %s is a private IP", hostname)
		}
		return nil
	}

	// DNS resolve with timeout
	resolver := &net.Resolver{}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	ips, err := resolver.LookupIPAddr(ctx, hostname)
	if err != nil {
		return fmt.Errorf("DNS resolution failed for %s: %w", hostname, err)
	}

	for _, ip := range ips {
		if isPrivateIP(ip.IP) {
			return fmt.Errorf("blocked: %s resolves to private IP %s", hostname, ip.IP)
		}
	}

	return nil
}

// isPrivateIP checks if an IP falls within any private/reserved range.
func isPrivateIP(ip net.IP) bool {
	for _, network := range parsedNetworks {
		if network.Contains(ip) {
			return true
		}
	}
	return false
}

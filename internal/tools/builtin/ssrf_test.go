package builtin

import (
	"net"
	"testing"
)

func TestCheckSSRF_PublicIP_Allowed(t *testing.T) {
	// Direct IP to a public address should pass
	err := checkSSRF("http://8.8.8.8/path", nil)
	if err != nil {
		t.Errorf("expected public IP to be allowed, got: %v", err)
	}
}

func TestCheckSSRF_Loopback_Blocked(t *testing.T) {
	err := checkSSRF("http://127.0.0.1/admin", nil)
	if err == nil {
		t.Error("expected loopback to be blocked")
	}
}

func TestCheckSSRF_RFC1918_10_Blocked(t *testing.T) {
	err := checkSSRF("http://10.0.0.1:8080/api", nil)
	if err == nil {
		t.Error("expected 10.x.x.x to be blocked")
	}
}

func TestCheckSSRF_RFC1918_192_Blocked(t *testing.T) {
	err := checkSSRF("http://192.168.1.1/", nil)
	if err == nil {
		t.Error("expected 192.168.x.x to be blocked")
	}
}

func TestCheckSSRF_IPv6Loopback_Blocked(t *testing.T) {
	err := checkSSRF("http://[::1]:8080/", nil)
	if err == nil {
		t.Error("expected IPv6 loopback to be blocked")
	}
}

func TestCheckSSRF_BadScheme_Blocked(t *testing.T) {
	err := checkSSRF("file:///etc/passwd", nil)
	if err == nil {
		t.Error("expected file:// scheme to be blocked")
	}

	err = checkSSRF("ftp://internal.server/data", nil)
	if err == nil {
		t.Error("expected ftp:// scheme to be blocked")
	}
}

func TestCheckSSRF_AllowedDomain_Pass(t *testing.T) {
	err := checkSSRF("http://127.0.0.1/api", []string{"127.0.0.1"})
	if err != nil {
		t.Errorf("expected whitelisted domain to pass, got: %v", err)
	}
}

func TestCheckSSRF_InvalidURL(t *testing.T) {
	err := checkSSRF("://bad-url", nil)
	if err == nil {
		t.Error("expected invalid URL to be rejected")
	}
}

func TestIsPrivateIP(t *testing.T) {
	tests := []struct {
		ip      string
		private bool
	}{
		{"127.0.0.1", true},
		{"10.0.0.1", true},
		{"172.16.0.1", true},
		{"192.168.1.1", true},
		{"169.254.1.1", true},
		{"100.64.0.1", true},
		{"0.0.0.0", true},
		{"8.8.8.8", false},
		{"1.1.1.1", false},
		{"203.0.113.1", false},
		{"::1", true},
		{"fe80::1", true},
		{"fc00::1", true},
		{"2001:4860:4860::8888", false},
	}
	for _, tt := range tests {
		ip := net.ParseIP(tt.ip)
		got := isPrivateIP(ip)
		if got != tt.private {
			t.Errorf("isPrivateIP(%s) = %v, want %v", tt.ip, got, tt.private)
		}
	}
}

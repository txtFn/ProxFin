package src

import (
	"net"
	"testing"
)

func TestDirectHLSPublicIP(t *testing.T) {
	tests := []struct {
		address string
		allowed bool
	}{
		{"127.0.0.1", false},
		{"10.0.0.1", false},
		{"192.168.1.1", false},
		{"172.16.0.1", false},
		{"169.254.169.254", false},
		{"100.100.100.200", false},
		{"198.18.0.1", false},
		{"192.0.2.1", false},
		{"203.0.113.1", false},
		{"::1", false},
		{"fc00::1", false},
		{"fe80::1", false},
		{"2001:db8::1", false},
		{"8.8.8.8", true},
		{"1.1.1.1", true},
		{"2606:4700:4700::1111", true},
	}

	for _, tt := range tests {
		t.Run(tt.address, func(t *testing.T) {
			result := directHLSPublicIP(net.ParseIP(tt.address))

			if result != tt.allowed {
				t.Errorf(
					"IP %s: expected allowed=%v, got %v",
					tt.address,
					tt.allowed,
					result,
				)
			}
		})
	}
}

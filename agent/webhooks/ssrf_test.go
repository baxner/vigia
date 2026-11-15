package webhooks

import (
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestValidateWebhookURL(t *testing.T) {
	valid := []string{
		"http://homeassistant.local:8123/api/webhook/vigia",
		"https://ntfy.sh/vigia-alerts",
		"http://192.168.1.50:8080/hook",
		"http://100.64.1.10:9000/hook",
		"https://[::1]:8443/hook",
	}
	for _, u := range valid {
		if err := ValidateWebhookURL(u); err != nil {
			t.Errorf("ValidateWebhookURL(%q) error = %v, want nil", u, err)
		}
	}

	invalid := []string{
		"",
		"not-a-url",
		"ftp://example.com/hook",
		"file:///etc/passwd",
		"javascript:alert(1)",
		"http:///missing-host",
		"https://:8080/missing-host",
		"http://user:pass@example.com/hook",
		"https://token@example.com/hook",
	}
	for _, u := range invalid {
		if err := ValidateWebhookURL(u); err == nil {
			t.Errorf("ValidateWebhookURL(%q) = nil, want error", u)
		}
	}
}

func TestIsBlockedIP_AllowsLANAndTailscaleBlocksSpecial(t *testing.T) {
	cases := []struct {
		ip      string
		blocked bool
	}{
		{"127.0.0.1", true},
		{"127.255.255.254", true},
		{"::1", true},
		{"::ffff:127.0.0.1", true},
		{"0.0.0.0", true},
		{"::", true},
		{"169.254.169.254", true},
		{"169.254.0.1", true},
		{"::ffff:169.254.169.254", true},
		{"fe80::1", true},
		{"224.0.0.1", true},
		{"ff02::1", true},
		// Permitidas: LAN privada y Tailscale (CGNAT) y públicas
		{"192.168.0.10", false},
		{"10.0.0.5", false},
		{"172.16.5.2", false},
		{"100.64.0.1", false},
		{"100.120.5.9", false},
		{"93.184.216.34", false},
	}
	for _, tc := range cases {
		ip := net.ParseIP(tc.ip)
		if ip == nil {
			t.Fatalf("ParseIP(%q) = nil", tc.ip)
		}
		if got := isBlockedIP(ip); got != tc.blocked {
			t.Errorf("isBlockedIP(%s) = %v, want %v", tc.ip, got, tc.blocked)
		}
	}
}

func TestSenderBlocksLoopbackByDefault(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		t.Fatal("el servidor loopback no debía recibir la petición")
	}))
	defer srv.Close()

	s := NewSender("test")
	_, err := s.Attempt(context.Background(), Target{ID: 1, URL: srv.URL}, []byte("{}"), "TEST", "d-1")
	if !errors.Is(err, ErrBlockedAddress) {
		t.Fatalf("err = %v, want ErrBlockedAddress", err)
	}
}

package clientip

import (
	"crypto/tls"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	trusted, err := ParseList("10.0.0.0/8, 192.0.2.1")
	if err != nil {
		t.Fatal(err)
	}
	r := New(trusted)

	cases := []struct {
		name, remote, xff, want string
	}{
		{"direct client ignores header", "203.0.113.7:5000", "1.2.3.4", "203.0.113.7"},
		{"behind proxy", "10.1.2.3:5000", "198.51.100.9", "198.51.100.9"},
		{"spoofed left entry is skipped", "10.1.2.3:5000", "6.6.6.6, 198.51.100.9", "198.51.100.9"},
		{"chain of trusted proxies", "10.1.2.3:5000", "198.51.100.9, 10.9.9.9, 192.0.2.1", "198.51.100.9"},
		{"garbage stops the walk", "10.1.2.3:5000", "198.51.100.9, not-an-ip", "10.1.2.3"},
		{"no header", "10.1.2.3:5000", "", "10.1.2.3"},
		{"ipv4-mapped peer", "[::ffff:10.1.2.3]:5000", "198.51.100.9", "198.51.100.9"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			req := httptest.NewRequest("GET", "/", nil)
			req.RemoteAddr = c.remote
			if c.xff != "" {
				req.Header.Set("X-Forwarded-For", c.xff)
			}
			if got := r.ClientIP(req).String(); got != c.want {
				t.Errorf("ClientIP = %s, want %s", got, c.want)
			}
		})
	}
}

func TestIsHTTPS(t *testing.T) {
	trusted, _ := ParseList("10.0.0.0/8")
	r := New(trusted)

	req := httptest.NewRequest("GET", "/", nil)
	req.RemoteAddr = "203.0.113.7:5000"
	req.Header.Set("X-Forwarded-Proto", "https")
	if r.IsHTTPS(req) {
		t.Error("untrusted peer may not claim HTTPS")
	}
	req.RemoteAddr = "10.0.0.5:5000"
	if !r.IsHTTPS(req) {
		t.Error("trusted proxy's X-Forwarded-Proto ignored")
	}
	req.Header.Set("X-Forwarded-Proto", "http")
	if r.IsHTTPS(req) {
		t.Error("plain http reported as https")
	}
	req.TLS = &tls.ConnectionState{}
	if !r.IsHTTPS(req) {
		t.Error("direct TLS not detected")
	}
}

func TestParseList(t *testing.T) {
	if _, err := ParseList("10.0.0.0/33"); err == nil {
		t.Error("invalid prefix accepted")
	}
	if _, err := ParseList("example.com"); err == nil {
		t.Error("host name accepted")
	}
	p, err := ParseList("")
	if err != nil || len(p) != 0 {
		t.Errorf("empty list: %v %v", p, err)
	}
}

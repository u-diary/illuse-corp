package httpx

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	tests := []struct {
		remote, xff, want string
	}{
		{"127.0.0.1:1234", "198.51.100.7", "198.51.100.7"},
		{"127.0.0.1:1234", "1.2.3.4, 198.51.100.7", "198.51.100.7"}, // 先頭は偽装されうる
		{"[::1]:1234", "198.51.100.7", "198.51.100.7"},
		{"127.0.0.1:1234", "", "127.0.0.1"},
		{"192.0.2.1:1234", "198.51.100.7", "192.0.2.1"}, // プロキシ経由でなければ XFF は無視
	}
	for _, tt := range tests {
		req := httptest.NewRequest(http.MethodGet, "/", nil)
		req.RemoteAddr = tt.remote
		if tt.xff != "" {
			req.Header.Set("X-Forwarded-For", tt.xff)
		}
		if got := ClientIP(req); got != tt.want {
			t.Errorf("ClientIP(%q, %q) = %q, want %q", tt.remote, tt.xff, got, tt.want)
		}
	}
}

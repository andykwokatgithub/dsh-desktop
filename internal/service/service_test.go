package service

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestHealthy(t *testing.T) {
	cases := []struct {
		name   string
		status int
		body   string
		want   bool
	}{
		{"index served 200", http.StatusOK, "<html>dsh</html>", true},
		{"token->cookie see-other", http.StatusSeeOther, "", true},
		{"dsh auth boundary 401", http.StatusUnauthorized, "dsh web authentication required; reopen the URL printed by dsh web.\n", true},
		{"unrelated 401 is not dsh", http.StatusUnauthorized, "Access Denied\n", false},
		{"not-found during boot", http.StatusNotFound, "", false},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer srv.Close()

			u, err := url.Parse(srv.URL)
			if err != nil {
				t.Fatalf("parse server url: %v", err)
			}
			port, err := strconv.Atoi(u.Port())
			if err != nil {
				t.Fatalf("parse server port: %v", err)
			}
			b := Behavior{Host: u.Hostname(), Port: port}

			if got := b.Healthy(); got != tc.want {
				t.Fatalf("Healthy() = %v, want %v (status %d body %q)", got, tc.want, tc.status, tc.body)
			}
		})
	}
}

func TestURLFromReadyLine(t *testing.T) {
	cases := []struct {
		line string
		want string
	}{
		{"dsh web: http://127.0.0.1:3080/?token=abc (LAN: http://192.168.1.5:3080/?token=xyz)", "http://127.0.0.1:3080/?token=abc"},
		{"dsh web: http://127.0.0.1:3080/?token=abc", "http://127.0.0.1:3080/?token=abc"},
		{"dsh web:http://127.0.0.1:3080/?token=abc", "http://127.0.0.1:3080/?token=abc"},
		{"dsh web: opening the default browser; pass --no-open to disable", ""},
		{"unrelated line", ""},
	}

	for _, tc := range cases {
		if got := urlFromReadyLine(tc.line); got != tc.want {
			t.Errorf("urlFromReadyLine(%q) = %q, want %q", tc.line, got, tc.want)
		}
	}
}

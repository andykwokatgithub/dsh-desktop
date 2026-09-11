package service

import (
	"context"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strconv"
	"testing"
)

func TestParseTokenInput(t *testing.T) {
	const token = "Zm9vYmFyYmF6cXV4MTIzNDU2Nzg5MGFiY2RlZg"
	cases := []struct {
		name  string
		input string
		want  string
	}{
		{"bare token", token, token},
		{"whole authenticated url", "http://127.0.0.1:3080/?token=" + token, token},
		{"ready line with a LAN address", "dsh web: http://127.0.0.1:3080/?token=" + token + " (LAN: http://192.168.1.5:3080/?token=xyz)", token},
		{"quoted url", `"http://127.0.0.1:3080/?token=` + token + `"`, token},
		{"extra query parameters", "http://127.0.0.1:3080/?token=" + token + "&foo=bar", token},
		{"padded input", "  " + token + "\n", token},
		{"empty", "   ", ""},
		{"too short to be a token", "abc", ""},
		{"unrelated text", "hello world", ""},
		{"token parameter without value", "http://127.0.0.1:3080/?token=", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := ParseTokenInput(tc.input); got != tc.want {
				t.Fatalf("ParseTokenInput(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// TestValidateToken pins the three outcomes against a stand-in for dsh's auth
// boundary: 303 for the right token, the 401 fence for anything else, and an
// error (not a rejection) when the endpoint cannot be reached.
func TestValidateToken(t *testing.T) {
	const good = "goodtoken0123456789abcdef"
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Query().Get("token") == good {
			w.Header().Set("Set-Cookie", "dsh-auth-x=y; Path=/")
			w.WriteHeader(http.StatusSeeOther)
			return
		}
		w.Header().Set("Content-Type", "text/plain; charset=utf-8")
		w.WriteHeader(http.StatusUnauthorized)
		_, _ = w.Write([]byte("dsh web authentication required; reopen the URL printed by dsh web.\n"))
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

	ok, err := ValidateToken(context.Background(), u.Hostname(), port, good)
	if err != nil || !ok {
		t.Fatalf("valid token: ok=%v err=%v, want true, nil", ok, err)
	}

	ok, err = ValidateToken(context.Background(), u.Hostname(), port, "badtoken0123456789abcdef")
	if err != nil {
		t.Fatalf("invalid token returned an error: %v", err)
	}
	if ok {
		t.Fatal("invalid token reported as valid")
	}

	if _, err := ValidateToken(context.Background(), u.Hostname(), port, ""); err == nil {
		t.Fatal("empty token should be an error")
	}

	if _, err := ValidateToken(context.Background(), "127.0.0.1", 1, good); err == nil {
		t.Fatal("an unreachable endpoint should be an error, not a rejection")
	}
}

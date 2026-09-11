package service

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
)

// TokenValidationTimeout bounds one token validation request.
const TokenValidationTimeout = 3 * time.Second

// tokenPattern matches the base64url process launch token dsh prints (32 random
// bytes encoded -> 43 characters). The lower bound keeps short unrelated strings
// out; the upper bound rejects pasted blobs.
var tokenPattern = regexp.MustCompile(`^[A-Za-z0-9_-]{16,256}$`)

// ParseTokenInput extracts a launch token from whatever the user pasted: a bare
// token, the whole authenticated URL printed by `dsh web` (".../?token=..."), or
// a longer line that contains one (the ready line also carries a LAN address).
// It returns "" when nothing token-shaped is found, so callers can report a
// precise error instead of navigating with junk.
func ParseTokenInput(input string) string {
	s := strings.TrimSpace(input)
	if s == "" {
		return ""
	}
	// A terminal copy often carries the surrounding quotes.
	s = strings.Trim(s, "\"'`")
	s = strings.TrimSpace(s)

	if strings.Contains(s, "token=") {
		// Mostly a URL: let net/url do the decoding.
		if u, err := url.Parse(s); err == nil {
			if v := strings.TrimSpace(u.Query().Get("token")); v != "" {
				return v
			}
		}
		// Otherwise scan for the parameter inside surrounding text.
		if i := strings.Index(s, "token="); i >= 0 {
			rest := s[i+len("token="):]
			if j := strings.IndexAny(rest, "& \t\r\n\"'`"); j >= 0 {
				rest = rest[:j]
			}
			if v := strings.TrimSpace(rest); v != "" {
				return v
			}
		}
		return ""
	}
	if tokenPattern.MatchString(s) {
		return s
	}
	return ""
}

// ValidateToken reports whether token authenticates against the dsh web origin
// at host:port.
//
// dsh answers the process-token branch with 303 + Set-Cookie (or 200 when the
// request is already authenticated); an invalid or absent token gets the 401
// auth fence. A transport error means "unknown" and must not be reported as a
// rejection: the caller says "cannot reach the instance" instead.
func ValidateToken(ctx context.Context, host string, port int, token string) (bool, error) {
	if token == "" {
		return false, fmt.Errorf("service: empty token")
	}
	ctx, cancel := context.WithTimeout(ctx, TokenValidationTimeout)
	defer cancel()

	target := fmt.Sprintf("http://%s:%d/?token=%s", host, port, url.QueryEscape(token))
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, target, nil)
	if err != nil {
		return false, err
	}
	client := &http.Client{
		// The 303 *is* the answer: following it would hide the token exchange
		// and store a cookie in a throwaway jar we do not want.
		CheckRedirect: func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse },
	}
	resp, err := client.Do(req)
	if err != nil {
		return false, err
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusSeeOther, http.StatusOK:
		return true, nil
	case http.StatusUnauthorized:
		return false, nil
	}
	return false, fmt.Errorf("service: token validation got HTTP %d", resp.StatusCode)
}

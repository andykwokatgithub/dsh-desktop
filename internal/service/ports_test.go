package service

import (
	"net"
	"strconv"
	"testing"
)

func TestPickFreePort(t *testing.T) {
	port, err := PickFreePort("127.0.0.1", FallbackPortMin)
	if err != nil {
		t.Fatalf("PickFreePort: %v", err)
	}
	if port < FallbackPortMin || port > 65535 {
		t.Fatalf("PickFreePort returned %d, want [%d, 65535]", port, FallbackPortMin)
	}

	ln, err := net.Listen("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)))
	if err != nil {
		t.Fatalf("the picked port %d is not bindable: %v", port, err)
	}
	_ = ln.Close()

	if _, err := PickFreePort("127.0.0.1", 0); err == nil {
		t.Fatal("a minimum port of 0 should be rejected")
	}
	if _, err := PickFreePort("127.0.0.1", 70000); err == nil {
		t.Fatal("a minimum port above 65535 should be rejected")
	}
}

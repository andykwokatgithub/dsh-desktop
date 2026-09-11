package service

import (
	"fmt"
	"math/rand"
	"net"
	"strconv"
)

// FallbackPortMin is the lowest port used when the preferred endpoint cannot be
// taken: above the common service ports (3000/3080/8080) and inside the range
// the project requires for a self-started instance.
const FallbackPortMin = 10000

// PickFreePort returns a TCP port on host inside [min, 65535] that is bindable
// right now.
//
// The returned port is handed to dsh immediately, so a small race window
// remains: another process can still win the bind, which then surfaces as a
// startup timeout rather than as a wrong endpoint.
func PickFreePort(host string, min int) (int, error) {
	if min < 1 || min > 65535 {
		return 0, fmt.Errorf("service: invalid minimum port %d", min)
	}
	if host == "" {
		host = "127.0.0.1"
	}
	span := 65535 - min + 1
	for attempt := 0; attempt < 64; attempt++ {
		port := min + rand.Intn(span)
		ln, err := net.Listen("tcp", net.JoinHostPort(host, strconv.Itoa(port)))
		if err != nil {
			continue
		}
		_ = ln.Close()
		return port, nil
	}
	return 0, fmt.Errorf("service: no free port in [%d, 65535] on %s", min, host)
}

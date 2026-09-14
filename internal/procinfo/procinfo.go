// Package procinfo answers "which process owns this TCP port, and what is it?"
// on Windows using native APIs only.
//
// It deliberately avoids shelling out to netstat or PowerShell: netstat's table
// headers are localized and its text output needs fragile substring matching
// (":3080" also matches ":30801", a remote address, or an IPv6 literal), and a
// PowerShell/CIM query costs a subprocess spawn plus a WMI dependency. The same
// facts are available in-process through GetExtendedTcpTable (listening socket
// -> owning PID) and NtQueryInformationProcess (PID -> command line), which is
// what this package uses.
package procinfo

import (
	"errors"
	"fmt"
	"net"
	"syscall"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

const (
	afInet  = 2  // AF_INET
	afInet6 = 23 // AF_INET6

	// tcpTableOwnerPIDListener is TCP_TABLE_OWNER_PID_LISTENER: LISTENING rows
	// that carry the owning PID (MIB_TCPROW_OWNER_PID / MIB_TCP6ROW_OWNER_PID).
	tcpTableOwnerPIDListener = 3

	// processCommandLineInformation is the NtQueryInformationProcess class that
	// returns the target's command line as a UNICODE_STRING.
	processCommandLineInformation = 60

	// maxCommandLineBytes bounds the length the class may ask for, so a bogus
	// value can never turn into a wild allocation.
	maxCommandLineBytes = 64 << 10

	// fallbackCommandLineBytes is used when the size query reports nothing.
	fallbackCommandLineBytes = 8 << 10

	// maxAncestorDepth bounds the parent-chain walk. A dsh instance is a handful
	// of levels deep (shell -> cmd.exe -> node.exe -> workers), so eight is more
	// than enough and keeps a broken/corrupt chain from turning into a long walk.
	maxAncestorDepth = 8
)

var (
	iphlpapi = windows.NewLazySystemDLL("iphlpapi.dll")
	ntdll    = windows.NewLazySystemDLL("ntdll.dll")

	procGetExtendedTcpTable       = iphlpapi.NewProc("GetExtendedTcpTable")
	procNtQueryInformationProcess = ntdll.NewProc("NtQueryInformationProcess")
)

// Entry is one process's identity; fields that could not be read stay empty.
type Entry struct {
	PID         int
	ParentPID   int
	ImagePath   string
	CommandLine string
	StartedAt   time.Time // zero when unavailable
}

// listenRow is one LISTENING socket and the process that owns it.
type listenRow struct {
	ip   net.IP
	port int
	pid  int
}

// ListenerPID returns the PID owning the LISTENING socket that serves
// host:port, preferring an exact bound-address match and falling back to a
// wildcard (0.0.0.0 / ::) bind.
//
// A returned error means "unknown" -- nothing matched, or the table could not
// be read (for example an IPv6-only listener on an old interface set). Callers
// must not read it as "not dsh": the identification rules in
// docs/dsh-desktop-prd.md (FR-01) degrade to the HTTP fingerprint instead.
func ListenerPID(host string, port int) (int, error) {
	if port <= 0 || port > 65535 {
		return 0, fmt.Errorf("procinfo: invalid port %d", port)
	}
	rows, err := listenRows()
	if err != nil {
		return 0, err
	}
	want := hostIPs(host)
	for _, r := range rows {
		if r.port != port {
			continue
		}
		for _, ip := range want {
			if r.ip.Equal(ip) {
				return r.pid, nil
			}
		}
	}
	for _, r := range rows {
		if r.port == port && r.ip.IsUnspecified() {
			return r.pid, nil
		}
	}
	return 0, fmt.Errorf("procinfo: no LISTENING socket on %s:%d", host, port)
}

// Info reads what it can about one process: every field that fails (permissions,
// already exited) is simply left empty.
func Info(pid int) Entry {
	e := Entry{PID: pid}
	if parent, err := ParentPID(pid); err == nil {
		e.ParentPID = parent
	}
	if image, err := ImagePath(pid); err == nil {
		e.ImagePath = image
	}
	if cmd, err := CommandLine(pid); err == nil {
		e.CommandLine = cmd
	}
	if started, err := StartTime(pid); err == nil {
		e.StartedAt = started
	}
	return e
}

// Ancestors returns pid and its ancestors, nearest first, following up to
// depth parent links. The parent chain is a hint, not proof of ownership: a
// parent may have exited, and Windows keeps the recorded parent id after that.
func Ancestors(pid, depth int) []Entry {
	var out []Entry
	seen := map[int]bool{}
	for i := 0; i <= depth && pid > 0 && !seen[pid]; i++ {
		e := Info(pid)
		out = append(out, e)
		seen[pid] = true
		if e.ParentPID <= 0 || e.ParentPID == pid {
			break
		}
		pid = e.ParentPID
	}
	return out
}

// ParentPID returns the kernel-reported parent process id of pid.
func ParentPID(pid int) (int, error) {
	h, err := openProcess(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err != nil {
		return 0, err
	}
	defer windows.CloseHandle(h)

	var pbi processBasicInformation
	if err := queryInformation(h, 0, unsafe.Pointer(&pbi), unsafe.Sizeof(pbi)); err != nil {
		return 0, err
	}
	return int(pbi.inheritedFromUniqueProcessID), nil
}

// IsChildOf reports whether pid was started directly by parent.
//
// Unlike a recorded start time this needs nothing stored up front, and a
// recycled PID cannot fake it: only a process we actually spawned has us as its
// parent. It is the cheapest proof available that killing pid is safe.
func IsChildOf(pid, parent int) bool {
	if pid <= 0 || parent <= 0 || pid == parent {
		return false
	}
	got, err := ParentPID(pid)
	return err == nil && got == parent
}

// IsDescendantOf reports whether pid is a (possibly distant) descendant of
// ancestor, following the kernel's parent links. It answers the same question as
// IsChildOf one or more levels down -- useful when the process that ends up doing
// the work is a grandchild of the one we spawned -- and, like IsChildOf, it is a
// statement about the current process tree, so a reused PID cannot fake it.
func IsDescendantOf(pid, ancestor int) bool {
	if pid <= 0 || ancestor <= 0 || pid == ancestor {
		return false
	}
	for _, e := range Ancestors(pid, maxAncestorDepth) {
		if e.ParentPID == ancestor {
			return true
		}
	}
	return false
}

// Exited reports whether pid has already terminated.
//
// It exists because "the PID still resolves" is not the same as "the process
// still runs": a process object outlives its process while any handle to it is
// open, and Go's os/exec keeps a handle for every child it starts -- so a dead
// wrapper can still answer OpenProcess, GetProcessTimes and even a taskkill
// "failure". Waiting on the handle is what actually distinguishes the two.
//
// A process that cannot be opened returns an error, which callers must read as
// "unknown" rather than as "exited", so an unreadable process is never mistaken
// for a stopped one.
func Exited(pid int) (bool, error) {
	h, err := openProcess(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE)
	if err != nil {
		return false, err
	}
	defer windows.CloseHandle(h)

	event, err := windows.WaitForSingleObject(h, 0)
	if err != nil {
		return false, err
	}
	return event == windows.WAIT_OBJECT_0, nil
}

// ImagePath returns the fully-qualified executable path of pid.
func ImagePath(pid int) (string, error) {
	h, err := openProcess(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	buf := make([]uint16, 4096)
	size := uint32(len(buf))
	if err := windows.QueryFullProcessImageName(h, 0, &buf[0], &size); err != nil {
		return "", err
	}
	return windows.UTF16ToString(buf[:size]), nil
}

// CommandLine returns the command line of pid.
//
// It queries ProcessCommandLineInformation, which on current Windows copies the
// string into the caller's buffer as a self-contained UNICODE_STRING, so
// PROCESS_QUERY_LIMITED_INFORMATION suffices and no cross-process read is
// needed. (Should a build instead return a pointer into the target address
// space, the code falls back to a PROCESS_VM_READ read.)
//
// A failure means "unreadable" -- typically an elevated or other-session
// process -- and is not evidence about what the process is: callers degrade to
// weaker evidence.
func CommandLine(pid int) (string, error) {
	h, err := openProcess(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	// The class reports the required length through STATUS_INFO_LENGTH_MISMATCH
	// (0xC0000004); the upper bound keeps a bogus length from becoming a wild
	// allocation.
	var need uint32
	status, _, _ := procNtQueryInformationProcess.Call(uintptr(h), processCommandLineInformation, 0, 0, uintptr(unsafe.Pointer(&need)))
	if int32(status) >= 0 || need == 0 || need > maxCommandLineBytes {
		need = fallbackCommandLineBytes
	}
	buf := make([]byte, need)

	var returned uint32
	status, _, _ = procNtQueryInformationProcess.Call(uintptr(h), processCommandLineInformation,
		uintptr(unsafe.Pointer(&buf[0])), uintptr(len(buf)), uintptr(unsafe.Pointer(&returned)))
	if int32(status) < 0 {
		return "", fmt.Errorf("procinfo: NtQueryInformationProcess class %d: NTSTATUS 0x%08X", processCommandLineInformation, uint32(status))
	}

	us := (*unicodeString)(unsafe.Pointer(&buf[0]))
	if us.buffer == nil || us.length == 0 {
		return "", errors.New("procinfo: empty command line")
	}
	if start := uintptr(unsafe.Pointer(&buf[0])); uintptr(unsafe.Pointer(us.buffer)) >= start &&
		uintptr(unsafe.Pointer(us.buffer))+uintptr(us.length) <= start+uintptr(len(buf)) {
		return windows.UTF16ToString(unsafe.Slice(us.buffer, int(us.length)/2)), nil
	}
	return readRemoteCommandLine(pid, us)
}

// readRemoteCommandLine handles the layout where the UNICODE_STRING points into
// the target process instead of the caller's buffer.
func readRemoteCommandLine(pid int, us *unicodeString) (string, error) {
	h, err := openProcess(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.PROCESS_VM_READ)
	if err != nil {
		return "", err
	}
	defer windows.CloseHandle(h)

	raw := make([]byte, int(us.length)+2) // +2 keeps the terminating NUL
	var read uintptr
	if err := windows.ReadProcessMemory(h, uintptr(unsafe.Pointer(us.buffer)), &raw[0], uintptr(len(raw)), &read); err != nil {
		return "", err
	}
	if read < 2 {
		return "", errors.New("procinfo: short command line read")
	}
	return windows.UTF16ToString(unsafe.Slice((*uint16)(unsafe.Pointer(&raw[0])), read/2)), nil
}

// StartTime returns the creation time of pid. Pairing it with the PID is what
// makes an ownership record survive PID reuse.
func StartTime(pid int) (time.Time, error) {
	h, err := openProcess(pid, windows.PROCESS_QUERY_LIMITED_INFORMATION)
	if err != nil {
		return time.Time{}, err
	}
	defer windows.CloseHandle(h)

	var creation, exit, kernel, user windows.Filetime
	if err := windows.GetProcessTimes(h, &creation, &exit, &kernel, &user); err != nil {
		return time.Time{}, err
	}
	return time.Unix(0, creation.Nanoseconds()), nil
}

// processBasicInformation mirrors PROCESS_BASIC_INFORMATION (class 0). On
// 64-bit Windows NTSTATUS is 4 bytes followed by 4 bytes of padding, which the
// uintptr-sized first field reproduces.
type processBasicInformation struct {
	exitStatus                   uintptr
	pebBaseAddress               uintptr
	affinityMask                 uintptr
	basePriority                 uintptr
	uniqueProcessID              uintptr
	inheritedFromUniqueProcessID uintptr
}

// unicodeString mirrors UNICODE_STRING on 64-bit Windows.
type unicodeString struct {
	length        uint16
	maximumLength uint16
	buffer        *uint16
}

// mibTCPRowOwnerPID mirrors MIB_TCPROW_OWNER_PID.
type mibTCPRowOwnerPID struct {
	state      uint32
	localAddr  uint32
	localPort  uint32
	remoteAddr uint32
	remotePort uint32
	owningPID  uint32
}

// mibTCP6RowOwnerPID mirrors MIB_TCP6ROW_OWNER_PID.
type mibTCP6RowOwnerPID struct {
	localAddr     [16]byte
	localScopeID  uint32
	localPort     uint32
	remoteAddr    [16]byte
	remoteScopeID uint32
	remotePort    uint32
	state         uint32
	owningPID     uint32
}

func openProcess(pid int, access uint32) (windows.Handle, error) {
	if pid <= 0 {
		return 0, fmt.Errorf("procinfo: invalid pid %d", pid)
	}
	h, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return 0, fmt.Errorf("procinfo: OpenProcess(%d): %w", pid, err)
	}
	return h, nil
}

func queryInformation(h windows.Handle, class uint32, out unsafe.Pointer, size uintptr) error {
	var returned uint32
	status, _, _ := procNtQueryInformationProcess.Call(uintptr(h), uintptr(class), uintptr(out), size, uintptr(unsafe.Pointer(&returned)))
	if int32(status) < 0 {
		return fmt.Errorf("procinfo: NtQueryInformationProcess class %d: NTSTATUS 0x%08X", class, uint32(status))
	}
	return nil
}

// listenRows reads the LISTENING rows of both address families. IPv4 is the
// normal case (dsh binds 127.0.0.1); IPv6 is read too so a "--host ::1"
// endpoint still resolves to a PID.
func listenRows() ([]listenRow, error) {
	var rows []listenRow
	var firstErr error
	for _, af := range []uint32{afInet, afInet6} {
		buf, err := extendedTCPTable(af)
		if err != nil {
			if firstErr == nil {
				firstErr = err
			}
			continue
		}
		if af == afInet {
			rows = append(rows, parseIPv4Listeners(buf)...)
		} else {
			rows = append(rows, parseIPv6Listeners(buf)...)
		}
	}
	if len(rows) == 0 && firstErr != nil {
		return nil, firstErr
	}
	return rows, nil
}

// extendedTCPTable fetches one family's LISTENING table: a first call sizes the
// buffer (ERROR_INSUFFICIENT_BUFFER), the second fills it.
func extendedTCPTable(af uint32) ([]byte, error) {
	var size uint32
	ret, _, _ := procGetExtendedTcpTable.Call(0, uintptr(unsafe.Pointer(&size)), 0, uintptr(af), tcpTableOwnerPIDListener, 0)
	if ret != 0 && ret != uintptr(windows.ERROR_INSUFFICIENT_BUFFER) {
		return nil, fmt.Errorf("procinfo: GetExtendedTcpTable(af=%d) size: %w", af, syscall.Errno(ret))
	}
	if size == 0 {
		return nil, nil
	}
	buf := make([]byte, size)
	ret, _, _ = procGetExtendedTcpTable.Call(uintptr(unsafe.Pointer(&buf[0])), uintptr(unsafe.Pointer(&size)), 0, uintptr(af), tcpTableOwnerPIDListener, 0)
	if ret != 0 {
		return nil, fmt.Errorf("procinfo: GetExtendedTcpTable(af=%d): %w", af, syscall.Errno(ret))
	}
	if int(size) < len(buf) {
		buf = buf[:size]
	}
	return buf, nil
}

func parseIPv4Listeners(buf []byte) []listenRow {
	if len(buf) < 4 {
		return nil
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	rowSize := int(unsafe.Sizeof(mibTCPRowOwnerPID{}))
	var out []listenRow
	for i := uint32(0); i < count; i++ {
		off := 4 + int(i)*rowSize
		if off+rowSize > len(buf) {
			break
		}
		r := (*mibTCPRowOwnerPID)(unsafe.Pointer(&buf[off]))
		out = append(out, listenRow{
			// LocalAddr carries the address in network byte order.
			ip:   net.IPv4(byte(r.localAddr), byte(r.localAddr>>8), byte(r.localAddr>>16), byte(r.localAddr>>24)),
			port: networkPort(r.localPort),
			pid:  int(r.owningPID),
		})
	}
	return out
}

func parseIPv6Listeners(buf []byte) []listenRow {
	if len(buf) < 4 {
		return nil
	}
	count := *(*uint32)(unsafe.Pointer(&buf[0]))
	rowSize := int(unsafe.Sizeof(mibTCP6RowOwnerPID{}))
	var out []listenRow
	for i := uint32(0); i < count; i++ {
		off := 4 + int(i)*rowSize
		if off+rowSize > len(buf) {
			break
		}
		r := (*mibTCP6RowOwnerPID)(unsafe.Pointer(&buf[off]))
		ip := make(net.IP, net.IPv6len)
		copy(ip, r.localAddr[:])
		out = append(out, listenRow{ip: ip, port: networkPort(r.localPort), pid: int(r.owningPID)})
	}
	return out
}

// networkPort decodes a socket port that is stored in network byte order inside
// the low 16 bits of a DWORD (byte-swap only, no getservbyname).
func networkPort(v uint32) int {
	return int(uint16(v&0xff)<<8 | uint16(v>>8)&0xff)
}

// hostIPs resolves the probe host to comparable addresses. A literal is used
// as-is; a name is resolved best effort (an unresolvable name leaves only the
// wildcard fallback).
func hostIPs(host string) []net.IP {
	if host == "" {
		host = "127.0.0.1"
	}
	if ip := net.ParseIP(host); ip != nil {
		return []net.IP{ip}
	}
	ips, err := net.LookupIP(host)
	if err != nil {
		return nil
	}
	return ips
}

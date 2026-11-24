//go:build windows

package mem

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

// ProcessEnv returns one environment variable of a process ("" when unset or unreadable), from
// its own environment block: PEB -> ProcessParameters -> Environment / EnvironmentSize. The
// offsets are the caller's pointer size (a 64-bit reader sees a WoW64 target's native PEB,
// whose parameters carry the same block).
func ProcessEnv(p Process, name string) string {
	proc, ok := p.(process)
	if !ok {
		return ""
	}
	h := windows.Handle(proc.h)
	var pbi windows.PROCESS_BASIC_INFORMATION
	if windows.NtQueryInformationProcess(h, windows.ProcessBasicInformation, unsafe.Pointer(&pbi), uint32(unsafe.Sizeof(pbi)), nil) != nil {
		return ""
	}
	ptr := unsafe.Sizeof(uintptr(0))
	paramsOff, envOff, envSizeOff := uintptr(0x10), uintptr(0x48), uintptr(0x290)
	if ptr == 8 {
		paramsOff, envOff, envSizeOff = 0x20, 0x80, 0x3F0
	}
	read := func(addr uintptr, out unsafe.Pointer, n uintptr) bool {
		var got uintptr
		return windows.ReadProcessMemory(h, addr, (*byte)(out), n, &got) == nil && got == n
	}
	var params, env, size uintptr
	if !read(uintptr(unsafe.Pointer(pbi.PebBaseAddress))+paramsOff, unsafe.Pointer(&params), ptr) ||
		!read(params+envOff, unsafe.Pointer(&env), ptr) || env == 0 {
		return ""
	}
	if !read(params+envSizeOff, unsafe.Pointer(&size), ptr) || size == 0 || size > 0x40000 {
		size = 0x8000
	}
	buf := make([]uint16, size/2)
	var got uintptr
	windows.ReadProcessMemory(h, env, (*byte)(unsafe.Pointer(&buf[0])), size, &got)
	buf = buf[:got/2]
	prefix := strings.ToUpper(name) + "="
	for start := 0; start < len(buf); {
		end := start
		for end < len(buf) && buf[end] != 0 {
			end++
		}
		if end == start {
			break
		}
		kv := windows.UTF16ToString(buf[start:end])
		if strings.HasPrefix(strings.ToUpper(kv), prefix) {
			return kv[len(prefix):]
		}
		start = end + 1
	}
	return ""
}

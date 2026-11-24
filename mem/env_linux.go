//go:build linux

package mem

import (
	"bytes"
	"fmt"
	"os"
)

// ProcessEnv returns one environment variable of a process ("" when unset or unreadable).
func ProcessEnv(p Process, name string) string {
	b, err := os.ReadFile(fmt.Sprintf("/proc/%d/environ", p.Pid()))
	if err != nil {
		return ""
	}
	prefix := []byte(name + "=")
	for _, kv := range bytes.Split(b, []byte{0}) {
		if bytes.HasPrefix(kv, prefix) {
			return string(kv[len(prefix):])
		}
	}
	return ""
}

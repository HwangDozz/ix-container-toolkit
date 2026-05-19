// Package strutil provides shared string utilities.
package strutil

import "strings"

// IsSharedLibrary returns true if the filename looks like a shared library:
// libfoo.so, libfoo.so.1, libfoo.so.1.2.3, etc.
func IsSharedLibrary(name string) bool {
	if strings.HasSuffix(name, ".so") {
		return true
	}
	idx := strings.Index(name, ".so.")
	if idx < 0 {
		return false
	}
	suffix := name[idx+4:]
	if len(suffix) == 0 {
		return false
	}
	return suffix[0] >= '0' && suffix[0] <= '9'
}

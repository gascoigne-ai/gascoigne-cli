//go:build !windows

package ortrt

import (
	"unsafe"

	"github.com/ebitengine/purego"
)

func openLibrary(path string) (uintptr, error) {
	return purego.Dlopen(path, purego.RTLD_NOW|purego.RTLD_GLOBAL)
}

func lookup(h uintptr, name string) (uintptr, error) { return purego.Dlsym(h, name) }

// modelPath is a C string: ORTCHAR_T is char here.
func modelPath(path string) unsafe.Pointer {
	b := cstr(path)
	return unsafe.Pointer(&b[0])
}

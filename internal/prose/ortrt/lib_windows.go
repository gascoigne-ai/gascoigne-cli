package ortrt

import (
	"syscall"
	"unsafe"
)

func openLibrary(path string) (uintptr, error) {
	h, err := syscall.LoadLibrary(path)
	return uintptr(h), err
}

func lookup(h uintptr, name string) (uintptr, error) {
	return syscall.GetProcAddress(syscall.Handle(h), name)
}

// modelPath is UTF-16: ORTCHAR_T is wchar_t on Windows.
func modelPath(path string) unsafe.Pointer {
	p, _ := syscall.UTF16PtrFromString(path)
	return unsafe.Pointer(p)
}
